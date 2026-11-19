# Apps

A window is a shell, an agent, or some other program: lazygit, a monitor,
something of your own. mia knows the agents. Any other program says what it
is with a manifest, and `mia api worktrees` then names it in the window, for
an app such as mia desk to show it by its name and icon.

A manifest is one folder per app in mia's config folder,
`~/.config/mia/apps/<name>/app.toml` (under `$XDG_CONFIG_HOME/mia` when that
is set). The folder's name is the app's name.

```toml
# ~/.config/mia/apps/lazygit/app.toml
label = "Git"                 # shown in place of the name
icon = "icon.svg"             # a file beside app.toml

[match]
commands = ["lazygit"]        # the pane's foreground command is one of these

[[keys]]                      # hints an app shows; mia only passes them on
key = "c"
label = "commit"
```

`label`, `icon` and `[[keys]]` may be left out. `[match]` has to say at
least one way to tell the app:

| key | means |
|---|---|
| `commands` | the pane's foreground command is one of these |
| `title` | a regular expression the pane's title matches, tried only when the command is one of `via` |
| `via` | the commands a runtime shows as — `node`, `python3` — under which the title alone names the app |

A program that runs under a runtime has `node` for a command, and only its
title says which program it is:

```toml
# ~/.config/mia/apps/notes/app.toml
[match]
title = "^notes"
via = ["node", "python3"]
```

A window whose pane matches says `"app":"<name>"`. One told by its title
under a `via` command is called by the app's name too: its `command`
becomes the name, as pi's does, and so does the window's name when it was
only the runtime's. One told by `commands` keeps its `command`. Where two
apps match a pane, a title under `via` is taken before a command, and then
the first by name.

An agent is never an app: a window mia knows as claude, codex, pi or any
other agent is left as it is, and a manifest in a folder named as an agent
is ignored.

Manifests are read each time mia runs, so a new one needs nothing restarted.
`mia api apps` lists them, and says on stderr each one it could not read —
TOML that does not parse, a `[match]` with neither `commands` nor `title`, a
`title` with no `via`, or a `title` that is no regular expression. See
[Apps](api.md#apps) for the answer.
