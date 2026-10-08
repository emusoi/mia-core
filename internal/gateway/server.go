package gateway

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/names"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/store"
)

const Port = 7842

var Addr = fmt.Sprintf("127.0.0.1:%d", Port)

func StateDir() string { return filepath.Join(config.Dir(), "gateway") }

func Serve(ctx context.Context) error {
	ca, err := LoadCA(StateDir())
	if err != nil {
		return err
	}
	proxy := &Proxy{CA: ca, Routes: LiveRoutes, Fresh: refreshRoutes}
	go LiveRoutes()

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", servePAC)
	startedAs := Build()
	mux.HandleFunc("/build", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, startedAs)
	})
	mux.HandleFunc("/ca.pem", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, ca.CertPath())
	})
	mux.HandleFunc("/", serveStatus)

	server := &http.Server{
		Addr: Addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSelf(r.Host) && r.Method != http.MethodConnect {
				mux.ServeHTTP(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 30 * time.Second,
	}

	listener, err := net.Listen("tcp", Addr)
	if err != nil {
		return fmt.Errorf("%w — another mia gateway is probably already running", err)
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func isSelf(host string) bool {
	name, _ := splitHostPort(host, Port)
	return !strings.HasSuffix(name, Suffix)
}

func Build() string {
	path, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	info, err := os.Stat(path)
	if err != nil {
		return path
	}
	return fmt.Sprintf("%s %d %d", path, info.Size(), info.ModTime().UnixNano())
}

func ServingBuild() (string, bool) {
	client := http.Client{Timeout: 600 * time.Millisecond}
	response, err := client.Get("http://" + Addr + "/build")
	if err != nil {
		return "", false
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return "", false
	}
	return string(body), true
}

func Running() bool {
	conn, err := net.DialTimeout("tcp", Addr, 400*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func LiveRoutes() Table {
	cached.Lock()
	table := cached.table
	if table == nil {
		cached.Unlock()
		return refreshRoutes()
	}
	if time.Since(cached.at) >= freshness && !cached.refreshing {
		cached.refreshing = true
		go refreshRoutes()
	}
	cached.Unlock()
	return table
}

func refreshRoutes() Table {
	fresh := buildRoutes()
	cached.Lock()
	defer cached.Unlock()
	cached.table, cached.at, cached.refreshing = fresh, time.Now(), false
	return fresh
}

const freshness = 2 * time.Second

var cached struct {
	sync.Mutex
	table      Table
	at         time.Time
	refreshing bool
}

func buildRoutes() Table {
	registry := &names.Store{Path: config.NamesPath()}
	owners, err := registry.All()
	if err != nil {
		return Table{}
	}

	table := make(Table, len(owners))
	running := map[container.Engine]map[string]bool{}
	for name, owner := range owners {
		if _, err := os.Stat(owner); err != nil {
			continue
		}
		where, record, ok := locate(owner, name)
		if !ok {
			continue
		}
		containerName := env.ContainerName(env.Sanitise(record.Name))
		up, asked := running[where.Engine]
		if !asked {
			up = where.Engine.Running()
			running[where.Engine] = up
		}
		if !up[containerName] {
			continue
		}
		engine := where.Engine
		settings := environmentSettings(owner)
		table[name+Suffix] = Route{
			Host: name + Suffix,
			Name: name,
			Page: settings.Page,
			Dial: func(ctx context.Context, port int) (net.Conn, error) {
				return engine.Dial(containerName, port)
			},
			Serving: func() []int {
				open, _ := engine.Listening(containerName)
				return open
			},
			PassHost: settings.PassHost,
		}
	}
	return table
}

func locate(worktree, name string) (env.Location, model.Record, bool) {
	_, common, err := git.Repository(worktree)
	if err != nil {
		return env.Location{}, model.Record{}, false
	}
	miaDir := filepath.Join(common, "mia")
	records, err := (store.Store{Dir: miaDir}).Load()
	if err != nil {
		return env.Location{}, model.Record{}, false
	}
	var record model.Record
	for _, candidate := range records {
		if candidate.Path == worktree {
			record = candidate
		}
	}
	if record.Name == "" {
		return env.Location{}, model.Record{}, false
	}
	cfg, err := config.Load(miaDir)
	if err != nil {
		return env.Location{}, model.Record{}, false
	}
	local, err := container.Find()
	if err != nil {
		return env.Location{}, model.Record{}, false
	}
	manager := env.Manager{
		Engine:   local,
		Runtimes: runtime.Store{Path: config.RuntimesPath()},
		Repo:     filepath.Dir(common),
		Settings: cfg.Env,
	}
	where, err := manager.Where(record)
	if err != nil {
		return env.Location{}, model.Record{}, false
	}
	return where, record, true
}

func environmentSettings(worktree string) env.Settings {
	_, common, err := git.Repository(worktree)
	if err != nil {
		return env.Settings{}
	}
	cfg, err := config.Load(filepath.Join(common, "mia"))
	if err != nil {
		return env.Settings{}
	}
	return cfg.Env
}

func servePAC(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	fmt.Fprintf(w, `function FindProxyForURL(url, host) {
  if (dnsDomainIs(host, "%s") || host == "%s") return "PROXY %s";
  return "DIRECT";
}
`, Suffix, strings.TrimPrefix(Suffix, "."), Addr)
}

func serveStatus(w http.ResponseWriter, r *http.Request) {
	table := LiveRoutes()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><meta charset=utf-8><title>mia gateway</title>
<style>body{font:15px/1.7 ui-monospace,SFMono-Regular,Menlo,monospace;max-width:36rem;margin:12vh auto;padding:0 1.5rem;color:#1a1d23;background:#f7f8fa}
h1{font-size:1rem;margin:0 0 1rem}a{color:inherit}li{margin:.2rem 0}p{color:#4a5160}
@media(prefers-color-scheme:dark){body{color:#e7eaef;background:#14171c}p{color:#a9b1bf}}</style>
<h1>mia gateway</h1>`)
	if len(table) == 0 {
		fmt.Fprint(w, "<p>No environment is running. <code>mia env up</code></p>")
	} else {
		fmt.Fprint(w, "<ul>")
		for host, route := range table {
			fmt.Fprintf(w, `<li><a href="https://%s/">%s</a> · <a href="http://%s.localhost:%d/">direct</a></li>`,
				host, host, route.Name, Port)
		}
		fmt.Fprint(w, "</ul>")
	}
	fmt.Fprintf(w, `<p>PAC: <code>http://%s/proxy.pac</code></p>`, Addr)
}
