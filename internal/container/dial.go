package container

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emusoi/mia-core/internal/run"
)

// Dial opens a connection to port inside the container. On a remote podman
// host it joins the container's network namespace with nsenter, which takes
// about 80 ms, where a podman exec took a second on a slow box and every page
// load and reconnect through the gateway paid it. podman exec stays the way
// in when that cannot be used.
func (e Engine) Dial(name string, port int) (net.Conn, error) {
	if e.SSH != "" && filepath.Base(e.Binary) == "podman" {
		if conn, err := e.dialNamespace(name, port); err == nil {
			return conn, nil
		}
	}
	tool, err := e.dialer(name)
	if err != nil {
		return nil, err
	}
	stdin, stdout, command, err := e.tool().Pipe(append([]string{"exec", "-i", "-w", "/", name}, tool.argv(port)...)...)
	if err != nil {
		return nil, err
	}
	return &execConn{command: command, in: stdin, out: stdout, name: name, port: port}, nil
}

// A container's process and id on its host, looked up once: a podman
// inspect is itself slow on a slow box.
type namespaceTarget struct{ pid, id string }

var (
	namespacesMu sync.Mutex
	namespaces   = map[string]namespaceTarget{}
)

func (e Engine) dialNamespace(name string, port int) (net.Conn, error) {
	key := e.SSH + "/" + name
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		namespacesMu.Lock()
		target, known := namespaces[key]
		namespacesMu.Unlock()
		if !known {
			out, inspectErr := e.run("inspect", "--format", "{{.State.Pid}} {{.Id}}", name)
			fields := strings.Fields(out)
			if inspectErr != nil || len(fields) != 2 || fields[0] == "0" {
				return nil, fmt.Errorf("%s is not running", name)
			}
			target = namespaceTarget{pid: fields[0], id: fields[1]}
			namespacesMu.Lock()
			namespaces[key] = target
			namespacesMu.Unlock()
		}
		var conn net.Conn
		if conn, err = e.namespaceConn(target, name, port); err == nil {
			return conn, nil
		}
		// A restarted container has a new process: look it up again once.
		namespacesMu.Lock()
		delete(namespaces, key)
		namespacesMu.Unlock()
	}
	return nil, err
}

// namespaceConn relays a connection through bash inside the container's
// network namespace. The host first checks the process is still this
// container (its id is in the process's cgroup) and answers with one byte,
// so a stale process is noticed before any data flows.
func (e Engine) namespaceConn(target namespaceTarget, name string, port int) (net.Conn, error) {
	script := fmt.Sprintf(`grep -q %s /proc/%s/cgroup 2>/dev/null || exit 97
printf M
exec nsenter -t %s -U -n --preserve-credentials bash -c 'exec 3<>/dev/tcp/127.0.0.1/%d || exit 1; cat <&3 & cat >&3; wait'`,
		target.id, target.pid, target.pid, port)
	stdin, stdout, command, err := run.Tool{Binary: "sh", SSH: e.SSH, Label: e.SSH}.Pipe("-c", script)
	if err != nil {
		return nil, err
	}
	conn := &execConn{command: command, in: stdin, out: stdout, name: name, port: port}
	var mark [1]byte
	if _, err := io.ReadFull(stdout, mark[:]); err != nil || mark[0] != 'M' {
		conn.Close()
		return nil, fmt.Errorf("%s: its process is not the one mia knew", name)
	}
	return conn, nil
}

type dialTool struct {
	name string
	argv func(port int) []string
}

var dialTools = []dialTool{
	{"socat", func(p int) []string {
		return []string{"socat", "-", fmt.Sprintf("TCP:127.0.0.1:%d", p)}
	}},
	{"nc", func(p int) []string {
		return []string{"nc", "127.0.0.1", fmt.Sprint(p)}
	}},
	{"bash", func(p int) []string {
		return []string{"bash", "-c", fmt.Sprintf(
			"exec 3<>/dev/tcp/127.0.0.1/%d; cat <&3 & cat >&3; wait", p)}
	}},
	{"python3", func(p int) []string {
		return []string{"python3", "-c", pythonRelay, fmt.Sprint(p)}
	}},
}

const pythonRelay = `
import socket,sys,threading
s=socket.create_connection(("127.0.0.1",int(sys.argv[1])))
def pump(r,w):
    try:
        while True:
            b=r.read(65536)
            if not b: break
            w.write(b); w.flush()
    finally:
        try: s.shutdown(socket.SHUT_WR)
        except OSError: pass
threading.Thread(target=pump,args=(sys.stdin.buffer,s.makefile("wb")),daemon=True).start()
pump(s.makefile("rb"),sys.stdout.buffer)
`

var (
	dialersMu sync.Mutex
	dialers   = map[string]dialTool{}
)

func (e Engine) dialer(name string) (dialTool, error) {
	dialersMu.Lock()
	defer dialersMu.Unlock()
	key := e.SSH + "/" + name
	if tool, ok := dialers[key]; ok {
		return tool, nil
	}
	for _, tool := range dialTools {
		if _, err := e.Exec(name, "sh", "-c", "command -v "+tool.name); err == nil {
			dialers[key] = tool
			return tool, nil
		}
	}
	return dialTool{}, fmt.Errorf(
		"nothing in this image can open a connection — mia needs one of socat, nc, bash or python3 inside %s", name)
}

type execConn struct {
	command *exec.Cmd
	in      io.WriteCloser
	out     io.ReadCloser
	name    string
	port    int
	once    sync.Once
}

func (c *execConn) Read(b []byte) (int, error)  { return c.out.Read(b) }
func (c *execConn) Write(b []byte) (int, error) { return c.in.Write(b) }

func (c *execConn) Close() error {
	c.once.Do(func() {
		c.in.Close()
		c.out.Close()
		if c.command.Process != nil {
			c.command.Process.Kill()
		}
		c.command.Wait()
	})
	return nil
}

func (c *execConn) SetDeadline(time.Time) error      { return errNoDeadline }
func (c *execConn) SetReadDeadline(time.Time) error  { return errNoDeadline }
func (c *execConn) SetWriteDeadline(time.Time) error { return errNoDeadline }

var errNoDeadline = fmt.Errorf("a connection through the container engine has no deadline")

func (c *execConn) LocalAddr() net.Addr  { return execAddr("mia") }
func (c *execConn) RemoteAddr() net.Addr { return execAddr(fmt.Sprintf("%s:%d", c.name, c.port)) }

type execAddr string

func (a execAddr) Network() string { return "mia-exec" }
func (a execAddr) String() string  { return string(a) }
