# Apps

A window runs a shell, an agent, or some other program: lazygit, a monitor,
something of your own. mia knows the agents. Any other program can say
what it is with a manifest, an `app.toml`. A window that runs it is then
named as that app in `mia api worktrees` and `mia window ls --json`, so an
app reading them can show it by its name and icon.

Each app is a folder in mia's config folder,
`~/.config/mia/apps/<name>/app.toml` (under `$XDG_CONFIG_HOME/mia` when that
is set). The folder's name is the app's name.

```toml
# ~/.config/mia/apps/lazygit/app.toml
label = "Git"                 # shown in place of the name
icon = "icon.svg"             # a file beside app.toml

[match]
commands = ["lazygit"]        # the pane's foreground command is one of these

[[keys]]                      # keys worth showing beside it; mia only passes them on
key = "c"
label = "commit"
```

| key | means |
|---|---|
| `label` | the name to show in place of the folder's; optional |
| `icon` | a file in the app's folder, by a path relative to it; optional, and left out when the file is not there |
| `[match] commands` | the pane's foreground command is one of these |
| `[match] title` | a regular expression the pane's title matches, tried only when the command is one of `via` |
| `[match] via` | the commands a runtime shows as, such as `node` or `python3`, under which the title alone names the app |
| `[[keys]]` | each a `key` and a `label`: hints for an app to show; optional |

`[match]` has to say at least one way to tell the app: `commands`, or
`title` with `via`.

A program that runs under a runtime has `node` for its command, and only its
title says which program it is:

```toml
# ~/.config/mia/apps/notes/app.toml
[match]
title = "^notes"
via = ["node", "python3"]
```

## What a window says

A window whose pane matches says `"app":"<name>"`. One told by its title
under a `via` command is called by the app's name: its `command` becomes
the name, and so does the window's name when it was only the runtime's. One
told by `commands` keeps its `command`. When two apps match a pane, a title
under `via` wins over a command, and then the first by folder name.

An agent is never an app. A window mia knows as claude, codex, pi or any
other agent stays that agent, and a folder named as an agent is left out.

## Reading them

Manifests are read each time mia runs, so a new one needs nothing
restarted; `mia serve` reads them again when one is written. `mia api apps`
lists them, and says on stderr each one it left out: TOML that does not
parse, a `[match]` with neither `commands` nor `title`, a `title` with no
`via`, a `title` that is no regular expression, or a folder named as an
agent. See [Automation](api.md#apps) for the answer.
