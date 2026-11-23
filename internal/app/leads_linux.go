package app

import (
	"os"
	"strconv"
	"strings"
)

// leadsIn reports whether a process leads a group of its own in the
// session a pane's process leads: what a shell in the pane runs, in front
// or not, and the pane's own program. One that is gone leads none.
func leadsIn(pid, pane int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// pid (comm) state ppid pgrp session …, comm in parentheses, and in it anything
	at := strings.LastIndex(string(stat), ") ")
	if at < 0 {
		return false
	}
	fields := strings.Fields(string(stat)[at+2:])
	if len(fields) < 4 {
		return false
	}
	return fields[2] == strconv.Itoa(pid) && fields[3] == strconv.Itoa(pane)
}
