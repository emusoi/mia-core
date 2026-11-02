# Changes

What each release adds, changes and takes away, newest first. Work lands
under **Unreleased**. A release renames that heading to its tag and date,
`## v0.2.0 — 2026-11-09`, and the same section becomes the release's notes
on GitHub; a tag without its section is not released.

## Unreleased

### Added

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
- **`mia resurrect [--dry-run]`**: after tmux or the computer went down,
  each session opens again with its windows in their folders and their
  programs restarted; claude continues its last conversation
  (`--continue`), a bare codex runs `codex resume --last`. A session you
  closed while tmux kept running stays closed.
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

- A signal ranked above 50, such as claude's `busy`, is a word on its row
  under active, not a call for you.
- `mia dash` is the radar under its old name.
- A machine hears what this computer has every 30 seconds while you are
  away, not once.

### Removed

- The full-screen dashboard; the radar, signals and plugins' verbs do its
  work.
- mia no longer pushes or opens pull requests; those are yours.
