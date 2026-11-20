package cli

import (
	"encoding"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/stack"
)

var updateShapes = flag.Bool("update", false, "write testdata/api from the API as it is now")

// apiShapes is every answer of `mia api`: what is typed for it, and the Go
// type the verb marshals. A query in apiQueryList with no line here fails
// the test.
var apiShapes = []struct {
	name   string // the query, and its file in testdata/api
	usage  string
	output reflect.Type
}{
	{"capabilities", "mia api capabilities", reflect.TypeFor[capabilities]()},
	{"worktrees", "mia api worktrees [<worktree>...]", reflect.TypeFor[[]app.Listing]()},
	{"machines", "mia api machines", reflect.TypeFor[[]app.MachineState]()},
	{"here", "mia api here", reflect.TypeFor[app.HereState]()},
	{"repos", "mia api repos", reflect.TypeFor[[]app.Repository]()},
	{"env", "mia api env [<worktree>]", reflect.TypeFor[env.Snapshot]()},
	{"stack", "mia api stack [<worktree>]", reflect.TypeFor[[]stack.Layer]()},
	{"base", "mia api base", reflect.TypeFor[baseBranch]()},
	{"conversations", conversationsUsage, reflect.TypeFor[[]app.Head]()},
	{"conversation", conversationUsage, reflect.TypeFor[[]app.Turn]()},
	{"main", mainTurnsUsage, reflect.TypeFor[app.MainState]()},
	{"main conversations", "mia api main conversations", reflect.TypeFor[[]app.Head]()},
	{"main turns", "mia api main turns [--conversation <id>] [--after <n>] [--follow]", reflect.TypeFor[[]app.Turn]()},
	{"standing", standingUsage, reflect.TypeFor[app.Standing]()},
	{"agents", "mia api agents", reflect.TypeFor[[]app.AgentModels]()},
	{"setups", setupsUsage, reflect.TypeFor[[]app.Setup]()},
	{"accounts", "mia api accounts [--projects]", reflect.TypeFor[[]app.Account]()},
	{"accounts --projects", "mia api accounts --projects", reflect.TypeFor[[]app.Project]()},
	{"apps", "mia api apps", reflect.TypeFor[[]app.Manifest]()},
}

// closedSets are the string types whose values are all there are.
var closedSets = map[reflect.Type][]string{
	reflect.TypeFor[app.ReachState](): reachStates(),
	reflect.TypeFor[session.Kind]():   paneKinds(),
}

// closedFields are the plain strings, by the type that holds them and the
// name they are marshalled under, whose values are all there are.
var closedFields = map[string][]string{
	"app.Turn.kind": app.TurnKinds,
}

func paneKinds() []string {
	var all []string
	for _, kind := range session.Kinds {
		all = append(all, string(kind))
	}
	return all
}

func reachStates() []string {
	var all []string
	for _, state := range app.ReachStates {
		all = append(all, string(state))
	}
	return all
}

// shape is what a value looks like as JSON: its type, and for an object
// each field by the name it is marshalled under.
type shape struct {
	Type string `json:"type"` // string, time, integer, number, boolean, object, array, map, any, opaque
	// Optional is of a field: it may be left out. Nullable: it may be null.
	Optional bool              `json:"optional,omitempty"`
	Nullable bool              `json:"nullable,omitempty"`
	Enum     []string          `json:"enum,omitempty"`   // every value a string of a closed set takes
	Fields   map[string]*shape `json:"fields,omitempty"` // of an object
	Items    *shape            `json:"items,omitempty"`  // of an array, and of a map's values
	Go       string            `json:"go,omitempty"`     // the type, where the shape is its own doing or its own again
}

