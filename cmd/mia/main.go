//go:build unix

package main

import (
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"

	"github.com/emusoi/mia-core/internal/cli"
)

func main() {
	runtime.GOMAXPROCS(2)
	debug.SetGCPercent(50)
	if path := os.Getenv("MIA_CPUPROFILE"); path != "" {
		if f, err := os.Create(path); err == nil {
			pprof.StartCPUProfile(f)
			code := cli.Run(os.Args[1:])
			pprof.StopCPUProfile()
			f.Close()
			os.Exit(code)
		}
	}
	os.Exit(cli.Run(os.Args[1:]))
}
