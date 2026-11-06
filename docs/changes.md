# Changes

What each release adds, changes and takes away, newest first. Work lands
under **Unreleased**. A release renames that heading to its tag and date,
`## v0.2.0 — 2026-11-09`, and the same section becomes the release's notes
on GitHub; a tag without its section is not released.

## Unreleased

### Added

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