// input is one thing typed after a query: a flag, with the value it takes,
// a place, or a word.
type input struct {
	Name     string `json:"name"`
	Value    string `json:"value,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type apiShape struct {
	Query  string  `json:"query"`
	Usage  string  `json:"usage"`
	Inputs []input `json:"inputs"`
	Output *shape  `json:"output"`
}

var (
	marshaler     = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

// shapeOf reads a type as encoding/json writes it.
func shapeOf(t reflect.Type, within []reflect.Type) *shape {
	if t.Kind() == reflect.Pointer {
		of := shapeOf(t.Elem(), within)
		of.Nullable = true
		return of
	}
	switch {
	case t == reflect.TypeFor[time.Time]():
		return &shape{Type: "time"}
	case t.Implements(marshaler) || reflect.PointerTo(t).Implements(marshaler):
		return &shape{Type: "opaque", Go: t.String()} // it writes itself: its fields are not read here
	case t.Implements(textMarshaler) || reflect.PointerTo(t).Implements(textMarshaler):
		return &shape{Type: "string"}
	}
	switch t.Kind() {
	case reflect.String:
		return &shape{Type: "string", Enum: closedSets[t]}
	case reflect.Bool:
		return &shape{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &shape{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &shape{Type: "number"}
	case reflect.Interface:
		return &shape{Type: "any"}
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return &shape{Type: "string"} // base64
		}
		return &shape{Type: "array", Items: shapeOf(t.Elem(), within)}
	case reflect.Map:
		return &shape{Type: "map", Items: shapeOf(t.Elem(), within)}
	case reflect.Struct:
		if slices.Contains(within, t) {
			return &shape{Type: "object", Go: t.String()} // itself again, further down
		}
		object := &shape{Type: "object", Fields: map[string]*shape{}}
		fieldsOf(t, append(within, t), object.Fields)
		return object
	}
	panic(fmt.Sprintf("%s has no shape as JSON", t))
}

// fieldsOf adds a struct's fields as encoding/json names them: an embedded
// struct's are its holder's.
func fieldsOf(t reflect.Type, within []reflect.Type, into map[string]*shape) {
	for i := range t.NumField() {
		field := t.Field(i)
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" || (!field.IsExported() && !field.Anonymous) {
			continue
		}
		if field.Anonymous && name == "" {
			if embedded := field.Type; embedded.Kind() == reflect.Struct {
				fieldsOf(embedded, within, into)
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		of := shapeOf(field.Type, within)
		if values, closed := closedFields[t.String()+"."+name]; closed {
			of.Enum = values
		}
		of.Optional = strings.Contains(options, "omitempty") || strings.Contains(options, "omitzero")
		into[name] = of
	}
}

// inputsOf reads a usage line: what is in brackets may be left out.
func inputsOf(query, usage string) []input {
	inputs := []input{}
	depth := 0
	words := strings.Fields(strings.TrimPrefix(usage, "mia api "+strings.Fields(query)[0]))
	for _, word := range words {
		opens := len(word) - len(strings.TrimLeft(word, "["))
		word = word[opens:]
		closes := max(0, strings.Count(word, "]")-strings.Count(word, "["))
		word = word[:len(word)-closes]
		depth += opens
		for _, one := range strings.Split(word, "|") {
			switch last := len(inputs) - 1; {
			case one == "":
			case strings.HasPrefix(one, "<") && last >= 0 && strings.HasPrefix(inputs[last].Name, "--") && inputs[last].Value == "":
				inputs[last].Value = one
			default:
				inputs = append(inputs, input{Name: one, Required: depth == 0})
			}
		}
		depth -= closes
	}
	return inputs
}

// compared is what changed between the shape kept and the one now: breaks
// are what an app written to the kept one would trip on, added is what it
// would not notice.
type compared struct {
	breaks, added []string
}

func (c *compared) shapes(at string, was, is *shape) {
	if was.Type != is.Type {
		c.breaks = append(c.breaks, fmt.Sprintf("%s was %s and is %s", at, was.Type, is.Type))
		return
	}
	if !was.Optional && is.Optional {
		c.breaks = append(c.breaks, at+" was always there and may now be left out")
	}
	if !was.Nullable && is.Nullable {
		c.breaks = append(c.breaks, at+" was never null and may now be")
	}
	if was.Optional && !is.Optional || was.Nullable && !is.Nullable || was.Go != is.Go {
		c.added = append(c.added, at+" is surer than it was")
	}
	for _, value := range was.Enum {
		if !slices.Contains(is.Enum, value) {
			c.breaks = append(c.breaks, fmt.Sprintf("%s no longer takes the value %q", at, value))
		}
	}
	for _, value := range is.Enum {
		if !slices.Contains(was.Enum, value) {
			c.added = append(c.added, fmt.Sprintf("%s takes the new value %q", at, value))
		}
	}
	for _, name := range sorted(was.Fields) {
		now, kept := is.Fields[name]
		if !kept {
			c.breaks = append(c.breaks, at+"."+name+" was removed or renamed")
			continue
		}
		c.shapes(at+"."+name, was.Fields[name], now)
	}
	for _, name := range sorted(is.Fields) {
		if _, had := was.Fields[name]; !had {
			c.added = append(c.added, at+"."+name+" is new")
		}
	}
	if was.Items != nil && is.Items != nil {
		c.shapes(at+"[]", was.Items, is.Items)
	}
}

func (c *compared) inputs(at string, was, is []input) {
	named := func(all []input, name string) (input, bool) {
		for _, one := range all {
			if one.Name == name {
				return one, true
			}
		}
		return input{}, false
	}
	for _, old := range was {
		now, kept := named(is, old.Name)
		switch {
		case !kept:
			c.breaks = append(c.breaks, at+" no longer takes "+old.Name)
		case !old.Required && now.Required:
			c.breaks = append(c.breaks, at+" now needs "+old.Name+", which could be left out")
		case old != now:
			c.added = append(c.added, at+" takes "+old.Name+" otherwise")
		}
	}
	for _, now := range is {
		if _, had := named(was, now.Name); had {
			continue
		}
		if now.Required {
			c.breaks = append(c.breaks, at+" now needs "+now.Name+", a new input")
		} else {
			c.added = append(c.added, at+" takes "+now.Name+", which is new")
		}
	}
}

func sorted(fields map[string]*shape) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func shapeFile(query string) string {
	return filepath.Join("testdata", "api", strings.ReplaceAll(strings.ReplaceAll(query, " --", "-"), " ", "-")+".json")
}

// The apps are written to what `mia api` answered when they were built.
// testdata/api keeps that shape, read off the Go types; what is there may
// not go, change type, or become optional, and no query may come to need
// an input it did not. What is added passes, and is kept with -update.
func TestTheAPIAnAppWasBuiltOnStillAnswersTheSame(t *testing.T) {
	listed := map[string]bool{}
	var stale []string
	for _, one := range apiShapes {
		listed[strings.Fields(one.name)[0]] = true
		now := apiShape{Query: one.name, Usage: one.usage, Inputs: inputsOf(one.name, one.usage), Output: shapeOf(one.output, nil)}
		file := shapeFile(one.name)
		if *updateShapes {
			data, err := json.MarshalIndent(now, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, append(data, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		data, err := os.ReadFile(file)
		if os.IsNotExist(err) {
			stale = append(stale, "mia api "+one.name+" is new")
			continue
		}
		var kept apiShape
		if err == nil {
			err = json.Unmarshal(data, &kept)
		}
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var c compared
		c.shapes("mia api "+one.name+": the answer", kept.Output, now.Output)
		c.inputs("mia api "+one.name, kept.Inputs, now.Inputs)
		for _, broke := range c.breaks {
			t.Errorf("an app built on the released API breaks: %s", broke)
		}
		stale = append(stale, c.added...)
	}
	for _, query := range apiQueryList {
		if !listed[query] {
			t.Errorf("mia api %s has no line in apiShapes: its shape is not kept", query)
		}
	}
	kept, err := filepath.Glob(filepath.Join("testdata", "api", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range kept {
		if !slices.ContainsFunc(apiShapes, func(one struct {
			name   string
			usage  string
			output reflect.Type
		}) bool {
			return shapeFile(one.name) == file
		}) {
			t.Errorf("an app built on the released API breaks: the query kept in %s is gone", file)
		}
	}
	if len(stale) > 0 {
		t.Logf("testdata/api is behind the API in %d places, none of which breaks an app; keep them with\n\tgo test ./internal/cli -run TestTheAPIAnAppWasBuiltOnStillAnswersTheSame -update\n\t%s", len(stale), strings.Join(stale, "\n\t"))
	}
}

// What the comparison calls a break, and what it lets pass, on a small answer.
func TestWhatBreaksAnAppAndWhatDoesNot(t *testing.T) {
	type machine struct {
		Name    string         `json:"name"`
		Answers bool           `json:"answers"`
		Detail  string         `json:"detail,omitempty"`
		State   app.ReachState `json:"state"`
	}
	was := shapeOf(reflect.TypeFor[[]machine](), nil)
	breaks := func(is reflect.Type) string {
		t.Helper()
		var c compared
		c.shapes("the answer", was, shapeOf(is, nil))
		return strings.Join(c.breaks, "; ")
	}
	for _, c := range []struct {
		why  string
		is   reflect.Type
		want string
	}{
		{"a field removed", reflect.TypeFor[[]struct {
			Name   string         `json:"name"`
			Detail string         `json:"detail,omitempty"`
			State  app.ReachState `json:"state"`
		}](), "the answer[].answers was removed or renamed"},
		{"a field renamed", reflect.TypeFor[[]struct {
			Name    string         `json:"name"`
			Answers bool           `json:"answered"`
			Detail  string         `json:"detail,omitempty"`
			State   app.ReachState `json:"state"`
		}](), "the answer[].answers was removed or renamed"},
		{"a type changed", reflect.TypeFor[[]struct {
			Name    string         `json:"name"`
			Answers int            `json:"answers"`
			Detail  string         `json:"detail,omitempty"`
			State   app.ReachState `json:"state"`
		}](), "the answer[].answers was boolean and is integer"},
		{"a field that was always there made optional", reflect.TypeFor[[]struct {
			Name    string         `json:"name,omitempty"`
			Answers *bool          `json:"answers"`
			Detail  string         `json:"detail,omitempty"`
			State   app.ReachState `json:"state"`
		}](), "the answer[].answers was never null and may now be; the answer[].name was always there and may now be left out"},
		{"a closed set no longer a set", reflect.TypeFor[[]struct {
			Name    string `json:"name"`
			Answers bool   `json:"answers"`
			Detail  string `json:"detail,omitempty"`
			State   string `json:"state"`
		}](), `the answer[].state no longer takes the value "ok"`},
		{"a field added, an optional one made sure", reflect.TypeFor[[]struct {
			Name    string         `json:"name"`
			Answers bool           `json:"answers"`
			Detail  string         `json:"detail"`
			State   app.ReachState `json:"state"`
			Fix     string         `json:"fix,omitempty"`
		}](), ""},
	} {
		if got := breaks(c.is); !strings.HasPrefix(got, c.want) || (c.want == "") != (got == "") {
			t.Errorf("%s: breaks = %q, want %q", c.why, got, c.want)
		}
	}
	var added compared
	added.shapes("the answer", was, shapeOf(reflect.TypeFor[[]struct {
		machine
		Fix string `json:"fix,omitempty"`
	}](), nil))
	if len(added.breaks) != 0 || !slices.Equal(added.added, []string{"the answer[].fix is new"}) {
		t.Errorf("a field added: %+v", added)
	}

	// inputs: one taken away or newly needed breaks, a new optional one does not
	had := inputsOf("conversation", "mia api conversation <id> [--worktree <name>] [--after <n>]")
	if want := []input{{Name: "<id>", Required: true}, {Name: "--worktree", Value: "<name>"}, {Name: "--after", Value: "<n>"}}; !slices.Equal(had, want) {
		t.Fatalf("inputs = %+v", had)
	}
	if got := inputsOf("main", mainTurnsUsage); !slices.Equal(got, []input{{Name: "conversations"}, {Name: "turns"}, {Name: "--conversation", Value: "<id>"}, {Name: "--after", Value: "<n>"}, {Name: "--follow"}}) {
		t.Errorf("of %q, inputs = %+v", mainTurnsUsage, got)
	}
	if got := inputsOf("standing", standingUsage); !slices.Equal(got, []input{{Name: "--here"}, {Name: "--seen", Value: "<repository>/<worktree>[@<machine>]"}}) {
		t.Errorf("of %q, inputs = %+v", standingUsage, got)
	}
	for usage, want := range map[string]string{
		"mia api conversation <id> [--after <n>]":                                       "mia api conversation no longer takes --worktree",
		"mia api conversation <id> --worktree <name> [--after <n>]":                     "mia api conversation now needs --worktree, which could be left out",
		"mia api conversation <id> <turn> [--worktree <name>] [--after <n>]":            "mia api conversation now needs <turn>, a new input",
		"mia api conversation <id> [--worktree <name>] [--after <n>] [--before <n>]":    "",
		"mia api conversation <id> [--worktree <worktree>] [--after <n>] [--limit <n>]": "",
	} {
		var c compared
		c.inputs("mia api conversation", had, inputsOf("conversation", usage))
		if got := strings.Join(c.breaks, "; "); got != want {
			t.Errorf("%s: breaks = %q, want %q", usage, got, want)
		}
		if want == "" && len(c.added) == 0 {
			t.Errorf("%s: nothing is said to be new", usage)
		}
	}
}
