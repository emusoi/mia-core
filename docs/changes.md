# Changes

What each release adds, changes and takes away, newest first. Work lands
under **Unreleased**. A release renames that heading to its tag and date,
`## v0.2.0 — 2026-11-09`, and the same section becomes the release's notes
on GitHub; a tag without its section is not released.

## Unreleased

### Added

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

### Fixed

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
  --last`), pi, opencode and cursor-agent (`--continue`) and gemini
  (`--resume latest`) each continue their folder's last conversation, are
  known behind node or by a version's path as `ps` shows them, and are kept
  by a window back at its shell as a claude is.

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
