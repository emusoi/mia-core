# Changes

What each release adds, changes and takes away, newest first. Work lands
under **Unreleased**. A release renames that heading to its tag and date,
`## v0.2.0 — 2026-11-09`, and the same section becomes the release's notes
on GitHub; a tag without its section is not released.

## Unreleased

### Added

- **One list in tmux.** `prefix k` (⌘K) opens one list in which what is
  typed says what Enter does: who waits and who works, a project's
  worktrees (`project/`), any worktree by name, an answer by its number,
  an agent started on a task (`pi fix the test`), this conversation handed
  to another agent and model (`> codex`), this worktree moved to a machine
  or brought back (`@`), `new` and `rm`, the stack, the editor, lazygit,
  and past conversations on every machine, resumed. `prefix K` (⌘O) is a
  popup terminal, `prefix e` (⌘.) acts on this window, `prefix b` and `B`
  go back and forward, and `mia tmux keys kitty|ghostty` prints the lines
  that give a terminal app the ⌘ keys. See [mia in tmux](tmux.md).

- **A tool's turn says how it went.** A `tool` turn of
  `mia api conversation`, `mia api main turns` and a chat has `status`
  once the agent's transcript says: `ok`, or `error` when it failed —
  claude's `is_error`, pi's `isError`, opencode's part state, the code a
  codex command exited with or how its script ended, and what agy says of
  a command. Followed, the tool comes again under its `n` with it once
  what it gave back is written. It is left out while the tool runs and
  where the transcript says nothing of it, so a reader that knows no
  `status` reads every turn as before. See
  [Conversations](api.md#conversations).

- **`mia api sessions`, `signals` and `plugins`.** What `mia resurrect
  --list --json`, `mia signal ls --json` and `mia plugin ls --json`
  print is a query of `mia api`, and so of [mia serve](serve.md#get-apiquery),
  which keeps it as it keeps any: mia desk asks the server for them and
  starts no mia. See [Automation](api.md#json-commands).

- **A chat of mia's is drawn as a window's chat is.** `mia chat --chat
  <name>`, `main` the main agent, is the same chat as `mia chat --window`
  over a chat of mia's, from any folder: its thread followed, each switch
  the band its own turn says; what is sent, chosen and stopped done as
  `mia chats say`, `press` and `interrupt` do it, the agent at work at
  once after a send; Models switching its model as `mia chats model` does
  and going on with another agent as `mia chats switch` does; Fork making
  a chat of its own that goes on from a reply and models marked sending
  the next message to a chat each; Earlier its own
  conversations. With `[main] machine` another machine's, it runs
  there over ssh with this terminal as its own. See
  [Chats](chats.md#shown-as-a-chat).

- **`mia serve`, a server of this machine's.** It answers every query of
  `mia api` over a unix socket of the person's alone,
  `~/.config/mia/serve/serve.sock`, as `GET /api/<query>`; tells whoever
  listens to `GET /events` when an answer changes, as server-sent events,
  asking once for every listener and again only when tmux or a file says
  something changed; and runs a verb for whoever asks, `POST /run`,
  refusing one that needs a terminal. `mia api` asks it when it runs and
  is the same build, and reads for itself otherwise, or when
  `MIA_NO_SERVE` is set. It runs in the foreground, or as a launchd or
  systemd service, `mia serve install|uninstall|start|stop|status`,
  written with the environment of the shell it is installed from; a
  server that sees mia rebuilt ends, for the service to start the new
  build. `mia git` and `mia files` listen to it for `mia api status`, a
  worktree's git status, and `mia chat` for `mia api window`, whether a
  window's agent is at work and what its screen asks, in place of looking
  each second or two, when one of their build answers. See
  [mia serve](serve.md).

- **agy, the Antigravity CLI, is an agent mia knows.** A window that
  runs `agy` is agy's; `mia api agents` lists it, installed when `agy`
  is on `PATH`, with the models `agy models` prints, an id and a name a
  line, kept for an hour as opencode's are, and the efforts `low` to
  `max`: `mia window new <worktree> agy --model <id> --effort <level>`
  starts `agy --model <id> --effort <level>`, and the chat's models have
  them under its name. Its conversations are listed, read, followed as
  it writes them and recalled as the others' are: mia reads the steps
  agy writes of each, in `~/.gemini/antigravity-cli/brain/<id>`, what
  the person sent without agy's own lines around it — a paste as it was
  pasted, so a conversation carried to it is the person's — what it
  said, and each tool it used in a line, and the folder each was made in
  from what agy keeps of them apart. One is taken up again as
  `agy --conversation <id>`; handed to another agent and back, agy takes
  its own up again so, and is handed only what was said since; and
  `mia window model` and a pick in the chat switch it by starting it
  again in its pane on its conversation. Its window has no title that
  says it works, so `mia api window` and the chat say agy is at work
  until the last step it wrote of the conversation the window has open
  is a reply that calls no tool. See [Agents](api.md#agents) and
  [conversations](api.md#conversations).

- **The chat's models and Earlier are modals.** In `mia chat`, `models`
  and `earlier` no longer take the chat's place: each is a card over it,
  the conversation in sight behind, and `esc` takes it away with nothing
  changed. Models is the search line over the list, as it was; Earlier
  the list beside the conversation under the keys. Each conversation in
  Earlier has a menu, under the pointer's other button too — Resume,
  Continue with this chat's agent, Copy its id — as each model has its
  efforts. A send the window refuses gives the words back to the chat's
  field; a turn taken back is taken out of the same chat, so what is
  being typed stays; the status changes alone; and what went well — a
  model switched, a conversation handed over or taken up — is a toast
  that goes by itself, not a status that stays. See
  [conversations](api.md#conversations).

- **opencode has models.** opencode's models
  are those of the providers it is signed in to, so mia asks it —
  `opencode models`, a `provider/model` a line — and keeps what it said
  for an hour: `mia api agents` lists them, each labelled by what comes
  after its provider, and the chat's models have them under Opencode
  with the provider beside each. When opencode cannot say — not
  installed, nothing kept yet, not signed in, no answer in 5 s — it has
  none, and one row as before. opencode's own interface has no word for
  a model and its minimal one has, so `mia window new <worktree>
  opencode --model <provider/model>` starts `opencode mini --model …`,
  and with no model `opencode` as before. Nothing typed into it switches
  it, so `mia window model`, and a pick in the chat, end it and start it
  again in the same pane with the model and `--session <id>`: the
  window, its number and what is kept on it stay; `mia api agents` says
  so with `again`, and `mia api capabilities` lists `agent-again`. A
  model of opencode's picked from another agent's chat hands the
  conversation over to it on that model. A conversation of opencode's
  says the model opencode says it runs on, as `model` in
  `mia api conversations` and over its chat, until mia chooses one for
  its window. It has no effort mia can choose. mia reads neither its
  sign-in nor its config.
  See [Agents](api.md#agents).

- **How a worktree stands in git is an app mia desk draws.** `mia git`,
  run in a pane in any folder of a repository, says it to mia desk by its
  UI protocol: the branch and where it stands against its base — the
  layer under it in a stack, else the base branch — over a list of the
  files staged, changed and untracked and the commits ahead, with the
  diff of the row under the keys beside it, hunk by hunk with each line's
  numbers. `space` stages a file or unstages a staged one, and on a line
  of the diff the hunk that line is in; `a` stages all; `c` brings up a
  commit screen, whose message is committed as typed; `r` asks again.
  The standing is read again after each, and when `git status`, asked
  every 2 s, says something else. A long diff goes on in patches and
  ends at 1500 lines. What fails is the view's status and the app stays
  up. `mia api capabilities` lists `git-app`.
  See [mia git](api.md#mia-git).

- **`mia git` does what a daily git tool does.** `d` discards what is
  changed in a file, the hunk under the keys, or deletes an untracked
  file; `⏎` on a line added or deleted stages that line alone, and
  unstages it in a staged file's diff; `u` on the last commit takes it
  off and leaves its changes staged; the commit screen has **Amend the
  last one**; `b` brings up the branches, to switch to one, follow a
  remote's or make the one named; `s` stashes everything, and a stash,
  listed under the commits with its diff, is applied with `space`,
  popped with `p` and dropped with `d`; `f` fetches, `P` pulls where the
  branch can only be moved on, `p` pushes to what the branch follows, or
  to `origin` under its own name. What loses something or sends it —
  discard, drop, undo, push — is asked twice: the first press says
  exactly what in the status, the second does it. Nothing is merged,
  rebased or pushed by force, and a commit the upstream has is neither
  amended nor undone. With a remote git runs beside the app and the
  status says so. The head says what is to push and to pull, a commit
  says its subject and files over its diff, and the foot says the keys
  of where the keys are. Not yet: merging and rebasing, conflicts, a
  push by force, deleting a branch, tags, colour by syntax.

- **`mia git` asks in a modal, says what it did in a toast, and has a
  menu on every row.** Discard, delete, drop, undo and push put up a
  modal that says what is lost or sent, with a button that does it —
  its key the key that asked, so the key twice still does it — and one
  that keeps things as they are, as `esc` does; what it says is checked
  again as it is confirmed. The branches are a modal too. What was just
  done is a toast, and the status is kept for what was refused or
  failed; it and the keys are said alone, by a patch. Each file, commit
  and stash has a menu, under `→` and the pointer's other button: stage,
  discard, open in the editor (`e`), copy its path or hash (`y`), undo,
  apply, pop, drop. The foot's keys follow the piece the window says the
  keys are on, and the diff names what `⏎` does on a line. With more
  than 30 files the list is behind tabs, Changes, Commits and Stashes;
  and a fetch, pull or push is a bar at the head's end while it runs.
  See [mia git](api.md#mia-git).

- **A worktree's files are an app mia desk draws.** `mia files`, run in a
  pane in any folder of a repository, says them to mia desk by its UI
  protocol: the tree of the files git does not ignore, a folder opened
  and shut with `⏎`, a file changed against the base marked `A`, `M` or
  `D`, and beside it the file under the keys with its lines numbered.
  `e` hands the file to `$EDITOR` (else `nvim`) at the line under the
  keys and takes the pane again when it leaves; `f` finds a file by the
  letters of its path; `/` finds lines in the files with `git grep`, said
  as they are found, 500 at most — as written, after a `/` by a regular
  expression, after an `=` whole words alone; `r` asks again. `a` makes
  a file, handed to the editor at once, or with a `/` at its end a
  folder, where the keys are; `R` renames or moves the path under them,
  with `git mv` where git tracks it; `D` deletes it, asked twice, the
  status saying first what goes and whether git can bring it back; `y`
  copies its path by an OSC 52 string. `c` lists only what changed; `d`
  shows a changed file as its diff against the base; `B` puts each
  line's commit and its age at its right, and `⏎` on one shows that
  commit; `:` goes to a line. A file of any length is read to its end,
  1500 lines said at a time around the keys. It reads the files again
  when `git status`, asked every 2 s, says something else. In a folder
  of no repository the tree is the folder's. Not yet: editing in place.
  `mia api capabilities` lists `files-app`. See [mia
  files](api.md#mia-files).
- **`mia files` asks in a modal, says what it did in a toast, and has a
  menu on every row.** The name of a new file or folder, where a path is
  renamed to and the line to go to are asked in a field in a modal over
  the view, with why a name is refused under it. `D` puts up a modal
  that says what goes and whether git can bring it back, with **Delete**
  — its key `D`, so `D` twice still deletes — and **Keep it**, as `esc`
  does; what goes is counted again as it is confirmed. `made`, `now`,
  `deleted` and `copied` are toasts, and a path is copied by the
  window's own `copy`, no longer by an OSC 52 string. Each row of the
  tree has a menu, under `→` and the pointer's other button: open in
  the editor, new file here, rename, delete, copy path, diff against the
  base, blame. The foot's keys follow the piece the window says the keys
  are on. See [mia files](api.md#mia-files).
- **A code review is an app mia desk draws.** `mia review`, run in a
  pane of a worktree, draws the review sawa keeps of it — the one of the
  branch against its base, opened when there is none; `mia review
  <range>`, `pr <number>` or `w` another — by desk's UI protocol: the
  change's title, the findings that stand and the verdict over its
  files, by folder or (`o`) in sawa's reading order with why, and beside
  them the diff of the file under the keys with each finding as a note
  under the line it is about. `n` and `N` go through the findings across
  files; `⏎` opens one, to reply or reply and resolve; `c` comments on a
  line; `space` marks a file read and shows the next unread, and when
  all are read puts the keys on the verdict; `v` gives the verdict; `u`
  shows the resolved; `r` looks again. A finding on the review as a
  whole is listed in the header under the summary. One that suggests an
  edit shows it as a small diff with **Apply**, and one resolved has
  **Reopen**. `f` shows every open finding, those that wait on you, or
  the resolved too; `g` shows sawa's gates, and `⏎` on one what was
  judged, with Pass, Warn and Fail to record it; `e` hands the file to
  your editor at the line; `[` and `]` go to the layer under and over
  where sawa reviews a stack. A hunk's head says the function it is in,
  as git names it. Each is done by `sw` as you, never as an agent, and
  what sawa refuses is the view's status. The review is read again when
  sawa's record changes, so an agent's findings and answers come in as
  you read. Not yet: changing the words of a finding you filed (sawa has
  no verb for it), sawa's map and patterns, a pull request's GitHub
  comments. `mia api capabilities` lists `review-app`. See [mia
  review](api.md#mia-review).
- **`mia review` asks in a modal, says what it did in a toast, and has
  a menu on its files and findings.** The verdicts and the gates come up
  in a modal over the view, which stays as it was under them, and `esc`
  takes them away. A reply sent, a finding resolved, reopened or filed,
  an edit applied, a file marked read, a gate's result and the verdict
  given are toasts; the status is kept for what sawa refused. Each file
  has a menu, under `→` and the pointer's other button — mark read,
  open in the editor, copy path — and each finding on the review as a
  whole one — reply, resolve or reopen, copy its text; `y` copies a
  path, or what a finding says. The foot's keys follow the piece the
  window says the keys are on, and the diff says `⏎` opens a finding.
  See [mia review](api.md#mia-review).

- **The chat does what desk's own chat did.** `mia chat` fills its pane
  under a line that says the agent, its model, effort and account. What
  is sent shows at once and is not said twice when the transcript has
  it; one the window refuses is taken back, with why and the words as
  the status. What the agent's screen asks — a permission — is a question
  in the chat, and an answer is pressed in its window for every agent.
  `models` (m, by `tab` from the field) is a palette of the setups used
  here and each installed agent's models with their efforts: a model of
  the chat's own agent switches the window, one of another agent hands
  the conversation over — written out as a handoff, that agent started
  on the model and told nothing, the old window closed once the new one
  runs, the chat gone on there, and the handoff said in front of the
  first thing sent, by whichever `mia chat` of that window sends it: it
  is kept on the window (`@mia_context`) until then. `earlier` (e) lists the worktree's conversations
  beside the one under the keys read back: `⏎` goes on with one, `c`
  continues another agent's in this chat's agent, asked twice. A window
  of an agent mia reads no conversations of still sends, and says so.
  See [conversations](api.md#conversations).

- **A conversation is a chat mia desk draws.** `mia chat`, run in a pane
  of a worktree's session, follows the conversation a window of the
  worktree has open — `--window` says which, `--conversation` another —
  and says it to mia desk by its UI protocol: the view whole, then each
  new or grown turn as a patch, and whether the agent is at work. What is
  sent, chosen and stopped there is typed into the agent's window. A
  conversation no window has open is shown with no field to reply in.
  `mia api capabilities` lists `chat`.

- **A view follows a conversation as it is written.**
  `mia api conversation <id> --follow` and `mia api main turns --follow`
  print the turns in one line of JSON and keep running, a line more of
  only the new turns each time there are some, so a view no longer asks
  once a second. A turn that grew comes again under its `n`, to replace
  the one it has. A transcript is read on from where it stopped once its
  size or time changes; opencode's are exported every second. It ends,
  exit 0, when stdin ends, its reader goes, or on SIGINT or SIGTERM.

- **opencode's conversations are read as claude's, codex's and pi's
  are.** opencode keeps them in a database of its own, so mia asks
  opencode for them as JSON — `opencode api GET /api/session` for the
  list, `opencode session export <id>` for one — and only when it is
  installed and has a data folder. They are in `mia api conversations`,
  placed in the window that runs opencode in their worktree, read by
  `mia api conversation <id>` and `mia recall`, taken up again by
  `mia conversation resume <id>` as `opencode --session <id>`, and handed
  to another agent by `mia conversation continue <id> --with <agent>`.
  What a list read of each is kept, and one is exported again only once
  it has changed or while it is at work; a subagent's conversation is
  its parent's.

- **An app says what it is.** mia knew a window's program only where it
  was written into mia. A folder in mia's config folder,
  `apps/<name>/app.toml`, now declares an app: a `label`, an `icon`
  beside it, the `[[keys]]` worth showing, and under `[match]` how to
  tell it — `commands` its pane runs, or a `title` its pane has under a
  runtime's command in `via`. `mia api apps` lists them from any folder,
  saying on stderr each manifest it could not read, and a window in
  `mia api worktrees` and `mia window ls --json` has `app`, the name of
  the one it runs; told by its title alone, its `command` is that name
  too, as pi's is. An agent's window is never an app, and no manifest
  takes an agent's name. Capability `apps`. See [Apps](apps.md).

- **A conversation goes on with another agent.** No agent loads
  another's conversation, so `mia conversation continue <id> --with
  <agent> [--worktree <name>] [--dry-run]` writes it out as a handoff —
  who said what, each thing the agent did in a line, in order — in the
  repository's git folder,
  `.git/mia/handoffs/<worktree>/<agent>-<id>.md`, and opens a window in
  its worktree's session with the other agent, started with a model, an
  effort and an account as a window's is, and told as its first message
  to read the handoff and go on from where it stops. The conversation is
  said by its id, never guessed: a worktree can have several open, so
  none is "the last written to", and `--from <window>` in place of the
  id is the one conversation that window has open, or a refusal that
  lists the ids. A handoff is less than the conversation, and says so:
  what the first agent thought and what came back from its tools are not
  carried over, and one over 60,000 characters keeps the first thing
  asked and the last turns, with a line for the middle that names
  `…-full.md`, the file with every turn. With the same agent it is
  refused: that agent's own `mia conversation resume <id>` loses
  nothing. An agent that asks something first, or ends as it starts, is
  typed nothing, and the message is printed to say there. `--json` says
  `from`, the conversation, and `to`, the agent and its window;
  `--dry-run` writes and starts nothing. Each handoff is kept beside its
  file, and `mia api conversations` says the two conversations are one
  thread — `continues` on the one the other agent began, `continued_by`
  on the first — for a chat to read on from one into the other.
  Capabilities `conversation-continue` and `conversation-thread`. See
  [With another agent](conversations.md#with-another-agent).

- **pi says what it does.** Only claude told mia it was working, asking
  or done, through its hooks. `mia account signals pi [<name> | --all]
  [--dry-run]` writes an extension of mia's into pi's folder,
  `extensions/mia-signal.ts`, which runs `mia signal . busy`, `asks` and
  `done` from pi as those hooks do, `--from pi`: a run begun, an
  extension's dialog waiting for you, pi settled. It is written only
  when asked, begins with a line that says it is mia's, and a file of
  that name without that line is left as it is. Where there is no `mia`
  or no worktree of mia's it does nothing, and it never holds pi up.
  Capability `account-signals`. See [pi says what it
  does](tmux.md#pi-says-what-it-does).

- **An account has the agent's own setup.** Each account is a whole
  folder of the agent's, so a session on one started without the
  person's settings, the hooks that tell mia what claude does among
  them, CLAUDE.md and skills. `mia account add` and `mia account login`
  now link, before the agent starts, what claude's own folder has of
  `settings.json`, `CLAUDE.md`, `skills`, `commands`, `agents`,
  `plugins`, `output-styles`, `hooks`, `statusline.sh` and
  `statusline-command.sh` into the account's, and `mia account share
  <agent> <name>` or `--all` does it for those there already. Nothing
  else is linked or read: no sign-in, no `.claude.json`, no
  conversations or state. What an account has of its own under one of
  those names is kept; run again, only what is missing is linked. `mia
  api accounts` and `mia account ls` say what each `shared`; codex and
  pi share nothing yet. See [Its setup,
  shared](agents.md#its-setup-shared).

- **An account is signed in to again, and one is the user-wide default.**
  `mia account login <agent> <name> [--in <worktree>]` opens a window
  running the agent on an account it has, `default` too, for the person
  to sign in to it again with the agent's own login; `mia account add`
  of a name it has says so. `mia account use <agent> <name> --global`
  sets `[agents.<agent>] account` in the user-wide config, and `default`
  takes the line out; a rename or a removal of that account is followed
  there. `mia api accounts` says which is the user-wide one,
  `user_default`. `mia config --global agents.<agent>.<key>` reads and
  sets those keys. Capabilities `account-login` and
  `account-user-default`. See [An agent's
  account](agents.md#an-agents-account).

- **Roles: a setup with opening instructions.** A file of yours,
  `~/.config/mia/roles/<name>.md`, says in its front matter the agent,
  model, effort and account, all optional, and below them what to tell
  the agent first. `mia window new <worktree> --role <name>` and `mia
  main start --role <name>` start it and send it the body once it is
  ready, never into a question. `mia role ls`, `show` and `path` read
  them; `mia api setups` lists them after the other setups, with `role`,
  `body` and `account`. An agent mia does not know, or a model it has
  not, is refused before anything starts. Capability `roles`. See
  [A role](agents.md#a-role).

- **The API the apps were built on is kept.** `internal/cli/testdata/api`
  holds the shape of every `mia api` answer and the inputs its usage
  names, read off the Go types; a test fails, naming the field, when one
  goes, is renamed, changes type, may newly be left out or loses a value
  of a closed set, or when a query comes to need a new input. What is
  added passes and is kept with `-update`. No answer changes. See
  [Automation](api.md).

- **A machine that does not answer says what is wrong, and the fix.**
  Each machine in `mia api standing` and `mia api machines` has a
  `reach`: a `state` — `ok`, `unreachable`, `needs_login`, `no_mia`,
  `old_mia`, `not_set_up`, `failed` — a `detail` of one line and the
  `fix`, the one command that puts it right. `mia machine ls` shows it as
  standing last found it, and `mia doctor` says it of each machine it
  cannot use. One that is off or wants a login is asked once a minute
  after failing twice, not every 30 seconds. A Linux machine whose user
  services stop at logout warns of it, with `loginctl enable-linger`.
  Capability `machine-reach`. See [Standing](api.md#standing).

- **A new conversation for the main agent.** `mia main start --new`
  starts it going on with none; the one it had stays, and is listed and
  resumed as any other. One that runs is refused in a line that says
  what ending it would cut short, and `--new --restart` ends it and
  starts it again as the agent it was, with the model, the effort and
  the account it had. `mia api main` says the new `conversation`, and
  none until there is one. Capability `main-start-new`. See [A new
  conversation](main.md#a-new-conversation).

- **Accounts.** An agent can have several logins, each a folder of its
  own it keeps its sign-in and conversations in — claude's
  `CLAUDE_CONFIG_DIR`, codex's `CODEX_HOME`, pi's `PI_CODING_AGENT_DIR`,
  a plugin's `account_env`; its usual folder is `default`. `mia account
  add <agent> <name>` makes one and opens a window for the person to sign
  in with the agent's own login; mia never reads, copies or moves a
  sign-in. Each repository starts each agent on one — `mia account use`,
  `[agents.<name>] account`, or the default — and `--account` on `mia
  window new`, `mia conversation resume` and `mia main start` starts one
  on another, this once. Conversations are found in every account's
  folder and say their `account`; one resumed on another account is
  copied there, and `mia move` carries one into the account of the same
  name on the other side. `mia api accounts [--projects]`. See [An agent's
  account](agents.md#an-agents-account) and [Accounts](api.md#accounts).

- **Setups.** `mia api setups` is the agents, models and efforts a
  person has used, as one, each with how often, in how many worktrees and
  where last, the repository's usual one first, then the last used; what
  the config starts agents with comes after, for someone who has chosen
  nothing yet. Each start and switch with a model is written down for it.
  From any folder: in a repository, that one's; `--repository <name>` or
  `--all` for others. See [Setups](api.md#setups).

- **An agent's model.** `mia window new <worktree> <agent> --model <model>
  --effort <effort>` starts an agent with them, and `mia window model
  <worktree> <window> <model> [--effort <effort>]` switches one while it
  runs, typing what the agent takes — claude's `/model` and `/effort`,
  codex's `/model … --reasoning=…`, pi's `/model` and `/thinking`; one
  that cannot be switched so is refused, to be restarted with `--model`.
  `mia conversation resume <id> --model … --effort …` takes a conversation
  up again with them: how an app restarts an agent with another, once it
  is stopped. `mia main start --model …` and `mia main model …` do the
  same for the main agent, and `mia main start --agent <name>` starts it
  as another agent, this once, refused while one runs. `mia api agents`
  lists, from any folder, each agent's models and efforts, what the config
  starts it with and whether it can be switched while it runs;
  `[agents.<name>] model` and `effort`, and `[main] model` and `effort`,
  are what an agent is started with when nothing else is said. `mia api
  conversations` and `mia api main` give a running window the `model` and
  `effort` mia started it with or last switched it to. A plugin's agent
  says its own in its manifest. See [Agents](api.md#agents).

- **The main agent.** `mia main start` gives a person one agent to talk
  to, which runs the others: a session of its own, `mia-main`, in a folder
  mia makes under the user-wide config, with notes that say who it is and
  what it may answer. `mia main say <text…>` types a message into it,
  `mia main press 1` presses a key in it with no Enter, to pick an option
  when it is the one that asks,
  `mia main interrupt` stops it mid-work, `mia main stop` ends it; started
  again it goes on with its last conversation. `[main] agent` says which
  agent it is, a plugin's too, and `[main] machine` the machine it lives
  on: from any other, each of these is asked of that machine's mia over
  ssh. `mia api main` is where it stands and
  `mia api main turns` its conversation, each of mia's verbs it ran said
  as what it did: "Started goba on mia-desk · claude", "Told olasiti:
  rebase onto main", "Moved sakina to fedora". See [The main agent](main.md).

- **`mia api standing`** is every agent in every repository on the
  machine, and on each machine added: which wait on you, with the line
  that says why and the numbered options on its screen, which work, and
  which finished unseen. A machine that does not answer is named, not an
  error. `--seen <repository>/<worktree>` marks one seen.

- **Conversations.** `mia api conversations` lists the agent conversations
  of the repository, its main checkout and every worktree — one that was
  removed too — with what was first asked and last said, and whether a
  window has it open; `mia api conversation <id> [--after <n>]` reads one
  as turns for a chat view: what was asked and said, each tool in a line,
  questions and their answers, commits made. `mia conversation resume <id>
  [--in <worktree>]` opens a window with the agent taking it up again,
  where it was made or in another worktree; one copied there is listed
  once for each folder, and `--worktree <name>` reads or resumes the copy
  in one. All of it for claude, codex
  and pi, from their own transcripts, with nothing recorded as they work.

- **`mia recall` reads codex's and pi's conversations too**, and with
  `--json` says each one's `agent`, `worktree` and `n`, the turn it came
  up in.

- **A plugin can add an agent.** Its manifest declares the agent and what
  means it in a window, and mia asks the plugin for its conversations,
  their turns, a search, the resume command and the carry for a move, as
  it has them built in for claude, codex and pi.

- **`mia window key <worktree> <window> -- C-c`** presses a key by tmux's
  name for it — `C-c`, `Escape`, `Enter`, `Up` — so an agent or a dev
  server can be stopped, or a menu moved through, without closing the
  window or the session. `press` types its text as it is, and typed the
  letters `C-c`.

- **`mia window` takes `<worktree>@<machine>`**, as `mia run` and `mia
  shell` do: list, open, read, type into or close the windows of a worktree
  on another machine, answered by that machine's mia. It used to read the
  whole word as a window's name in the worktree it was run from, and
  `mia window new monduli@fedora` opened a window called that, here.

- **`mia move --dry-run`** reads the worktree and says what would cross —
  the branch, how many files staged, changed and new, what git ignores and
  so stays, the windows that would open — after checking the machine
  answers. Nothing is moved.

- **A move takes small ignored files along, and names what it leaves.**
  What git ignores never crossed, and went with the worktree when it was
  removed where it was, unsaid: a `.env` was lost. An ignored file of at
  most 1 MiB now crosses with its mode, and is not written over one the
  other side has differently. An ignored folder (`build/`,
  `node_modules/`) and an ignored file over 1 MiB stay, and `mia move`
  names them before it takes anything across.

- **Split windows come back split.** mia remembers every pane of a window,
  not only the one in front: `mia resurrect` and `mia move` open each in
  its folder with its program, laid out as it was. `mia move` takes an
  agent's conversation along from any pane, not only a window's first.

- **Editors and dev servers are kept as agents are.** A pane back at its
  shell keeps `nvim`, `vim`, `hx` or a dev server (`npm run dev`, `pnpm
  dev`, …) it ran, and `mia resurrect` starts it again; `keep = ["tail
  -f"]` in the config names more. What finishes — a build, a push — is
  never run again.

### Changed

- **A worktree with no session is asked `git status` every five
  minutes.** Whether a listing says it is `dirty` is asked of git again
  after thirty seconds for a worktree with a session (eight while its
  agent writes) and after five minutes for one with none, and at once
  for any whose index, `HEAD` or commit changed since: at rest, mia
  serve's look at everything every thirty seconds runs 60 `git status` a
  minute where it ran 148 over 118 worktrees, 30 with a session. See
  [mia serve](serve.md#get-events).

- **`mia serve` keeps what it was read.** `GET /api/<query>` gives a
  question read again what it answered, until something it is answered
  from changed as the server sees it for its listeners — looked at once
  when it is read, so what was just written or run is answered from — so
  a read again is a few milliseconds where it was the whole asking; a
  question of no repository is asked again when a repository's records
  are written, not its git, and mia desk's files in mia's folder change
  nothing. See [mia serve](serve.md#get-apiquery).

### Fixed

- **mia serve no longer stays stuck after `make install`.** A rebuilt
  server waited for every question it was asking again before it ended,
  and a fetch of a machine's branches that hung after ssh answered (no
  time limit) held it for good: nothing answered on the socket and every
  client fell back to starting mia. That fetch now gives up after 30 s,
  and an ending server lets go of what is still being asked after 5 s.

- **A window mia started is its agent's.** A conversation taken up
  again, handed over or continued is started by the session's shell,
  which is what tmux names in front of it: such a window was no agent's
  to `mia chat` — "no agent mia knows" over it — had no conversation in
  `mia api conversations`, and could not be switched. Its agent is now
  read from what `ps` says the shell runs, for a pane mia marked as an
  agent's, and it is no agent's again once the agent ends. So a
  conversation resumed twice in one worktree is the window that has it,
  not a second window.

- **The way to an account's window is pasted as it is.** `mia account
  add` said `tmux attach -t =mia-accounts goes there`, and zsh reads
  `=word` as the path of the command `word`: pasted, it said
  `mia-accounts not found`, as though the window were gone. It now says
  `tmux attach -t mia-accounts:<window>`, quoted when a shell would read
  anything in it, and `tmux switch-client -t …` from inside tmux.

- **A window tmux names is remembered with no name.** The name it had
  when mia looked was kept, and a session never looked at keeps the one
  tmux gave it as it started: `tmux`, which `mia resurrect --list`
  showed. Such a window now has `Name` `""` and `Auto` true, and is said
  by what runs in it, its label, or `shell`.

- **`mia dev --follow` costs nothing while nothing changes.** Each second
  it listed the worktree and main with git, both walks of the tree, and
  read every file of both to compare them: a follow left running burned
  most of a core. It now stats the files it copied and copies one whose
  stat changed, and lists the worktree only when a folder of it changed,
  or every 30 seconds. On 4,000 files: 35% of a core idle before, under
  2% now.

- **What is said to an agent arrives however long it is.** `mia main
  say`, a role's opening instructions and `mia window send` gave tmux
  the text on its command line, and tmux refused one over about 16 KB:
  "command too long". A paste now reads it from stdin, and what is typed
  goes 4 KB at a time.

- **A machine that hangs no longer holds up `mia api standing` or `mia api
  machines`.** ssh gave up only on a connection it could not make; one
  that connected and never answered held the answer for a minute, and an
  app that gave up first left the ssh running, one more each time it
  asked. A machine now has 8 seconds: one slower is `unreachable`, its ssh
  is killed, and it is not asked again for 30 seconds by any mia.

- **`mia signal ls` no longer lists a signal whose setter is gone.** It
  printed the signals file as it was, and what reads it — the Mac app
  between full answers, and before it approves an ask — showed a dead
  agent's `asks` and `busy` until the next full listing forgot them. It
  now goes by the rule `mia ls`, the radar and `mia api standing` go by,
  and forgets those it leaves out as `mia ls` does; a `--until seen`
  signal still outlives its process.

- **An ask from an agent killed in tmux no longer waits on you.** A
  signal set from a pane went only with the pane, and a pane outlives the
  agent killed in it, its shell left behind: claude's `asks` and `busy`
  stayed. `mia signal .` now records the process behind it in tmux too,
  and the signal goes when that process does, in `mia ls`, the radar and
  `mia api standing`. A `--until seen` signal such as `done` still
  outlives it; `TMUX_PANE=` emptied still records none; signals set before
  this go by their pane as they did.

- **Nothing typed into the main agent answers a question it asks.** In a
  folder it had not seen, claude asks whether you trust it, with
  **No, exit** chosen; `mia main say` and `mia main model` pasted and
  pressed Enter, the Enter answered No, and claude quit, and the next
  `mia main model` failed with `can't find window: claude`. Now `mia api
  main` says what it asks, as `asks`, and is not `working` then; `say` and
  `model` type nothing and say what it asks and how to answer, `mia main
  press <n>`, which moves the cursor where the agent takes no digit;
  `mia window send` into a worktree's window that asks is refused the
  same way. `mia main start` watches the agent for up to three seconds
  and says what it asks, or that it ended, with its last lines. One that
  ends as it is typed into is not running, as one stopped is. mia
  answers no question itself. See [The main agent](main.md#when-it-asks-first).

- **An ask from an agent outside tmux no longer waits on you after the
  agent is gone.** `mia signal .` run with no `TMUX_PANE` at all records
  the process behind it, the agent whose hook it is, and the signal goes
  when that process does, as one set from a pane goes with its pane: in
  `mia ls`, the radar and `mia api standing`. Signals set before this, and
  one set with `TMUX_PANE=` emptied, record none and stay as they did.

- **`mia api repos` runs no git.** It asked git about every worktree mia
  has named, one after another: most of a second with fifty of them. Each
  repository is now found from its files, in a hundredth.

- **A move carries claude's conversation from a path with an underscore
  or a space in it.** claude names a folder's conversations after its
  path with every character but a letter or a digit as a `-`; the carrier
  changed only `/` and `.`, and looked in a folder that was not there.

- **A session is remembered when it is made or changed.** Only the status
  line's look remembered sessions, so one made and used with no terminal —
  by the desk, the phone, a script — was never remembered, and `mia
  resurrect` knew nothing of it. `mia window new`, `close` and `open` and
  `mia new` now remember too, and so does mia just before it kills a
  session itself. A new worktree handed the name of a removed one starts
  with no session of that one's to bring back.
- **An agent that has said nothing is not said to be doing the machine's
  name.** Until claude titles its pane — at its prompt to trust a new
  folder — tmux titles it with the host's name, and `mia api worktrees`
  gave that as what the agent was doing: `"agent": "MacBookPro.lan"`.
- **`mia api stack` names each layer's worktree.** It left out `worktree`,
  which `mia stack show --json` gives for the same layer; the two are now
  one answer.
- **A fork is not listed clean with its work in it.** A listing that
  looked at a worktree while mia was still making it kept "clean" for up
  to 30 seconds after `mia fork`, `mia unarchive` or a move here had put
  uncommitted work into it. mia now drops what was kept about a worktree
  when it changes it; how long an answer is kept is as it was.
- **A move does not write over work already there.** When the branch was
  already checked out on the other side with uncommitted work — left by a
  move that stopped half way, or done there — its new files were
  overwritten by the ones arriving. The move now stops and says so, and
  the worktree it was moving is untouched.
- **`mia run <worktree>@<machine>` from a script is quiet.** Started with
  no terminal — by the desk, the phone, a script — it asked ssh for one
  anyway, and every call printed "Pseudo-terminal will not be allocated";
  a failing command's exit status was said twice.
- **`mia resurrect` of a removed worktree says it is gone.** It said
  "already running" and exited 0, and `--list` kept showing the session as
  closed until the next look.
- **`mia unarchive` and `mia fork` keep what was staged, staged.** Both
  brought the work back with nothing staged. The archive now keeps the
  index beside the files, as `git stash` does; archives made before this
  come back as they always did.
- **A move keeps what was staged, staged.** `mia move` took everything
  not committed across as one patch, so on the other side nothing was
  staged: a file added with `git add` arrived untracked, a `git rm`
  arrived as a deletion not staged. What is staged and what is changed
  beyond that now cross apart, and `git status` reads the same there.
- **A move from a Mac leaves no `._` files beside the conversation.**
  macOS tar packed an AppleDouble file for every file and folder of an
  agent's conversation, which Linux unpacked as `._<id>.jsonl` beside the
  real one, where the next move back could take it for the newest.
- **`mia api machines`, `here` and `capabilities` answer from anywhere.**
  Outside a repository they said "not inside a git repository", though
  none is about one: the machines and how busy each is are the same from
  any folder. A machine's worktrees are still listed only from inside the
  repository they belong to.
- **A session is remembered while you work in it.** mia's sessions show
  nothing on the right of their status line, and so never ran the look
  that remembers them: what `mia resurrect` brought back was the session
  as it stood when the radar last opened. They now run it unseen.
- **A brought-back window is named by tmux again.** `mia resurrect` and
  `mia move` gave every window its old name for good — a claude's version
  number, `zsh` — and tmux stopped naming it. Only a window you named
  keeps its name now.

- **`mia resurrect` brings back a claude that ended before tmux did.** A
  window back at its shell keeps the claude it ran, and the conversation,
  until it runs something else; before, the next look forgot it and the
  session came back as a bare shell. What it opens it now says: `brought
  back <worktree>: …`.
- **Every agent goes on where it was, not only claude.** codex (`resume
  --last`), pi, opencode and cursor-agent (`--continue`) each continue
  their folder's last conversation, are known behind node or by a version's path as `ps` shows them, and are kept
  by a window back at its shell as a claude is.
### Removed

- **gemini is no longer an agent mia knows.** Gemini CLI is gone; Google's
  agent is now agy, which mia starts, reads and switches as before. A
  `[agents.gemini]` in a config is an agent mia does not know, as any
  other name is.


## v0.1.0 — 2026-11-09

### Added

- **`mia new --name <name>`** picks the worktree's name, and
  **`mia rm --branch`** deletes the branch too once the base holds it.
- **Environments run as isolated Compose projects**, with `mia doctor`,
  `mia env up --wait --recreate`, `mia env down --all`, and remote sync
  that is safe: secrets stay behind unless `sync_secrets` says otherwise,
  `mia env sync … preview|conflicts|restart` show and settle it, and a
  change to the ignore rules asks for a restart instead of syncing wrongly.
- **`mia stack sync`** fetches the base, forgets landed layers and restacks
  the rest; mia still never pushes. **`mia gc --merged`** collects clean
  worktrees whose branch the base already holds.
- **Shared project settings** in a committed `.mia.toml`, and
  `mia config show|validate` for the configuration in effect.
- **`mia api capabilities`**: the API's version, queries and features, and
  `--json` on the rest of the commands scripts read.
- **The gateway browses apps directly over HTTP**: `mia env browse --direct`.
- **`mia new <branch> --from <ref>`**: start the new branch at a commit,
  tag or branch instead of base. A branch that is only on origin, such as a
  pull request to review after `git fetch`, now starts at origin's branch
  and tracks it, rather than as a new branch from base.
- **`mia ls` shows each worktree's best signal** at the end of its row:
  `note: tests green, ready for review`. A signal set with `TMUX_PANE=`
  emptied keeps no pane, so a note an agent leaves outlives the agent.
- **`mia new --bg`**: the worktree is there at once; `setup` runs in a
  `setup` window of its session, its output kept with a shell under it. A
  setup that fails leaves `setup: failed` on the worktree's row.
- **A `poll` event for plugins**, every 30 seconds or so, for one that
  looks outside mia. The `gh` plugin uses it: each worktree's pull request
  shows on its row as `ci failed`, `changes asked` or `approved`, or quietly
  as checks running. `mia plugin enable gh` to have it.
- **`mia fork [worktree] <branch>`**: a second try beside the first: a new
  worktree on a new branch from where the worktree is, its uncommitted and
  untracked work copied, the first left as it is.
- **`mia archive <worktree>`** and **`mia unarchive <name>`**: put a
  worktree away without committing or losing anything; its uncommitted and
  untracked work is kept as a commit under `refs/mia/archive/`, never
  touching the stash, and comes back uncommitted, under its old name.
  `mia archive ls` lists them; the radar offers it on a worktree.
- **`mia recall <text…>`**: which conversation was that? The claude
  conversations of the repository and its worktrees that mention it, newest
  first, your own words over claude's, each with its `claude --resume` line.
- **The diff in the radar's look**: `?` then `→` past the session's
  windows shows the worktree against where it left base, uncommitted work
  included, the stat first.
- **`prefix a`**: straight to the next worktree that needs you, without
  opening the radar; again for the one after. Run `mia tmux install` once
  more to get the key.
- **`mia why <file:line|commit>`**: the claude conversation that made a
  commit, or the line's last commit: what you asked before it, and the
  `claude --resume` line to go back to it. It reads claude's own
  transcripts, so nothing is recorded when committing, and it works on
  history made before it existed.
- **`mia resurrect [--dry-run] [--list] [<worktree>...]`**: after tmux or
  the computer went down, each session opens again with its windows in
  their folders and their programs restarted; each claude resumes the very
  conversation it had (`--resume <id>`, else `--continue`), a bare codex
  runs `codex resume --last`. Every session is remembered: one you closed
  stays closed until you name it, `mia resurrect <worktree>`, and
  `--list` shows them all — running, down or closed — and when each was
  last seen. The radar puts what tmux took down first, one row brings it
  all back, and a worktree's actions bring its own session back; the tmux
  menu's `b` does the same.
- **Answer from the radar**: on a row under needs you, `^y` approves and
  `^r` types a reply into the pane the signal came from, without switching.
  `mia signal .` now keeps the tmux pane it ran in, and `--yes <keys>` says
  what approving presses. See [mia in tmux](tmux.md#signals) for the hook.
- **`mia shell --view <name>`**: attach as a view of your own, at this
  terminal's size, its current window yours to choose.
- **The radar** (`prefix space`): one popup to find any worktree on any
  machine, switch to it, make one, or run a plugin's verb. Worktrees that
  need you come first; idle ones and ones mia did not make fold away. `?`
  shows the selected one's session live, `^d` removes it, `^x` clears its
  signal.
- **`prefix m`**: mia's menu, with the plugins' verbs the `menu` setting
  names.
- **`mia signal`**: anything outside mia (an agent's hook, a test watcher,
  you) can say something about a worktree, shown as its setter asks.
- **`mia move <worktree> <machine|local>`**: a worktree goes to another
  machine and back with its uncommitted work and windows, keeping its name
  when it is free there.
- **`mia gc --others [--apply|--ask]`**: the clean worktrees mia did not
  make, such as agents' own; `mia gc --ask` removes idle ones on `y`.
- **`mia config <key> [<value>]`**: say or set any one setting, comments
  kept.
- **`mia new --on <machine>`**: make the whole worktree on that machine.
- **`mia api`**: `repos`, `here`, and machines reachable through `run`.

### Changed

- **Sessions are found when mia runs without a locale.** Under launchd
  or systemd, as the phone's server runs it, tmux printed the tabs in
  mia's pane listing as `_`, so mia saw no session and every worktree
  looked idle. Every tmux client mia starts now passes `-u`.
- **A listing makes one `git status` per worktree, not two,** when the
  untracked check is due.
- **Plugins' manifests are read once, not on every call.** mia kept
  running every plugin's `manifest` on each poll, event and radar —
  about 0.2s; it keeps each answer while the plugin's file is unchanged,
  and asks again when it is rebuilt.
- **A restack is due only when a layer falls behind the layer under it.**
  The bottom layer being behind the base — `staging` moved on, as it always
  does — no longer lights `⟳ restack due` on the radar, the status line or
  the phone; `mia stack` and a layer's row say `staging moved on` instead,
  and `mia stack restack` still takes it.

- The radar shows, under each worktree, what its claude says it is doing:
  the summary claude keeps in its pane's title. The look shows it too.
- **active** is in the order worktrees last moved, newest first, and the
  age on a row is how long since anything happened in it, a commit or its
  windows, not only the last commit.
- A claude window is called `claude`, not after the version its installer
  names the binary for (`2.1.316`).
- A signal set from a tmux pane goes when that pane does.
- `mia recall` reads the newest transcripts first and stops once it has
  enough, and a subagent's prompt is no longer shown as what you said.
- A signal ranked above 50, such as claude's `busy`, is a word on its row
  under active, not a call for you.
- `mia dash` is the radar under its old name.
- A machine hears what this computer has every 30 seconds while you are
  away, not once.

### Removed

- The full-screen dashboard; the radar, signals and plugins' verbs do its
  work.
- mia no longer pushes or opens pull requests; those are yours.
