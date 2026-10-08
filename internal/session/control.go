package session

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/emusoi/mia-core/internal/run"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const controlSession = "_mia-control"

type control struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	in   io.Writer
	out  *bufio.Reader
	dead atomic.Bool
}

var (
	controlOn   bool
	controlOnce sync.Once
	controlling *control
)

func UseControl() { controlOn = os.Getenv("MIA_TMUX_SPAWN") == "" }

func client() *control {
	if !controlOn {
		return nil
	}
	controlOnce.Do(func() {
		c, err := startControl()
		if err == nil {
			controlling = c
		}
	})
	if controlling == nil || controlling.dead.Load() {
		return nil
	}
	return controlling
}

func startControl() (*control, error) {
	cmd := run.Local("tmux").Command("-C", "new-session", "-A", "-D", "-s", controlSession, "-x", "20", "-y", "5", "sleep 2147483647")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &control{cmd: cmd, in: in, out: bufio.NewReaderSize(out, 1<<20)}
	go func() { cmd.Wait(); c.dead.Store(true) }()
	if _, err := c.reply(); err != nil {
		c.dead.Store(true)
		return nil, err
	}
	return c, nil
}

func quoteArg(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func (c *control) run(args []string) (string, error) {
	var b strings.Builder
	for i, arg := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(quoteArg(arg))
	}
	return c.request(b.String())
}

func (c *control) request(line string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead.Load() {
		return "", errors.New("tmux control client is gone")
	}
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.dead.Store(true)
		return "", err
	}
	return c.reply()
}

func (c *control) reply() (string, error) {
	var out strings.Builder
	inBlock := false
	for {
		raw, err := c.out.ReadString('\n')
		if err != nil {
			c.dead.Store(true)
			return "", err
		}
		text := strings.TrimRight(raw, "\r\n")
		switch {
		case !inBlock && strings.HasPrefix(text, "%begin "):
			inBlock = true
		case inBlock && strings.HasPrefix(text, "%end "):
			return out.String(), nil
		case inBlock && strings.HasPrefix(text, "%error "):
			return out.String(), fmt.Errorf("tmux: %s", strings.TrimSpace(out.String()))
		case inBlock:
			out.WriteString(text)
			out.WriteByte('\n')
		}
	}
}

func (h Host) local(args ...string) (string, error) {
	if c := client(); c != nil {
		started := time.Now()
		out, err := c.run(args)
		if trace {
			fmt.Fprintf(os.Stderr, "tmuxC %4dms %s\n", time.Since(started).Milliseconds(), strings.Join(args, " "))
		}
		if err == nil || !c.dead.Load() {
			return out, err
		}
	}
	return run.Tool{Binary: "tmux", Verbose: trace}.Combined(args...)
}
