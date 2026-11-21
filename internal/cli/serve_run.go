package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// verbAsked is the body of POST /run: a verb as it is typed after `mia`,
// the folder it is typed in, and what it reads.
type verbAsked struct {
	Args  []string `json:"args"`
	Dir   string   `json:"dir"`
	Stdin string   `json:"stdin"`
}

// verbRan is what POST /run answers: what the verb printed, said and
// exited with.
type verbRan struct {
	Code   int    `json:"code"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// notServed says why the server does not run a verb, "" when it does. It
// has no terminal: a verb that draws on the one it is typed in, attaches
// it, asks it something or opens an editor in it is run where it is typed.
// So is one that never ends, and the server itself.
func notServed(args []string) string {
	if len(args) == 0 {
		return "no verb was given"
	}
	verb, rest := args[0], args[1:]
	sub := ""
	if len(rest) > 0 {
		sub = rest[0]
	}
	has := func(flag string) bool { return slices.Contains(rest, flag) }
	switch verb {
	case "chat", "git", "files", "review", "radar", "dash":
		return "is drawn on the terminal it is typed in"
	case "shell", "away":
		return "attaches the terminal it is typed in"
	case "serve":
		return "is the server"
	case "window", "windows":
		if sub == "open" {
			return "attaches the terminal it is typed in"
		}
	case "machine", "machines", "runtime":
		if sub == "attach" || sub == "shell" {
			return "attaches the terminal it is typed in"
		}
	case "env":
		if sub == "shell" {
			return "attaches the terminal it is typed in"
		}
	case "new":
		if has("--shell") {
			return "attaches the terminal it is typed in"
		}
	case "tmux":
		if sub == "menu" || sub == "next" {
			return "is drawn on the terminal it is typed in"
		}
	case "config":
		if sub == "" {
			return "opens an editor in the terminal it is typed in"
		}
	case "gc", "resurrect":
		if has("--ask") {
			return "asks the terminal it is typed in"
		}
	case "gateway":
		if sub == "trust" || sub == "untrust" {
			return "asks the terminal it is typed in"
		}
		if sub == "serve" {
			return "never ends"
		}
	case "api", "dev":
		if has("--follow") {
			return "never ends — GET /events follows a query"
		}
	}
	return ""
}

// run is POST /run: a verb run for whoever asks, by mia itself in the
// folder given, and then every question the listeners hold asked again.
func (s *server) run(w http.ResponseWriter, r *http.Request) {
	var asked verbAsked
	if err := json.NewDecoder(r.Body).Decode(&asked); err != nil {
		http.Error(w, `POST /run {"args":[…],"dir":"…","stdin":"…"}: `+err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if why := notServed(asked.Args); why != "" {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(verbRan{Code: exitUsage, Stderr: fmt.Sprintf("mia: `mia %s` %s: mia serve has no terminal — run it there\n", strings.Join(asked.Args, " "), why)})
		return
	}
	ran := verbRan{}
	self, err := os.Executable()
	if err == nil {
		var out, errs bytes.Buffer
		verb := exec.Command(self, asked.Args...)
		verb.Dir, verb.Stdin, verb.Stdout, verb.Stderr = asked.Dir, strings.NewReader(asked.Stdin), &out, &errs
		err = verb.Run()
		ran.Stdout, ran.Stderr = out.String(), errs.String()
	}
	var exited *exec.ExitError
	switch {
	case errors.As(err, &exited):
		ran.Code = exited.ExitCode()
	case err != nil: // it was not run: the folder is not there
		ran.Code, ran.Stderr = exitFailed, fmt.Sprintf("mia: %v\n", err)
	}
	s.hub.again()
	json.NewEncoder(w).Encode(ran)
}
