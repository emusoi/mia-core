package app

// carriers know where each agent keeps a worktree's conversation and how
// to pick it up again: claude, codex and pi. Each is a shell script run on
// the side the worktree leaves, after a line that sets home to the folder
// of the agent's account there (see accountShell), as `script <from path>
// <to path> pack` for a tar of files under that folder (nothing when there
// is no conversation), and `… command` for what to type in the window
// there. The conversation's recorded folder is rewritten to the new path,
// so --continue finds it. A fourth word is the id of the conversation to
// take, in place of the folder's newest.
var carriers = map[string]string{
	// claude keeps a folder's conversations in <home>/projects/<the path,
	// every character but a letter or a digit a ->; --continue picks the
	// newest there.
	"claude": `
from=$(printf %s "$1" | sed 's/[^a-zA-Z0-9]/-/g'); to=$(printf %s "$2" | sed 's/[^a-zA-Z0-9]/-/g')
newest=$(ls -t "$home/projects/$from"/*.jsonl 2>/dev/null | head -1)
[ -z "$4" ] || newest=$(ls "$home/projects/$from/$4.jsonl" 2>/dev/null)
[ -n "$newest" ] || exit 0
case "$3" in
command) echo "claude --continue" ;;
pack)
	tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
	mkdir -p "$tmp/projects/$to"
	sed "s|$1|$2|g" "$newest" > "$tmp/projects/$to/$(basename "$newest")"
	COPYFILE_DISABLE=1 tar -cf - -C "$tmp" projects ;;
esac`,
	// codex keeps every conversation under <home>/sessions/<y>/<m>/<d>/,
	// its folder on the first line, the time in the name so sorting finds the
	// newest; resume takes the id from the file name.
	"codex": `
dir="$home/sessions"
newest=$(find "$dir" -name 'rollout-*.jsonl' 2>/dev/null | sort -r | while read -r f; do
	head -1 "$f" | grep -q "\"cwd\":\"$1\"" && echo "$f" && break; done)
[ -z "$4" ] || newest=$(find "$dir" -name "rollout-*-$4.jsonl" 2>/dev/null | head -1)
[ -n "$newest" ] || exit 0
case "$3" in
command) echo "codex resume $(basename "$newest" .jsonl | cut -c29-)" ;;
pack)
	rel=${newest#"$dir"/}
	tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
	mkdir -p "$tmp/sessions/$(dirname "$rel")"
	sed "s|$1|$2|g" "$newest" > "$tmp/sessions/$rel"
	COPYFILE_DISABLE=1 tar -cf - -C "$tmp" sessions ;;
esac`,
	// pi keeps a folder's conversations in <home>/sessions/-<the path with
	// / as ->--; --continue picks the newest there.
	"pi": `
from="-$(printf %s "$1" | tr / -)--"; to="-$(printf %s "$2" | tr / -)--"
newest=$(ls -t "$home/sessions/$from"/*.jsonl 2>/dev/null | head -1)
[ -z "$4" ] || newest=$(ls "$home/sessions/$from"/*_"$4".jsonl 2>/dev/null | head -1)
[ -n "$newest" ] || exit 0
case "$3" in
command) echo "pi --continue" ;;
pack)
	tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
	mkdir -p "$tmp/sessions/$to"
	sed "s|$1|$2|g" "$newest" > "$tmp/sessions/$to/$(basename "$newest")"
	COPYFILE_DISABLE=1 tar -cf - -C "$tmp" sessions ;;
esac`,
}

// carrierOf is an agent's carrier for one of its accounts, as it runs on
// either side: with home set first, for one mia has built in.
func carrierOf(agent Agent, account string) string {
	if agent.home == "" {
		return agent.carrier // a plugin's, which knows its own place
	}
	return accountShell(agent, account) + "\n" + agent.carrier
}

// landing is the script that unpacks what a carrier packed for an account:
// in its folder, for an agent mia has built in, and under the home for a
// plugin's.
func landing(agent Agent, account string) string {
	if agent.home == "" {
		return `cd && tar -xf -`
	}
	return accountShell(agent, account) + `; mkdir -p "$home" && cd "$home" && tar -xf -`
}

// riders ride along with every worktree, window or not: sawa keeps a
// worktree's review beside the repository, not in it. A rider's script is
// run in the repository on the side the worktree leaves with its path
// there, its path on the other side, the verb and the other machine's name:
// `pack` prints what goes; `land` runs on the other side with that on its
// standard input; `done` runs once the worktree is gone from where it was.
// Nothing printed by pack means nothing to carry.
var riders = map[string]string{
	"sawa": `
case "$3" in
pack) command -v sw >/dev/null 2>&1 && sw carry pack "$1" "$2" ;;
land)
	common=$(git -C "$2" rev-parse --path-format=absolute --git-common-dir) || exit 1
	(cd "$common" && tar -xf -) || exit 1
	slug=$(cat "$common/sawa/moved") || exit 1
	rm -f "$common/sawa/moved"
	dir=$(git -C "$2" rev-parse --path-format=absolute --git-dir) || exit 1
	mkdir -p "$dir/sawa" && printf '%s\n' "$slug" > "$dir/sawa/current" ;;
done) command -v sw >/dev/null 2>&1 && sw carry away "$1" "$4" ;;
esac`,
}
