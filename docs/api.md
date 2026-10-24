# Automation

`mia` commands are the interface for scripts, `mia-ui` and plugins. Read
commands that support structured output take `--json`; `mia api` always emits
JSON.

## Discover the interface

```bash
mia api capabilities
```

The answer names the non-panel API version, available queries and feature
markers introduced by newer builds:

```json
{
  "version": 1,
  "queries": ["capabilities", "worktrees", "dashboard"],
  "features": ["direct-http", "project-config", "doctor"]
}
```

The actual answer includes every query and feature, not this shortened list.
Check capabilities before using a feature. Existing response shapes are not
wrapped in a version envelope. Panels keep their own `version` field for the
panel schema.

## JSON commands

| command | returns |
|---|---|
| `mia api <query>` | worktrees, panels, environments, stacks and base |
| `mia config show --json` | effective global, project and local configuration |
| `mia doctor --json` | every check and an overall `ok`; failures still exit non-zero |
| `mia gateway status --json` | `running`, `address`, `pac`, `ca`, `trust`, `differentBuild`, `routes` |
| `mia env host [worktree] --json` | `worktree`, `machine`, and remote `staging` when present |
| `mia dev --json` | whether main is lent, and its `worktree`, `path`, `stashed` |
| `mia env sync <worktree> status --json` | sync session, problems and conflict count |
| `mia env sync <worktree> conflicts --json` | the same session with `conflictPaths` |
| `mia env sync <worktree> preview --json` | ignore rules and secret paths excluded or allowed |

`mia dev <worktree> --json` reports the new lent state; `mia dev off --json`
reports `{"lent": false}`. `--follow` cannot be combined with `--json`.

Examples of placement and gateway state:

```json
{"worktree":"monduli","machine":"build","staging":"/home/me/.mia/shop/monduli"}
```

```json
{"running":true,"address":"127.0.0.1:7842","pac":"http://127.0.0.1:7842/proxy.pac","ca":"/home/me/.config/mia/gateway/ca.pem","routes":["monduli.mia"]}
```

## Exit status

A command that runs another command preserves its exit status where possible,
including `mia run`, `mia env run`, `mia env exec` and interactive shells.
Mia itself uses `1` for failure, `3` for a missing worktree or query and `64`
for usage errors.

## Plugin name collisions

A verb declared by two enabled plugins is refused until all but one are
disabled. A verb that shadows a core command is also reported by
`mia plugin ls` and `mia doctor`. Dashboard key clashes remain visible on the
dashboard and can be resolved with `[keys]` bindings.
