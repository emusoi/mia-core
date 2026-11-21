# mia serve

One server on this machine that answers what [`mia api`](api.md) answers,
tells whoever listens when an answer changes, and runs a verb for whoever
asks. It holds no truth: every answer is read from the same records, git
and tmux that `mia api` reads, and killing it loses nothing.

```bash
mia serve        # in the foreground, until ^C or SIGTERM
```

It says where it listens on stderr and runs until it is told to stop. A
second one is refused while the first answers, naming the first's process
id; a socket nobody answers on, left by one that was killed, is taken. It
is in no tmux, whatever it was started from: it reads the tmux server a
plain terminal would.

Each repository it is asked about is kept open between questions, and
opened again when its configuration, the person's configuration or what
another mia cached was written since, or after a minute. What a question
costs is mostly the git and tmux it asks — `mia api worktrees` runs git
in each worktree every time — so one question asked of the server costs
about what it costs `mia api` alone. What the server saves is the asking
again: [events](#get-events) ask once for every listener, and again only
when something an answer is read from changed.

## The socket

The server listens on a unix socket in mia's config folder, beside the
gateway's files:

```
~/.config/mia/serve/serve.sock          # $XDG_CONFIG_HOME/mia/serve/serve.sock when that is set
```

The folder is `0700` and the socket `0600`: the person's alone. That is
all the authentication there is. It speaks HTTP/1.1; any client that can
dial a unix socket can ask it:

```bash
curl --unix-socket ~/.config/mia/serve/serve.sock http://mia/version
```

The host in the URL is not read.

## mia api asks it

`mia api <query>` asks the server when one answers on the socket within
400 ms and it is the same build of mia — the same program file, by path,
size and modification time — and prints what it answered: the same bytes,
the same words on stderr and the same exit code. Otherwise it reads for
itself, as it always did:

- no server runs, or the socket is one a killed server left;
- the server is another build — after `make install`, until it is started
  again — which may answer what this one would not;
- the answer was cut off;
- the query has `--follow`, which is a stream ([events](#get-events) have it);
- `MIA_NO_SERVE` is set and not empty.

```bash
MIA_NO_SERVE=1 mia api worktrees    # never asks the server
```

Nothing else in mia asks the server. What the server runs, a verb or a
plugin, has `MIA_NO_SERVE` set and reads for itself.

The server answers from its own environment, not the asker's: `PATH`,
where plugins and agents are found, the tmux server, which it finds by
`TMUX_TMPDIR`, and where an agent keeps its conversations when a variable
says so — `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `PI_CODING_AGENT_DIR` — are
its own. Run it with the environment your shell has; a
[service](#as-a-service) is written with your `PATH`, and with any of
those your shell sets.

## Endpoints

| endpoint | answers |
|---|---|
| [`GET /version`](#get-version) | the build that serves, the API version and the process id |
| [`GET /api/<query>`](#get-apiquery) | what `mia api <query>` prints, typed in a folder |
| [`GET /events?q=…`](#get-events) | server-sent events: each question's answer, and again each time it is another |
| [`POST /run`](#post-run) | a verb run by mia, and what it printed, said and exited with |

Any other path is `404`.

### GET /version

```bash
curl --unix-socket ~/.config/mia/serve/serve.sock http://mia/version
```

```json
{"build":"/home/user/.local/bin/mia 16710802 1795302025695053983","api":1,"version":"dev","pid":61963}
```

`build` is the program that serves, its path, size and modification time:
`mia api` asks only a server whose `build` is its own. `api` is the
version [`mia api capabilities`](api.md#discover-the-interface) says.

### GET /api/&lt;query&gt;

```
GET /api/<query>?dir=<folder>&arg=<word>&arg=<word>…
```

`<query>` is what follows `mia api`, `dir` the folder it would be typed
in, and each `arg` one word after the query, in order — flags too. The
body is the bytes `mia api` prints, its exit code is the `Mia-Exit`
header, and what it says on stderr, when it says anything, is the
`Mia-Stderr` header, form-escaped. The status follows the exit code:
`0` is `200`, `3` (not found) is `404`, `64` (usage) is `400`, any other
`500`.

```bash
curl -i --unix-socket ~/.config/mia/serve/serve.sock \
  'http://mia/api/worktrees?dir=%2Fhome%2Fuser%2Fsrc%2Fapp&arg=longido'
```

```
HTTP/1.1 200 OK
Content-Type: application/json
Mia-Exit: 0

[
  {
    "name": "longido",
    "path": "/home/user/src/app.longido",
    "branch": "feature",
    "dirty": false,
    "session": false,
    "adopted": true,
    "where": "local",
    "last_work": "2026-11-21T18:00:26-05:00",
    "last_subject": "first"
  }
]
```

A query that fails is answered as `mia api` fails:

```
GET /api/stack?dir=%2Fhome%2Fuser%2Fsrc%2Fapp&arg=nowhere

HTTP/1.1 404 Not Found
Mia-Exit: 3
Mia-Stderr: mia%3A+no+worktree+called+%22nowhere%22+%E2%80%94+%60mia+ls%60+shows+what+exists%0A
```

A query with `--follow` is `400`, saying that `GET /events` has it.

### GET /events

```
GET /events?q=<question>&q=<question>…
```

Each `q` is what follows `/api/` in a read — `base?dir=%2Fhome%2Fuser%2Fsrc%2Fapp`
— escaped once more as a query value. The answer is a stream of
[server-sent events](https://html.spec.whatwg.org/multipage/server-sent-events.html),
`Content-Type: text/event-stream`. Every event is named `result` and its
`data` is one line of JSON:

| field | is |
|---|---|
| `q` | the question, as it was asked |
| `code` | what `mia api` exits with for it |
| `result` | what it prints, as JSON; left out when it prints nothing or what it prints is not JSON |
| `stderr` | what it says beside that; left out when it says nothing |

`data` is JSON as Go writes it: an `&` in `q` is `\u0026`.

```bash
curl -N --unix-socket ~/.config/mia/serve/serve.sock \
  'http://mia/events?q=base%3Fdir%3D%252Fhome%252Fuser%252Fsrc%252Fapp&q=stack%3Farg%3Dnowhere%26dir%3D%252Fhome%252Fuser%252Fsrc%252Fapp'
```

```
event: result
data: {"q":"base?dir=%2Fhome%2Fuser%2Fsrc%2Fapp","code":0,"result":{"branch":"main"}}

event: result
data: {"q":"stack?arg=nowhere\u0026dir=%2Fhome%2Fuser%2Fsrc%2Fapp","code":3,"stderr":"mia: no worktree called \"nowhere\" — `mia ls` shows what exists\n"}

```

and, after the repository's `base` is set to `feature`, only:

```
event: result
data: {"q":"base?dir=%2Fhome%2Fuser%2Fsrc%2Fapp","code":0,"result":{"branch":"feature"}}

```

Each question's answer comes once, then again only when its bytes are
another; a question that fails is an answer too. Two listeners with the
same question share one asking, and a question nobody listens to is not
asked. The server asks again:

- every question, at once, when tmux says a session or a window came,
  went, was renamed or put in front — it hears this as a control-mode
  client on a session of its own, `_mia-serve-<pid>`, and starts no tmux
  server to hear it;
- every question, at once, after a [`POST /run`](#post-run);
- a repository's questions, and those of no repository, when its
  records, its configuration, or a worktree's `HEAD` or index were
  written, looked at each second; every question when the person's
  configuration or the main agent's records were;
- every question every thirty seconds.

A question with `--follow` — `conversation <id> --follow`,
`main turns --follow` — is the stream `mia api … --follow` prints, an
event for each line, its `result` the turns that are new or grew.

A listener that does not read as fast as it is told is let go: the stream
ends, and a new `GET` starts again from each answer as it is now.

### POST /run

```
POST /run
{"args":["<verb>","<word>",…],"dir":"<folder>","stdin":"<what it reads>"}
```

The verb is run by mia itself — the program that serves — as a command of
its own in `dir`, with `stdin` as what it reads, and the answer is what it
printed, said and exited with. Then every question the listeners hold is
asked again, so they hear what it changed.

```bash
curl --unix-socket ~/.config/mia/serve/serve.sock http://mia/run \
  -d '{"args":["ls"],"dir":"/home/user/src/app"}'
```

```json
{"code":0,"stdout":"  app      main                         \n  longido  feature                      \n","stderr":""}
```

A verb that fails is `200` too, with its exit code in `code`. A body that
is not JSON is `400`.

The server has no terminal, so a verb that needs the one it is typed in
is refused, `403` with `code` `64` and why, and nothing of it is done:

```json
{"code":64,"stdout":"","stderr":"mia: `mia chat` is drawn on the terminal it is typed in: mia serve has no terminal — run it there\n"}
```

| refused | because it |
|---|---|
| `chat`, `git`, `files`, `review`, `radar`, `dash`, `tmux menu`, `tmux next` | is drawn on the terminal it is typed in |
| `shell`, `away`, `window open`, `windows open`, `machine attach`, `machine shell`, `runtime attach`, `runtime shell`, `env shell`, `new --shell` | attaches the terminal it is typed in |
| `config` with nothing after it | opens an editor in it |
| `gc --ask`, `resurrect --ask`, `gateway trust`, `gateway untrust` | asks it something |
| `gateway serve`, `api … --follow`, `dev … --follow` | never ends — [`GET /events`](#get-events) follows a query |
| `serve` | is the server |

## As a service

There is no `mia serve install` yet: a service is written by hand. Each
runs `mia serve` with your `PATH`, starts it at login and starts it again
when it ends. After `make install` it is another build, which `mia api`
does not ask until the service is started again.

### macOS: launchd

`~/Library/LaunchAgents/mia.serve.plist`, with `which mia` and your
`echo $PATH` written in:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>mia.serve</string>
  <key>ProgramArguments</key>
  <array><string>/Users/user/.local/bin/mia</string><string>serve</string></array>
  <key>EnvironmentVariables</key>
  <dict><key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string></dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardErrorPath</key><string>/Users/user/Library/Logs/mia-serve.log</string>
</dict>
</plist>
```

```bash
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/mia.serve.plist   # install and start
launchctl kickstart -k gui/$(id -u)/mia.serve                              # start it again, after make install
launchctl bootout gui/$(id -u)/mia.serve                                   # stop and uninstall
rm ~/Library/LaunchAgents/mia.serve.plist
```

### Linux: systemd

`~/.config/systemd/user/mia-serve.service`:

```ini
[Unit]
Description=mia serve

[Service]
ExecStart=/home/user/.local/bin/mia serve
Restart=always
RestartSec=2
Environment=PATH=/home/user/.local/bin:/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now mia-serve     # install and start
systemctl --user restart mia-serve          # start it again, after make install
systemctl --user disable --now mia-serve    # stop and uninstall
rm ~/.config/systemd/user/mia-serve.service
loginctl enable-linger $USER                # optional: keep it running while you are logged out
```

## Other machines, later

Not built: the server answers this machine only. Another machine will reach
it the way mia already reaches a [machine](environments.md#other-machines):
over the ssh connection mia keeps to it, one master per machine at
`~/.config/mia/ssh/%C` (`ControlMaster=auto`, `ControlPersist=5m`).
OpenSSH forwards a unix socket to a unix socket, and adds a forward to a
master that runs without opening another connection:

```bash
ssh -o ControlPath=~/.config/mia/ssh/%C -O forward \
  -L ~/.config/mia/serve/fedora.sock:/home/user/.config/mia/serve/serve.sock user@fedora
```

`user@fedora` is the machine's target as `mia machine add` was given it,
so `%C` names the master mia already has. Then `~/.config/mia/serve/fedora.sock` here is the server there, spoken to
as above, with `dir` a folder on that machine. Only the person who can ssh
to the machine reaches it, as the person who can open the socket reaches
it here; nothing listens on a port.
