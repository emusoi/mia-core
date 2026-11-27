# mia in tmux

How mia runs entirely in tmux, with mia desk not needed. This is what is built, as of 2026-11-27.
`docs/commands.md` has every command; this page is the shape of it.

## The idea

A worktree is a tmux session, a tab is a tmux window, an agent is a program in a window. mia adds
a status line, one list, a popup terminal and commands. It draws no screens of its own.

## One key, one list

`prefix k` (⌘K) opens the list. What you type says what Enter does:

| Typed | Enter |
|---|---|
| nothing | who waits for you (their answers numbered), who works, then the projects, the one here first |
| `project/` | that project's worktrees, those on a machine too |
| a name | goes to that worktree or project, in any project |
| a digit | answers whoever has waited longest |
| `pi fix the test` | starts that agent here on that task (a second one is `pi-2`) |
| `> codex` | moves this conversation to another agent |
| `> claude opus`, in claude's window | that agent's model switched where it runs |
| `say <words>` | types them and Enter to whoever waits, the longest first |
| `@` | moves this worktree to a machine, or brings one back |
| `new <branch>` | makes a worktree here and goes to it |
| `new <branch> on <machine>` | makes it on that machine |
| `fork <branch>` | forks this worktree and goes to it |
| `rm <name>` | removes one, refusing what `mia rm` refuses |
| `clean` | removes the idle worktrees, and the clean ones mia did not make |
| `clear` | clears a worktree's signal |
| `back` | brings back the sessions that went down |
| `stop` | stops the agent in this window |
| `stack` | the layers of the stack here |
| `nvim`, `git`, `shell` | opens the editor, lazygit or a shell in a window |
| a verb of the config's `menu` | runs that plugin's verb in a window |
| any other words | past conversations that mention them, in every project and on every machine; Enter resumes |
| `?` | says all of this |

## The other keys

| Prefix key | ⌘ key | Does |
|---|---|---|
| `k` | ⌘K | the list |
| `K` | ⌘O | the popup terminal, in the folder you are in (`mia cd` there jumps and closes it) |
| `a` | ⌘J | the next that needs you |
| `b` / `B` | ⌘[ / ⌘] | back and forward through where you have been |

The radar (`Space`, ⌘⇧J), the new-worktree form (`n`, ⌘N), mia's menu (`m`) and this window's
menu (`e`, ⌘.) are gone: the list does what they did. Their user keys stay in their places, bound
to nothing, so a terminal app's lines keep their numbers.

`mia tmux install` writes one marked block in the tmux config; `uninstall` removes it.
`mia tmux keys kitty|ghostty` prints the terminal app's lines for the ⌘ keys. They are not in the
kitty config yet: they would replace kitty's own ⌘T, ⌘W, ⌘K and ⌘1–9.

## Past conversations

- `mia recall <words>` looks in the project here; `--all` in every project and on every machine.
  A small index per repository (`.git/mia/cache/recall.gob`) makes it answer in a fraction of a second.
- `mia mcp` gives agents the same search (`recall`) and a way to read one (`conversation`).
  Add it to an agent yourself, e.g. `claude mcp add mia -- mia mcp`.
- A conversation whose worktree was removed is still found and read, and says it cannot be resumed.

## Machines

- `mia machine ship <name>` puts this mia on a machine.
- `mia move <worktree> <machine>` and `mia move <worktree>@<machine> local` take a worktree across:
  its branch, its uncommitted changes, its windows, and the conversation of each agent in them.
  It keeps its name and its folder's name.
- From the list, a worktree or conversation on a machine opens as that machine's session in a
  window here, named `worktree@machine`.

## Not done

- Carrying a claude or codex conversation between machines is untested (pi is proven).
- Carrying one conversation without its worktree.
- The ⌘ keys in kitty (see above).
- Deciding whether desk is deleted or kept as a thin window around tmux.
