package app

import (
	"strconv"
	"strings"
)

// Health is how busy a machine is: what the dash's machines view draws.
type Health struct {
	Load                [3]float64
	Cores               int
	MemUsed, MemTotal   float64 // GB
	DiskUsed, DiskTotal float64 // GB, of the disk the repository is on
	// Ports are the TCP ports listened on, by the directory of the process
	// that listens: matched to worktrees by whoever knows their paths.
	Ports map[string][]int
}

// probeScript measures the machine it runs on, in the directory $1, with
// what every Linux and macOS has. One fact a line; a missing tool leaves
// its line out.
const probeScript = `PATH="$PATH:/usr/sbin:/sbin"
cd "$1" 2>/dev/null || cd
echo "load $(uptime | sed 's/.*load averages*: //; s/,//g')"
echo "cores $(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu)"
if [ -r /proc/meminfo ]; then
	awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{printf "mem %.2f %.2f\n", (t-a)/1048576, t/1048576}' /proc/meminfo
else
	total=$(sysctl -n hw.memsize)
	vm_stat | awk -v total="$total" '/page size of/{page=$8} /^Pages (active|wired down|occupied by compressor):/{used+=$NF} END{printf "mem %.2f %.2f\n", used*page/1073741824, total/1073741824}'
fi
df -Pk . | awk 'NR==2{printf "disk %.2f %.2f\n", ($2-$4)/1048576, $2/1048576}'
listen=$(lsof -nP -iTCP -sTCP:LISTEN -Fpn 2>/dev/null) || listen=""
printf '%s\n' "$listen" | awk '/^p/{p=substr($0,2)} /^n/{n=$0; sub(/.*:/,"",n); print "listen", p, n}'
pids=$(printf '%s\n' "$listen" | sed -n 's/^p//p' | paste -sd, -)
[ -z "$pids" ] || lsof -a -d cwd -Fpn -p "$pids" 2>/dev/null | awk '/^p/{p=substr($0,2)} /^n/{print "cwd", p, substr($0,2)}'
true`

func parseHealth(out string) Health {
	h := Health{Ports: map[string][]int{}}
	listening := map[string][]int{} // pid → ports
	cwd := map[string]string{}      // pid → directory
	for _, line := range strings.Split(out, "\n") {
		word, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
		fields := strings.Fields(rest)
		number := func(i int) float64 {
			if i >= len(fields) {
				return 0
			}
			n, _ := strconv.ParseFloat(fields[i], 64)
			return n
		}
		switch word {
		case "load":
			h.Load = [3]float64{number(0), number(1), number(2)}
		case "cores":
			h.Cores = int(number(0))
		case "mem":
			h.MemUsed, h.MemTotal = number(0), number(1)
		case "disk":
			h.DiskUsed, h.DiskTotal = number(0), number(1)
		case "listen":
			if port := int(number(1)); len(fields) == 2 && port > 0 {
				listening[fields[0]] = append(listening[fields[0]], port)
			}
		case "cwd":
			if pid, dir, ok := strings.Cut(rest, " "); ok {
				cwd[pid] = dir
			}
		}
	}
	for pid, ports := range listening {
		if dir := cwd[pid]; dir != "" {
			for _, port := range ports {
				if !containsInt(h.Ports[dir], port) {
					h.Ports[dir] = append(h.Ports[dir], port)
				}
			}
		}
	}
	return h
}

func containsInt(list []int, n int) bool {
	for _, one := range list {
		if one == n {
			return true
		}
	}
	return false
}
