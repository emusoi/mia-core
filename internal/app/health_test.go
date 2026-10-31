package app

import (
	"slices"
	"testing"
)

func TestHealthReadsTheProbe(t *testing.T) {
	h := parseHealth(`load 31.79 27.81 27.42
cores 10
mem 41.61 64.00
disk 1178.83 1858.19
listen 2529 7842
listen 2529 7842
listen 2531 5173
listen 2531 5174
listen 99 22
cwd 2529 /home/me/src/app
cwd 2531 /home/me/src/app.kijenge/web
`)
	if h.Load != [3]float64{31.79, 27.81, 27.42} || h.Cores != 10 || h.MemUsed != 41.61 || h.MemTotal != 64 || h.DiskUsed != 1178.83 || h.DiskTotal != 1858.19 {
		t.Errorf("health = %+v", h)
	}
	if got := h.Ports["/home/me/src/app"]; !slices.Equal(got, []int{7842}) {
		t.Errorf("a port listened on twice is one port: %v", got)
	}
	if got := h.Ports["/home/me/src/app.kijenge/web"]; len(got) != 2 {
		t.Errorf("both of a process's ports, by its folder: %v", got)
	}
	if len(h.Ports) != 2 {
		t.Errorf("a listener whose folder is unknown is left out: %v", h.Ports)
	}
	if empty := parseHealth(""); empty.Cores != 0 || len(empty.Ports) != 0 {
		t.Errorf("nothing said is nothing known: %+v", empty)
	}
}
