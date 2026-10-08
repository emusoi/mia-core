#!/bin/sh
# Build a disposable lab to test mia against.
#
# EVERY live test runs here and nowhere else. Testing against a real project
# means a bug writes into work somebody cares about, and even a read-only run
# reports on repositories that are none of the test's business.
#
# Makes, under a fresh temp directory:
#   lab/service          a Python project with a dev server, worktrees, commits
#   lab/web              a TypeScript project, so image discovery is exercised on
#                        both stacks rather than on whichever one was handy
#   lab/service-extra    a SIBLING whose name starts with the first one's, so
#                        matching a repository by path prefix is caught
#
# Prints the lab path. Remove it with `rm -rf` when you are done; nothing in it
# is precious.
set -e

lab=$(mktemp -d)/lab
mkdir -p "$lab"

start_repo() {
	root="$lab/$1"
	mkdir -p "$root"
	cd "$root"
	git init -q -b main
	git config user.email lab@example.invalid
	git config user.name "mia lab"
}

make_python_repo() {
	start_repo "$1"

	cat > pyproject.toml <<'PY'
[project]
name = "service"
version = "0.1.0"
requires-python = ">=3.11"
PY
	cat > server.py <<'PY'
"""A dev server with nothing mia-specific in it — the point of the lab.

It binds the port it was written for, prints what Host it was handed, and
streams, so the gateway's port passthrough, Host rewriting and unbuffered
responses are all exercised by an app that has never heard of mia.
"""

import http.server
import time

PORT = 5173


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/stream":
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            for i in range(3):
                self.wfile.write(f"data: {i}\n\n".encode())
                self.wfile.flush()
                time.sleep(0.2)
            return
        body = (
            f"served {self.path} · Host={self.headers.get('Host')} "
            f"· XFH={self.headers.get('X-Forwarded-Host')}"
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


if __name__ == "__main__":
    http.server.HTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
PY
	# A TestCase, not a bare test_ function: the check runs stdlib unittest so the
	# container needs nothing installed, and unittest only collects TestCase
	# subclasses — a pytest-style function discovers as zero tests and exits
	# non-zero, which reads as a failing check rather than a mis-written one.
	cat > test_server.py <<'PY'
import unittest

import server


class PortTest(unittest.TestCase):
    def test_port_is_the_one_the_app_chose(self):
        self.assertEqual(server.PORT, 5173)
PY
	git add -A
	git commit -q -m "A service that has never heard of mia"
}

make_typescript_repo() {
	start_repo "$1"

	cat > package.json <<'JS'
{
  "name": "web",
  "private": true,
  "type": "module",
  "scripts": { "dev": "node --experimental-strip-types src/server.ts" }
}
JS
	# A lockfile, because that is the marker image discovery keys on — a project
	# with dependencies but no lockfile is not a project anybody runs twice.
	cat > package-lock.json <<'JS'
{ "name": "web", "lockfileVersion": 3, "requires": true, "packages": { "": { "name": "web" } } }
JS
	cat > tsconfig.json <<'JS'
{
  "compilerOptions": {
    "target": "es2022",
    "module": "nodenext",
    "moduleResolution": "nodenext",
    "strict": true,
    "noEmit": true
  },
  "include": ["src"]
}
JS
	mkdir -p src
	cat > src/server.ts <<'TS'
// A dev server with nothing mia-specific in it — the point of the lab.
//
// It binds the port it was written for, reports the Host it was handed, and
// streams, so port passthrough, Host rewriting and unbuffered responses are all
// exercised by an app that has never heard of mia.
import { createServer } from "node:http";

const PORT = 5173;

createServer((request, response) => {
  if (request.url === "/stream") {
    response.writeHead(200, { "Content-Type": "text/event-stream" });
    let sent = 0;
    const tick = setInterval(() => {
      response.write(`data: ${sent}\n\n`);
      if (++sent === 3) {
        clearInterval(tick);
        response.end();
      }
    }, 200);
    return;
  }
  response.writeHead(200, { "Content-Type": "text/plain; charset=utf-8" });
  response.end(
    `served ${request.url} · Host=${request.headers.host} ` +
      `· XFH=${request.headers["x-forwarded-host"]}`,
  );
}).listen(PORT, "0.0.0.0");
TS
	git add -A
	git commit -q -m "A service that has never heard of mia"
}

make_python_repo service
make_python_repo service-extra
make_typescript_repo web

cd "$lab/service"
git worktree add -q "$lab/service.arusha" -b feature-a
git worktree add -q "$lab/service.wk3" -b feature-b

cd "$lab/web"
git worktree add -q "$lab/web.longido" -b feature-c

cat > "$lab/mia.python.toml" <<'TOML'
# Copy to <repo>/.git/mia/config.toml to exercise the whole surface.
[env]
page = 5173

[[service]]
id = "web"
run = ["python3", "server.py"]
health = ["sh", "-c", "curl -fsS http://127.0.0.1:5173 >/dev/null"]
autostart = true

[checks]
imports = ["python3", "-c", "import server"]
tests = ["python3", "-m", "unittest", "discover", "-s", ".", "-p", "test_*.py"]
broken = ["sh", "-c", "echo nope >&2; exit 3"]
TOML

cat > "$lab/mia.typescript.toml" <<'TOML'
# Copy to <repo>/.git/mia/config.toml to exercise the whole surface.
[env]
page = 5173

[[service]]
id = "web"
run = ["node", "--experimental-strip-types", "src/server.ts"]
health = ["sh", "-c", "curl -fsS http://127.0.0.1:5173 >/dev/null"]
autostart = true

[checks]
types = ["npx", "--yes", "typescript@5", "tsc", "--noEmit"]
broken = ["sh", "-c", "echo nope >&2; exit 3"]
TOML

echo "$lab"
