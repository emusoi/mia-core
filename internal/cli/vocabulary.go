package cli

import "fmt"

const vocabulary = `WHAT THINGS ARE CALLED

worktree     A checkout: a directory with your files in it. A PLACE. It has a
             stable path and a stable name, and it survives the branch inside
             it being switched, renamed or detached. Everything else attaches
             to it. Never "workspace" — editors took that word.

name         A worktree's own word: monduli, kijenge. Allocated when the
             worktree is made and stable for its life. It is how you refer to
             the worktree and to everything attached to it, and it is what the
             hostname is made of. Not derived from the branch: branches get
             renamed and abandoned, names do not.

branch       What is checked out right now. CARGO. Read off the worktree when
             needed, changes freely, and a detached checkout is an ordinary
             state — never a failure and never garbage.

environment  Where a worktree's code runs: a container, with its interpreters,
             packages, services and ports. At most one per worktree, and a
             worktree does not need one. Plenty of checkouts are just files.

machine      Where environments can run: "local", or a computer you can ssh
             to, registered once with 'mia machine add'. 'mia env host' moves
             an environment between them, and nothing else about the
             environment changes when it moves.

staging copy A directory mia keeps on another machine holding its copy of a
             worktree, live-synced. Internal: you never type one. It is
             computed from the machine, the repository and the name, every
             time — never stored, so it cannot be inherited across a move.

service      A declared long-running process in an environment: a dev server,
             an API, a worker. An id, a command, an optional health check.

gateway      The one long-lived process. It makes <name>.mia reach the right
             container, over TLS, on whatever port the app is serving. It
             holds no truth: killing it loses nothing.

route        One name's entry in the gateway. DERIVED from the records every
             time it is asked for, never written down, so nothing can forget
             to publish one and nothing can publish a stale one.

plugin       A program named mia-<name>, in any language, on PATH or in
             ~/.config/mia/plugins. It does nothing until enabled ('mia
             plugin enable', or plugins = [...] in the config); then the
             verbs its manifest declares are mia verbs. It reads mia through
             'mia api' and keeps its own data under .git/mia/plugins/<name>.

session      A tmux session belonging to a worktree, holding shells and
             whatever you run. It runs on the worktree's machine, so a process
             started in an environment on another machine runs there.

window       One window of that session: a shell, an editor, anything you run.
             'mia window' lists them; 't' on the dashboard shows them.

stack        Several branches sharing ONE worktree, each built on the one
             below. A layer is one of those branches. 'mia up' and 'mia down'
             move between them in place, so the session, the environment and
             what runs in them survive. mia never merges into the base branch.

RETIRED WORDS

lane         Was both "the environment" and "the numbered slot it occupied".
             That conflation is what broke identity. Say environment.
city         Was the internal word for a name. The list happens to be place
             names; that is not something anyone needs to know.
slot         A number an environment occupied, which was then printed as
             identity and used as a matching key — so a pin left behind for
             slot 3 was inherited by the next environment to take that
             number. mia has no slots. The worktree is the identity.
berth        Say staging copy, and note it is derived rather than recorded.
runtime      Say machine. A box, a host, a remote: all of them are a machine.
`

func printVocabulary() int {
	fmt.Print(vocabulary)
	return exitOK
}
