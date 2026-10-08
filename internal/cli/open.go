package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/emusoi/mia-core/internal/app"
)

type opener struct {
	name string
	argv func(path string) []string
	here func() bool
}

func onPath(bin string) func() bool {
	return func() bool { _, err := exec.LookPath(bin); return err == nil }
}

func onMac() bool { return runtime.GOOS == "darwin" }

func macApp(name string) func() bool {
	return func() bool { return onMac() && exec.Command("open", "-Ra", name).Run() == nil }
}

var openers = []opener{
	{"cursor", func(p string) []string { return []string{"cursor", p} }, onPath("cursor")},
	{"code", func(p string) []string { return []string{"code", p} }, onPath("code")},
	{"zed", func(p string) []string { return []string{"zed", p} }, onPath("zed")},
	{"idea", func(p string) []string { return []string{"idea", p} }, onPath("idea")},
	{"subl", func(p string) []string { return []string{"subl", p} }, onPath("subl")},
	{"xcode", func(p string) []string { return []string{"open", "-a", "Xcode", p} }, macApp("Xcode")},
	{"finder", func(p string) []string { return []string{"open", p} }, onMac},
	{"terminal", func(p string) []string { return []string{"open", "-a", "Terminal", p} }, onMac},
}

var Openers = sync.OnceValue(func() []string {
	var found []string
	for _, o := range openers {
		if o.here() {
			found = append(found, o.name)
		}
	}
	return found
})

func cmdOpen(a *app.App, args []string) int {
	with := takeValue(&args, "--in")
	if len(args) > 1 {
		return usageErr("mia open [worktree] [--in <cursor|code|zed|idea|subl|xcode|finder|terminal>]")
	}
	target := ""
	if len(args) == 1 {
		target = args[0]
	}
	record, err := a.RecordOrHere(target)
	if err != nil {
		return fail(err)
	}
	if with == "" {
		found := Openers()
		if len(found) == 0 {
			return fail(fmt.Errorf("no editor or file browser found to open %s in", record.Name))
		}
		with = found[0]
	}
	for _, o := range openers {
		if o.name != with {
			continue
		}
		argv := o.argv(record.Path)
		if err := exec.Command(argv[0], argv[1:]...).Start(); err != nil {
			return fail(fmt.Errorf("open %s in %s: %w", record.Name, with, err))
		}
		fmt.Printf("opened %s in %s\n", record.Name, with)
		return exitOK
	}
	return usageErr("mia open: no opener called " + with + " — this machine has " + strings.Join(Openers(), ", "))
}
