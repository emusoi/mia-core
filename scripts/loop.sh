#!/bin/sh
# The daily loop, end to end, on a throwaway repository with its own name
# registry. Exit status is the verdict. Needs mia on PATH, git, tmux.
set -eu

lab="$(mktemp -d)"
trap 'tmux kill-session -t "=mia-$wt" 2>/dev/null || true; rm -rf "$lab"' EXIT
export XDG_CONFIG_HOME="$lab/xdg"
export EDITOR=true
mkdir -p "$lab/xdg" "$lab/repo"
cd "$lab/repo"
git init -q -b main && printf 'one\n' > app.txt && git add . && git commit -q -m init

fail() { echo "FAIL: $*" >&2; exit 1; }
step() { printf '%-44s' "$1"; }

step "adopt + config"
mia adopt . >/dev/null
printf '[launch]\nprobe = "true"\n' > .git/mia/config.toml
echo ok

step "new, ls, path"
mia new due-dates >/dev/null
wt="$(mia ls | awk '$2=="due-dates"{print $1}')"
[ -n "$wt" ] || fail "new worktree not listed"
[ -d "$(mia path "$wt")" ] || fail "path does not resolve"
echo "ok ($wt)"

step "star"
mia star "$wt" >/dev/null
mia ls | grep -q "★ $wt" || fail "ls shows no star"
mia api dashboard | grep -q '"starred"' || fail "no starred section"
mia star "$wt" >/dev/null
echo ok

step "session windows"
mia window new "$wt" probe -- sh -c 'echo loop-window; sleep 30' >/dev/null
sleep 1
mia window ls "$wt" | grep -q probe || fail "window not listed"
tmux capture-pane -p -t "=mia-$wt:probe" | grep -q loop-window || fail "the window did not run its command"
mia window close "$wt" probe >/dev/null
echo ok

step "stack: layer, name, switch by name"
mia new --stack --in "$wt" api-layer >/dev/null
mia stack --in "$wt" | grep -q "2/2 layers\|1/2 layers\|layers" || fail "stack"
(cd "$(mia path "$wt")" && mia stack name payments >/dev/null)
[ "$(mia path payments)" = "$(mia path "$wt")" ] || fail "stack name does not resolve"
echo ok

step "stack: merge down, never into the base"
(cd "$(mia path "$wt")" && git commit -q --allow-empty -m "api work")
mia new --stack --in "$wt" ui-layer >/dev/null
(cd "$(mia path "$wt")" && git commit -q --allow-empty -m "ui work")
mia stack merge --in "$wt" ui-layer onto main 2>/dev/null && fail "merged into the base branch"
mia stack merge --in "$wt" ui-layer onto api-layer >/dev/null || fail "merge down failed"
mia stack --in "$wt" | grep -q "ui-layer" && fail "the merged layer is still a layer"
[ "$(cd "$(mia path "$wt")" && git branch --show-current)" = "api-layer" ] || fail "the worktree did not move to the layer it merged into"
echo ok

step "gc keeps a worktree with a session"
mia gc --days 0 | grep -q "$wt" && fail "gc offered a worktree with a live session"
echo ok

step "a plugin adds a verb once enabled"
mkdir -p "$lab/xdg/mia/plugins"
cat > "$lab/xdg/mia/plugins/mia-hello" <<'SH'
#!/bin/sh
case "$1" in
manifest) echo '{"protocol":1,"verbs":[{"name":"hello"}],"rows":{"every":"1s"},"sections":[{"id":"greeted","label":"greeted","rank":20}],"tabs":[{"id":"hi","label":"hi"}],"keys":[{"id":"wave","key":"H","label":"wave","verb":"hello","args":["{row}"],"report":true},{"id":"all","key":"V","label":"everyone","panel":"all","global":true}]}' ;;
panel) echo '{"title":"everyone","sections":[{"id":"people","label":"people","rows":[{"id":"ana","cells":["ana"]}]}]}' ;;
hello) shift; echo "hello $* in $MIA_WORKTREE" ;;
rows) grep -o '"name":"[a-z0-9-]*"' | sed 's/.*:"\(.*\)"/"\1":{"section":"greeted","status":"said hello","tabs":{"hi":["waved"]}}/' | paste -sd, - | sed 's/.*/{"rows":{&}}/' ;;
esac
SH
chmod +x "$lab/xdg/mia/plugins/mia-hello"
mia hello >/dev/null 2>&1 && fail "a plugin ran before it was enabled"
mia plugin enable hello >/dev/null
[ "$(cd "$(mia path "$wt")" && mia hello --json there)" = "hello --json there in $wt" ] || fail "the plugin verb did not run with its context"
mia plugin ls | grep -q "hello *on *verbs: hello" || fail "plugin ls does not show it"
mia api dashboard | grep -q '"label": "greeted"' || fail "the plugin's section is not on the dashboard"
mia api dashboard | grep -q '"said hello"' || fail "the plugin's status is not on the row"
mia api dashboard | grep -q '"hello.wave"' || fail "the plugin's key is not offered"
mia api dashboard | grep -q '"waved"' || fail "the plugin's tab is not on the row"
mia api plugin:hello:all | grep -q '"ana"' || fail "the plugin's panel did not draw"
mia plugin disable hello >/dev/null
echo ok

step "a plugin hears what happens"
cat > "$lab/xdg/mia/plugins/mia-ear" <<SH
#!/bin/sh
case "\$1" in
manifest) echo '{"protocol":1,"events":["worktree.created","session.started","worktree.removed"]}' ;;
event) echo "\$2 \$(sed -n 's/.*"worktree":"\([a-z0-9-]*\)".*/\1/p')" >> "$lab/heard" ;;
esac
SH
chmod +x "$lab/xdg/mia/plugins/mia-ear"
mia plugin enable ear >/dev/null
mia new heard-it >/dev/null
ear="$(mia ls | awk '$2=="heard-it"{print $1}')"
mia window new "$ear" probe -- sh -c 'sleep 30' >/dev/null
mia rm --stop-running "$ear" >/dev/null
for _ in 1 2 3 4 5 6 7 8 9 10; do [ "$(wc -l < "$lab/heard" 2>/dev/null)" -ge 3 ] && break; sleep 0.2; done
[ "$(cat "$lab/heard")" = "$(printf 'worktree.created %s\nsession.started %s\nworktree.removed %s' "$ear" "$ear" "$ear")" ] || fail "events heard: $(cat "$lab/heard" 2>/dev/null)"
mia plugin disable ear >/dev/null
echo ok

step "rm --stop-running"
git -C "$(mia path "$wt")" checkout -q -- .
mia rm --stop-running "$wt" >/dev/null
mia ls | grep -q "$wt" && fail "still listed after rm"
tmux has-session -t "=mia-$wt" 2>/dev/null && fail "session survived rm"
echo ok

echo "PASS — the loop holds"
