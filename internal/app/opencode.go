package app

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/run"
)

// opencodeTalks is opencode's conversations. It keeps them in a database
// of its own — SQLite, <its data folder>/opencode.db — whose tables are
// its own to change, so its own program is asked for them, as JSON:
//
//	opencode api GET /api/session?order=desc&limit=N[&cursor=…]   every conversation: its id, its folder, when
//	opencode session export <id>                                  one, whole: what it is and each message
//
// Either asks the server opencode keeps in the background, and starts it
// when none runs: neither is run for someone opencode has kept nothing
// for. `opencode session list` is not asked: it lists one folder's
// project, and makes the folder a project of opencode's to do it.
type opencodeTalks struct {
	// cache is the folder what was read of each conversation is kept in.
	cache string
}

// opencodeSession is what opencode says of a conversation; its times are
// milliseconds since 1970.
type opencodeSession struct {
	ID       string `json:"id"`
	ParentID string `json:"parentID"` // of a subagent's: its parent's conversation
	Time     struct {
		Created int64 `json:"created"`
		Updated int64 `json:"updated"` // when the person last did something to it
		Busy    int64 `json:"busy"`    // when it last set to work
		Idle    int64 `json:"idle"`    // when it last came to rest
	} `json:"time"`
	Location struct {
		Directory string `json:"directory"`
	} `json:"location"`
	// Model is the model it runs on: an object, its id and its provider's.
	Model json.RawMessage `json:"model"`
}

// model is the model it runs on as opencode takes one, provider/model;
// "" when it does not say, or says it in a shape mia does not know.
func (s opencodeSession) model() string {
	var on struct {
		ID         string `json:"id"`
		ProviderID string `json:"providerID"`
	}
	if json.Unmarshal(s.Model, &on) != nil || on.ID == "" || on.ProviderID == "" {
		return ""
	}
	return on.ProviderID + "/" + on.ID
}

// active is when it was last written to, as far as it says.
func (s opencodeSession) active() int64 {
	return max(s.Time.Created, s.Time.Updated, s.Time.Busy, s.Time.Idle)
}

// atRest reports whether nothing more is being written of it: it has come
// to rest since it last set to work. One at work is written to without
// any of its times changing.
func (s opencodeSession) atRest() bool { return s.Time.Busy <= s.Time.Idle }

// opencodeList is a page of a list of sessions.
type opencodeList struct {
	Data   []opencodeSession `json:"data"`
	Cursor struct {
		Next string `json:"next"`
	} `json:"cursor"`
}

// opencodeExport is one conversation as opencode exports it.
type opencodeExport struct {
	Info     opencodeSession   `json:"info"`
	Messages []opencodeMessage `json:"messages"`
}

// opencodeMessage is one message: the person's is its text, the agent's
// its content. The others — system, synthetic, shell, skill, compaction,
// idle, a switch of agent or model — are opencode's own bookkeeping.
type opencodeMessage struct {
	Type string `json:"type"`
	Time struct {
		Created int64 `json:"created"`
	} `json:"time"`
	Text    string         `json:"text"`
	Content []opencodePart `json:"content"`
}

// opencodePart is a part of what the agent said: text, reasoning or the
// use of a tool. opencode keeps what it streams of each only once that is
// done: a text or a reasoning is "" until the whole of it is there, and
// a tool's input "" from when the agent begins to call it until it has
// written what it hands it, then that, in a string, until the tool runs.
type opencodePart struct {
	Type string `json:"type"`
	Text string `json:"text"`
	ID   string `json:"id"`
	Name string `json:"name"`
	// Time is when a reasoning part was begun and done, in milliseconds
	Time struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
	State struct {
		Input    json.RawMessage `json:"input"` // an object, and a string until the tool runs
		Content  []opencodeText  `json:"content"`
		Metadata json.RawMessage `json:"metadata"` // each tool's own
	} `json:"state"`
}

type opencodeText struct {
	Text string `json:"text"`
}

