package panel

import (
	"fmt"

	"github.com/emusoi/mia-core/internal/runtime"
)

type MachineRow struct {
	runtime.Machine
	Reachable bool
	Detail    string
	Hosting   []string
}

func Machines(rows []MachineRow) Panel {
	p := Panel{
		Columns: []Column{{Name: "machine"}, {Name: "where"}, {Name: "state"}},
		Version: Version,
		ID:      "machines",
		Title:   "Machines",
		Actions: machineActions(),
		Hints:   []string{"⏎ shell there", "a add", "x forget", "q back"},
		Order:   []string{"shell", "add", "forget"},
		Empty:   "only this computer — `a` adds a machine you can ssh to",
	}
	local := Row{
		ID:      runtime.Local,
		Glyph:   GlyphDone,
		Cells:   []string{runtime.Local, "this computer", "", ""},
		Facts:   []string{"machine  local", "where  this computer"},
		Actions: nil,
	}
	all := []Row{local}
	for _, r := range rows {
		glyph, note := GlyphDone, ""
		if !r.Reachable {
			glyph, note = "✗", "unreachable"
		}
		hosting := ""
		if len(r.Hosting) > 0 {
			hosting = fmt.Sprintf("%d here", len(r.Hosting))
		}
		facts := []string{"machine  " + r.Name, "ssh  " + r.SSH, "engine  " + r.Engine}
		if r.Detail != "" {
			facts = append(facts, "answers  "+r.Detail)
		}
		for _, name := range r.Hosting {
			facts = append(facts, "hosts  "+name)
		}
		all = append(all, Row{
			ID:      r.Name,
			Glyph:   glyph,
			Cells:   []string{r.Name, r.SSH, r.Engine, hosting},
			Note:    note,
			Actions: []string{"shell", "forget"},
			Facts:   facts,
		})
	}
	p.Sections = []Section{{ID: "machines", Label: "machines", Rows: all}}
	return p
}

func machineActions() map[string]Action {
	return map[string]Action{
		"shell":  {Lands: true, Popup: true, Key: "⏎", Label: "shell there", Verb: "machine", Args: []string{"shell", "{row}"}, Help: "an ssh shell on the machine, in a popup"},
		"add":    {Key: "a", Label: "add a machine", Verb: "machine", Args: []string{"add", "{input}"}, Input: "name and ssh target", Report: true, Help: "probe it over ssh now and record its container engine; a tailnet name is an ssh target like any other"},
		"forget": {Key: "x", Label: "forget", Verb: "machine", Args: []string{"rm", "{row}"}, Confirm: "Forget {row}? Environments recorded there will not start until moved.", Help: "drop the machine from the list; nothing on it is touched"},
	}
}
