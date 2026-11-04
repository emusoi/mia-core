package app

// carriers know where each agent keeps a worktree's conversation and how
// to pick it up again: claude, codex and pi. Each is a shell script run on
// the side the worktree leaves, as `script <from path> <to path> pack` for
// a tar of files under the home (nothing when there is no conversation),
// and `… command` for what to type in the window there. The conversation's
// recorded folder is rewritten to the new path, so --continue finds it.
var carriers = map[string]string{
	// claude keeps a folder's conversations in ~/.claude/projects/<the path
	// with / and . as ->; --continue picks the newest there.
	"claude": `
from=$(printf %s "$1" | tr '/.' '--'); to=$(printf %s "$2" | tr '/.' '--')
newest=$(ls -t "$HOME/.claude/projects/$from"/*.jsonl 2>/dev/null | head -1)
[ -n "$newest" ] || exit 0
case "$3" in
command) echo "claude --continue" ;;
pack)
	tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
	mkdir -p "$tmp/.claude/projects/$to"
	sed "s|$1|$2|g" "$newest" > "$tmp/.claude/projects/$to/$(basename "$newest")"
	tar -cf - -C "$tmp" .claude ;;
esac`,
	// codex keeps every conversation under ~/.codex/sessions/<y>/<m>/<d>/,
	// its folder on the first line, the time in the name so sorting finds the
	// newest; resume takes the id from the file name.
	"codex": `
dir="$HOME/.codex/sessions"
newest=$(find "$dir" -name 'rollout-*.jsonl' 2>/dev/null | sort -r | while read -r f; do
	head -1 "$f" | grep -q "\"cwd\":\"$1\"" && echo "$f" && break; done)
[ -n "$newest" ] || exit 0
case "$3" in
command) echo "codex resume $(basename "$newest" .jsonl | cut -c29-)" ;;
pack)
	rel=${newest#"$dir"/}
	tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
	mkdir -p "$tmp/.codex/sessions/$(dirname "$rel")"
	sed "s|$1|$2|g" "$newest" > "$tmp/.codex/sessions/$rel"
	tar -cf - -C "$tmp" .codex ;;
esac`,
	// pi keeps a folder's conversations in ~/.pi/agent/sessions/-<the path
	// with / as ->--; --continue picks the newest there.
	"pi": `
from="-$(printf %s "$1" | tr / -)--"; to="-$(printf %s "$2" | tr / -)--"
newest=$(ls -t "$HOME/.pi/agent/sessions/$from"/*.jsonl 2>/dev/null | head -1)
[ -n "$newest" ] || exit 0
case "$3" in
command) echo "pi --continue" ;;
pack)
	tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
	mkdir -p "$tmp/.pi/agent/sessions/$to"
	sed "s|$1|$2|g" "$newest" > "$tmp/.pi/agent/sessions/$to/$(basename "$newest")"
	tar -cf - -C "$tmp" .pi ;;
esac`,
}
