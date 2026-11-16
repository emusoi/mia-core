package app

import (
	"reflect"
	"testing"
)

// trustScreen is claude's question in a folder it has not seen, as claude
// 2.1.325 draws it: no numbers, and the cursor on No, exit.
const trustScreen = ` Accessing workspace:

 /home/user/.config/mia/main

 Quick safety check: Is this a project you created or one you trust? (Like your own code, a well-known open source project, or work from your
 team). If not, take a moment to review what's in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 Security guide

 ❯ No, exit
   Yes, I trust this folder

 Enter to confirm · Esc to cancel


`

func TestTheQuestionAScreenEndsWithIsWhatItAsks(t *testing.T) {
	for _, c := range []struct {
		name, screen string
		want         *Asks
	}{
		{"claude's trust in a folder, with no numbers", trustScreen,
			&Asks{Text: "Is this a project you created or one you trust?", Options: []string{"No, exit", "Yes, I trust this folder"}, Selected: 1}},
		{"its cursor moved", `
 Quick safety check: Is this a project you created or one you trust?

   No, exit
 ❯ Yes, I trust this folder

 Enter to confirm · Esc to cancel
`, &Asks{Text: "Is this a project you created or one you trust?", Options: []string{"No, exit", "Yes, I trust this folder"}, Selected: 2}},
		{"a permission, numbered", `
 Bash command

   rm -rf build
   Remove the build folder

 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for rm commands in this folder
   3. No, and tell Claude what to do differently (esc)

 Esc to cancel · Tab to amend
`, &Asks{Text: "Do you want to proceed?", Options: []string{"Yes", "Yes, and don't ask again for rm commands in this folder", "No, and tell Claude what to do differently (esc)"}, Selected: 1, numbered: true}},
		{"a choice in a box, the cursor on the second", `
╭──────────────────────────────╮
│ Which should go first?       │
│                              │
│   1. The login page          │
│ › 2. The settings page       │
╰──────────────────────────────╯
`, &Asks{Text: "Which should go first?", Options: []string{"The login page", "The settings page"}, Selected: 2, numbered: true}},
		{"a numbered list in what it said, its prompt under it", `
⏺ Three things to do:
  1. read the file
  2. change it
  3. test it

──────────────────────────────
❯
──────────────────────────────
  ? for shortcuts
`, nil},
		{"two lines typed into its prompt", `
──────────────────────────────
❯ first line of a draft
  and its second
──────────────────────────────
  ? for shortcuts
`, nil},
		{"a question answered long ago, far above", `
 Do you want to proceed?
 ❯ 1. Yes
   2. No

line
line
line
line
line
line
line
line
line
`, nil},
	} {
		got, on := asksOn(c.screen)
		if c.want == nil {
			if on {
				t.Errorf("%s: asks %+v", c.name, got)
			}
			continue
		}
		if !on || !reflect.DeepEqual(got, *c.want) {
			t.Errorf("%s: %+v, %v\nwant %+v", c.name, got, on, *c.want)
		}
	}
}

func TestAVerbRefusedWhileAnAgentAsksSaysHowToAnswer(t *testing.T) {
	asks, _ := asksOn(trustScreen)
	want := "claude asks first: Is this a project you created or one you trust? — answer it: `mia main press <n>` (1 No, exit · 2 Yes, I trust this folder)"
	if got := askedFirst("claude", asks, "mia main press").Error(); got != want {
		t.Errorf("said %q\nwant %q", got, want)
	}
}