// opencodeInput is what opencode hands a tool, as far as a line about it
// needs: its own tools' names for them, and those of the tools it had
// before its second version.
type opencodeInput struct {
	Path        string `json:"path"`
	FilePath    string `json:"filePath"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	Query       string `json:"query"`
	URL         string `json:"url"`
	ID          string `json:"id"`   // a skill's
	Name        string `json:"name"` // a skill's, before
	OldString   string `json:"oldString"`
	NewString   string `json:"newString"`
	Content     string `json:"content"`
	PatchText   string `json:"patchText"`
	Questions   []struct {
		Question string `json:"question"`
		Options  []struct {
			Label string `json:"label"`
		} `json:"options"`
	} `json:"questions"`
}

// opencodeAnswered is what its question tool keeps of what was chosen:
// for each question, the labels.
type opencodeAnswered struct {
	Answers [][]string `json:"answers"`
}

// opencodeNames is its tools by the name mia says the same thing under.
var opencodeNames = map[string]string{"subagent": "task"}

// opencodeID is an id as opencode makes one: nothing else is handed to it.
var opencodeID = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)

// opencodePage is how many sessions are asked for at once.
const opencodePage = 1000

// opencodeHome is the folder opencode keeps its data in.
func opencodeHome() string {
	if data := os.Getenv("XDG_DATA_HOME"); data != "" {
		return filepath.Join(data, "opencode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode")
}

// there reports whether opencode can be asked: it is on this machine and
// has kept something.
func (o opencodeTalks) there() bool {
	if _, err := exec.LookPath("opencode"); err != nil {
		return false
	}
	home := opencodeHome()
	if home == "" {
		return false
	}
	info, err := os.Stat(home)
	return err == nil && info.IsDir()
}

func (o opencodeTalks) ask(args ...string) (string, error) {
	return run.Tool{Binary: "opencode", Within: AgentTimeout}.Capture(args...)
}

// sessions is the folders' conversations, subagents' left out: theirs are
// their parent's.
func (o opencodeTalks) sessions(in folders) ([]opencodeSession, error) {
	if !o.there() {
		return nil, nil
	}
	var its []opencodeSession
	for cursor := ""; ; {
		path := fmt.Sprintf("/api/session?order=desc&limit=%d", opencodePage)
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		out, err := o.ask("api", "GET", path)
		if err != nil {
			return nil, err
		}
		var page opencodeList
		if err := json.Unmarshal([]byte(out), &page); err != nil {
			return nil, fmt.Errorf("its sessions are not the JSON mia knows: %w", err)
		}
		for _, s := range page.Data {
			if s.ID != "" && s.ParentID == "" && in.holds(s.Location.Directory) {
				its = append(its, s)
			}
		}
		if len(page.Data) < opencodePage || page.Cursor.Next == "" || page.Cursor.Next == cursor {
			return its, nil
		}
		cursor = page.Cursor.Next
	}
}

// export is one conversation whole.
func (o opencodeTalks) export(id string) (opencodeExport, error) {
	var whole opencodeExport
	out, err := o.ask("session", "export", id)
	if err != nil {
		return whole, err
	}
	// a field of another shape leaves the rest read; what is no JSON at all
	// has no id
	_ = json.Unmarshal([]byte(out), &whole)
	if whole.Info.ID != id {
		return whole, fmt.Errorf("session %s is not exported as the JSON mia knows", id)
	}
	return whole, nil
}

// exports is each of the conversations whole, a few read at once; one
// that could not be read is left out of both, and the first of those is
// why.
func (o opencodeTalks) exports(sessions []opencodeSession) (read []opencodeExport, of []opencodeSession, err error) {
	all, failed := make([]opencodeExport, len(sessions)), make([]error, len(sessions))
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(goruntime.NumCPU(), len(sessions)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				all[i], failed[i] = o.export(sessions[i].ID)
			}
		}()
	}
	for i := range sessions {
		next <- i
	}
	close(next)
	wg.Wait()
	for i, s := range sessions {
		if failed[i] != nil {
			err = cmp.Or(err, failed[i])
			continue
		}
		read, of = append(read, all[i]), append(of, s)
	}
	return read, of, err
}

// opencodeKept is what is kept of a conversation between runs, so that a
// list exports only those written to since: its times as they were, and
// what a list says of it.
type opencodeKept struct {
	Updated int64  `json:"updated"`
	Busy    int64  `json:"busy,omitempty"`
	Idle    int64  `json:"idle,omitempty"`
	Asked   string `json:"asked,omitempty"`
	Said    string `json:"said,omitempty"`
	Asks    int    `json:"asks,omitempty"`
	Turns   int    `json:"turns,omitempty"`
}

// of reports whether what is kept is of the conversation as it is now.
func (k opencodeKept) of(s opencodeSession) bool {
	return s.atRest() && k.Updated == s.Time.Updated && k.Busy == s.Time.Busy && k.Idle == s.Time.Idle
}

func (o opencodeTalks) cacheFile() string {
	if o.cache == "" {
		return ""
	}
	return filepath.Join(o.cache, "opencode.json")
}

func (o opencodeTalks) kept() map[string]opencodeKept {
	all := map[string]opencodeKept{}
	if data, err := os.ReadFile(o.cacheFile()); err == nil {
		_ = json.Unmarshal(data, &all)
	}
	return all
}

func (o opencodeTalks) keep(all map[string]opencodeKept) {
	file := o.cacheFile()
	if file == "" || os.MkdirAll(filepath.Dir(file), 0o755) != nil {
		return
	}
	if data, err := json.Marshal(all); err == nil {
		_ = writeAtomic(file, data)
	}
}

func (o opencodeTalks) heads(in folders) ([]Head, error) {
	sessions, err := o.sessions(in)
	if err != nil || len(sessions) == 0 {
		return nil, err
	}
	was := o.kept()
	now := make(map[string]opencodeKept, len(sessions)) // of those there are: what is kept of one that went is dropped
	var stale []opencodeSession
	for _, s := range sessions {
		if k, ok := was[s.ID]; ok && k.of(s) {
			now[s.ID] = k
		} else {
			stale = append(stale, s)
		}
	}
	read, of, err := o.exports(stale)
	for i, whole := range read {
		s := of[i]
		k := opencodeKept{Updated: s.Time.Updated, Busy: s.Time.Busy, Idle: s.Time.Idle}
		for _, turn := range opencodeTurns(whole.Messages) {
			k.Turns++
			switch turn.Kind {
			case turnAsked:
				if k.Asks == 0 {
					k.Asked = oneLine(turn.Text)
				}
				k.Asks++
			case turnSaid:
				k.Said = oneLine(turn.Text)
			}
		}
		now[s.ID] = k
	}
	if len(stale) > 0 || len(now) != len(was) {
		o.keep(now)
	}
	var heads []Head
	for _, s := range sessions {
		k, ok := now[s.ID]
		if !ok || k.Turns == 0 {
			continue // not read, or nothing said in it yet
		}
		heads = append(heads, Head{ID: s.ID, Agent: "opencode", Cwd: s.Location.Directory, Asked: k.Asked, Said: k.Said,
			Started: opencodeTime(s.Time.Created), Active: opencodeTime(s.active()), Turns: k.Asks, Model: s.model()})
	}
	return heads, err
}

// A conversation is in opencode's database once, whatever folder asks:
// there is no copy in cwd to choose.
func (o opencodeTalks) turns(in folders, id, cwd string, after int) ([]Turn, bool, error) {
	if !opencodeID.MatchString(id) || !o.there() {
		return nil, false, nil
	}
	whole, err := o.export(id)
	if err != nil {
		return nil, false, err
	}
	if made := whole.Info.Location.Directory; !in.holds(made) || (cwd != "" && made != cwd) {
		return nil, false, nil
	}
	var turns []Turn
	for _, turn := range numbered(opencodeTurns(whole.Messages)) {
		if turn.N > after {
			turns = append(turns, turn)
		}
	}
	return turns, true, nil
}

// One is exported whole, for there is no file of it to read on from, each
// time opencode has written its database: a reply it is still writing
// grows under its n.
func (o opencodeTalks) follow(in folders, id, cwd string, after int) (func() ([]Turn, bool, error), time.Duration) {
	next := polling(func() ([]Turn, bool, error) { return o.turns(in, id, cwd, after) })
	return whenWritten(func() []string { return o.written(in) }, opencodeBackstop, next), 150 * time.Millisecond
}

// opencodeBackstop is how often one followed is exported whatever its
// database says: opencode may be writing one mia does not know of.
const opencodeBackstop = 5 * time.Second

// written is what opencode writes its conversations to as it goes, every
// folder's alike: its database, the log SQLite writes ahead of it, and its
// data folder, which one made since is added to — not the memory SQLite
// shares beside them, which reading writes. OPENCODE_DB names its
// database; with none, it is opencode*.db there, one for each channel it
// has run on.
func (o opencodeTalks) written(folders) []string {
	home := opencodeHome()
	if home == "" {
		return nil
	}
	var dbs []string
	switch db := os.Getenv("OPENCODE_DB"); {
	case db == ":memory:":
		return nil
	case db != "":
		if !filepath.IsAbs(db) {
			db = filepath.Join(home, db)
		}
		dbs = []string{db}
	default:
		dbs, _ = filepath.Glob(filepath.Join(home, "opencode*.db"))
	}
	paths := []string{home}
	for _, db := range dbs {
		paths = append(paths, db, db+"-wal")
	}
	return paths
}

// ponytail: a search exports every conversation of the folders, each time;
// opencode's own search is of titles only. Keeping what was said beside
// what a list keeps is the upgrade when a repository has hundreds.
func (o opencodeTalks) search(in folders, text string, limit int) ([]Recalled, error) {
	needle := strings.ToLower(text)
	sessions, err := o.sessions(in)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(sessions, func(x, y opencodeSession) int { return cmp.Compare(y.active(), x.active()) })
	read, of, err := o.exports(sessions)
	var found []Recalled
	for i, whole := range read {
		if limit > 0 && len(found) >= limit {
			break
		}
		var yours, theirs *Recalled
		for _, turn := range numbered(opencodeTurns(whole.Messages)) {
			if (turn.Kind != turnAsked && turn.Kind != turnSaid) || !strings.Contains(strings.ToLower(turn.Text), needle) {
				continue
			}
			r := &Recalled{Conversation: Conversation{Session: of[i].ID, Cwd: of[i].Location.Directory, At: turn.At},
				Agent: "opencode", N: turn.N, Yours: turn.Kind == turnAsked, Line: around(turn.Text, needle)}
			if r.Yours {
				yours = r
			} else {
				theirs = r
			}
		}
		if r := cmp.Or(yours, theirs); r != nil { // your own words over the agent's, the latest of each
			found = append(found, *r)
		}
	}
	return found, err
}

// opencodeModels is opencode's models: those of the providers it is
// signed in to, which only it knows, so its own program is asked —
// opencode models, a provider/model a line — and what it said is kept.
// It is asked only where its conversations are: on this machine, with
// something kept.
type opencodeModels struct {
	// cache is the folder what it said is kept in; nothing is kept with none.
	cache string
}

const modelsKept = time.Hour

// modelListing is what is kept of what an agent said its models are.
type modelListing struct {
	At     time.Time      `json:"at"`
	Models []plugin.Model `json:"models"`
}

// fresh reports whether what is kept still stands at now.
func (l modelListing) fresh(now time.Time) bool {
	kept := modelsKept
	if len(l.Models) == 0 {
		kept = time.Minute
	}
	return !l.At.IsZero() && now.Sub(l.At) < kept
}

// keptModels is the models an agent says it has, what it said kept in
// cache, as <agent>.json, for modelsKept, so that a list of agents does
// not start it each time; that it said none, or could not say, for a
// minute. ask asks it, and is false where it is not worth asking: then it
// has none, and nothing is kept.
func keptModels(cache, agent string, ask func() ([]plugin.Model, bool)) []plugin.Model {
	file := ""
	if cache != "" {
		file = filepath.Join(cache, agent+".json")
	}
	var kept modelListing
	if data, err := os.ReadFile(file); err == nil && json.Unmarshal(data, &kept) == nil && kept.fresh(time.Now()) {
		return kept.Models
	}
	models, asked := ask()
	if !asked {
		return nil
	}
	kept = modelListing{At: time.Now().UTC(), Models: models}
	if file != "" && os.MkdirAll(cache, 0o755) == nil {
		if data, err := json.Marshal(kept); err == nil {
			_ = writeAtomic(file, data)
		}
	}
	return kept.Models
}

func (o opencodeModels) models() []plugin.Model {
	return keptModels(o.cache, "opencode", func() ([]plugin.Model, bool) {
		if !(opencodeTalks{}).there() {
			return nil, false
		}
		out, _ := run.Tool{Binary: "opencode", Within: AgentTimeout}.Capture("models") // one that fails has none
		return opencodeListed(out), true
	})
}

// opencodeListed is the models in what opencode models prints: each line
// that is a provider/model and nothing more, once, named by what comes
// after its provider. Anything else it prints is no model.
func opencodeListed(out string) []plugin.Model {
	models := []plugin.Model{}
	for _, line := range strings.Split(out, "\n") {
		id := strings.TrimSpace(line)
		provider, name, ok := strings.Cut(id, "/")
		if !ok || provider == "" || name == "" || strings.ContainsAny(id, " \t") {
			continue
		}
		if !slices.ContainsFunc(models, func(m plugin.Model) bool { return m.ID == id }) {
			models = append(models, plugin.Model{ID: id, Label: name})
		}
	}
	return models
}

func opencodeTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// opencodeTurns is a conversation's messages as turns, in order, not yet
// numbered, the one the agent is still writing as far as opencode keeps
// it: what it thought and what it said each once it is done, and a tool
// from when the agent begins to call it, said again as what it is handed
// is written. A turn stays where it is as parts are added after it, and
// so keeps its number as it grows: a commit a tool made is said right
// after that tool.
func opencodeTurns(messages []opencodeMessage) []Turn {
	var all []Turn
	for _, m := range messages {
		at := opencodeTime(m.Time.Created)
		switch m.Type {
		case "user":
			if strings.TrimSpace(m.Text) != "" {
				all = append(all, Turn{Kind: turnAsked, At: at, Text: m.Text})
			}
		case "assistant":
			var turns []Turn
			for _, part := range m.Content {
				switch part.Type {
				case "text":
					if strings.TrimSpace(part.Text) != "" {
						turns = append(turns, Turn{Kind: turnSaid, At: at, Text: part.Text})
					}
				case "reasoning": // what it keeps encrypted, in native, is never read
					if strings.TrimSpace(part.Text) != "" {
						turn := Turn{Kind: turnThought, At: at, Text: part.Text}
						if begun, done := part.Time.Created, part.Time.Completed; begun > 0 && done > begun {
							turn.Seconds = int((done - begun + 500) / 1000)
						}
						turns = append(turns, turn)
					}
				case "tool":
					turns = append(turns, opencodeTool(part, at)...)
					var output strings.Builder
					for _, c := range part.State.Content {
						output.WriteString(c.Text)
					}
					turns = append(turns, commitTurns(output.String(), at)...)
				}
			}
			all = append(all, readsTogether(turns)...)
		}
	}
	return all
}

// opencodeTool is the use of a tool in a line, as toolTurn says the other
// agents'; a question asked is a turn for each, and what was chosen one
// after it.
func opencodeTool(part opencodePart, at time.Time) []Turn {
	input := part.State.Input
	var written string
	if json.Unmarshal(input, &written) == nil {
		if strings.TrimSpace(written) == "" { // begun, and nothing it is handed written yet
			return []Turn{{Kind: turnTool, At: at, Name: part.Name, Text: "Using " + cmp.Or(opencodeNames[strings.ToLower(part.Name)], part.Name)}}
		}
		input = json.RawMessage(written)
	}
	var in opencodeInput
	_ = json.Unmarshal(input, &in)
	if len(in.Questions) > 0 {
		var answered opencodeAnswered
		_ = json.Unmarshal(part.State.Metadata, &answered)
		var turns []Turn
		for i, q := range in.Questions {
			turn := Turn{Kind: turnQuestion, At: at, Text: q.Question, ref: fmt.Sprintf("%s#%d", part.ID, i)}
			for _, o := range q.Options {
				turn.Options = append(turn.Options, o.Label)
			}
			turns = append(turns, turn)
			if i < len(answered.Answers) && len(answered.Answers[i]) > 0 {
				turns = append(turns, Turn{Kind: turnAnswer, At: at, Text: strings.Join(answered.Answers[i], ", "), ref: turn.ref})
			}
		}
		return turns
	}
	var turn Turn
	if in.PatchText != "" {
		turn = patchTurn(in.PatchText, at)
	} else {
		turn = toolTurn(cmp.Or(opencodeNames[strings.ToLower(part.Name)], part.Name), toolInput{Path: cmp.Or(in.Path, in.FilePath), Command: in.Command,
			Description: in.Description, Pattern: in.Pattern, Query: in.Query, URL: in.URL, Skill: cmp.Or(in.ID, in.Name),
			OldString: in.OldString, NewString: in.NewString, Content: in.Content}, at)
	}
	turn.Name = part.Name
	return []Turn{turn}
}

// numbered is turns numbered from one, each answer tied to the question
// it answers and said on it: what reading a transcript does line by line.
func numbered(turns []Turn) []Turn {
	open := map[string]int{} // a question's ref → where it is
	for i := range turns {
		turn := &turns[i]
		turn.N = i + 1
		switch {
		case turn.Kind == turnQuestion && turn.ref != "":
			open[turn.ref] = i
		case turn.Kind == turnAnswer:
			if at, ok := open[turn.ref]; ok {
				turn.Question, turns[at].Answer = turns[at].N, turn.Text
				delete(open, turn.ref)
			}
		}
	}
	return turns
}
