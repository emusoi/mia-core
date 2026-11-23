package app

import "syscall"

// leadsIn reports whether a process leads a group of its own in the
// session a pane's process leads: what a shell in the pane runs, in front
// or not, and the pane's own program. One that is gone leads none.
func leadsIn(pid, pane int) bool {
	sid, err := syscall.Getsid(pid)
	if err != nil || sid != pane {
		return false
	}
	group, err := syscall.Getpgid(pid)
	return err == nil && group == pid
}
