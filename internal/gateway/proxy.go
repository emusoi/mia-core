package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Route struct {
	Host string
	Name string
	Page int

	Dial func(ctx context.Context, port int) (net.Conn, error)

	Serving func() []int

	PassHost bool
}

type Table map[string]Route

type Resolver func() Table

type Proxy struct {
	CA         *CA
	Routes     Resolver
	Fresh      Resolver
	transports sync.Map
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	host, port := splitHostPort(r.Host, 80)
	route, ok := p.lookup(host)
	if !ok {
		p.explain(w, http.StatusNotFound, host, "", "no environment answers to that name")
		return
	}
	if strings.HasSuffix(host, ".localhost") {
		port = pagePort(route)
		if port == 0 {
			p.explain(w, http.StatusBadGateway, host, route.Name,
				"the environment has no configured page and is not serving a port")
			return
		}
	}
	p.forward(w, r, route, port)
}

func (p *Proxy) connect(w http.ResponseWriter, r *http.Request) {
	host, port := splitHostPort(r.Host, 443)
	route, ok := p.lookup(host)
	if !ok {
		http.Error(w, "no environment answers to "+host, http.StatusNotFound)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "this server cannot take over the connection", http.StatusInternalServerError)
		return
	}
	client, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer client.Close()

	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	inside := tls.Server(client, p.CA.TLSConfig())
	if err := inside.HandshakeContext(r.Context()); err != nil {
		return
	}
	defer inside.Close()

	serveOver(inside, func(rw http.ResponseWriter, req *http.Request) {
		p.forward(rw, req, route, port)
	})
}

func asLocalhost(header http.Header, name string) {
	value := header.Get(name)
	if value == "" {
		return
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || !strings.HasSuffix(parsed.Hostname(), Suffix) {
		return
	}
	host := "localhost"
	if port := parsed.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	}
	parsed.Scheme, parsed.Host = "http", host
	header.Set(name, parsed.String())
}

func asAsked(header http.Header, name, origin string) {
	value := header.Get(name)
	if origin == "" || value == "" || value == "*" {
		return
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() != "localhost" {
		return
	}
	header.Set(name, origin)
}

func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, route Route, port int) {
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort(route.Name, strconv.Itoa(port))}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = r.Host
			if !route.PassHost {
				pr.Out.Host = "localhost:" + strconv.Itoa(port)
				asLocalhost(pr.Out.Header, "Origin")
				asLocalhost(pr.Out.Header, "Referer")
			}
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Forwarded-Host", r.Host)
			pr.Out.Header.Set("X-Forwarded-Proto", "https")
		},
		ModifyResponse: func(res *http.Response) error {
			if !route.PassHost {
				asAsked(res.Header, "Access-Control-Allow-Origin", r.Header.Get("Origin"))
			}
			return nil
		},
		Transport: p.transport(route.Host, port),
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			p.explain(rw, http.StatusBadGateway, route.Host, route.Name,
				fmt.Sprintf("nothing answered on port %d inside %s.%s", port, route.Name, serving(route, port)))
		},
		FlushInterval: -1,
	}
	proxy.ServeHTTP(w, r)
}

func (p *Proxy) transport(host string, port int) *http.Transport {
	key := net.JoinHostPort(host, strconv.Itoa(port))
	if held, ok := p.transports.Load(key); ok {
		return held.(*http.Transport)
	}
	fresh := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			route, ok := p.lookup(host)
			if !ok {
				return nil, fmt.Errorf("no environment answers to %s", host)
			}
			return route.Dial(ctx, port)
		},
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	held, _ := p.transports.LoadOrStore(key, fresh)
	return held.(*http.Transport)
}

func serving(route Route, wanted int) string {
	if route.Serving == nil {
		return ""
	}
	open := route.Serving()
	if len(open) == 0 {
		return " It is serving nothing at all — its server may not have started."
	}
	text := make([]string, 0, len(open))
	listening := false
	for _, port := range open {
		text = append(text, strconv.Itoa(port))
		if port == wanted {
			listening = true
		}
	}
	if listening {
		return fmt.Sprintf(" It IS listening on %d, so the environment itself could not be reached — `mia env up %s` to rebuild it.", wanted, route.Name)
	}
	if len(open) == 1 {
		return fmt.Sprintf(" It is serving %s, not %d.", text[0], wanted)
	}
	return fmt.Sprintf(" It is serving %s — not %d.", strings.Join(text, ", "), wanted)
}

func (p *Proxy) lookup(host string) (Route, bool) {
	host = strings.ToLower(host)
	route, ok := find(p.Routes(), host)
	if !ok && p.Fresh != nil {
		route, ok = find(p.Fresh(), host)
	}
	return route, ok
}

func find(table Table, host string) (Route, bool) {
	route, ok := table[host]
	if !ok {
		if name, direct := strings.CutSuffix(host, ".localhost"); direct && name != "" {
			route, ok = table[name+Suffix]
		}
	}
	return route, ok
}

func pagePort(route Route) int {
	if route.Page > 0 {
		return route.Page
	}
	if route.Serving == nil {
		return 0
	}
	lowest := 0
	for _, port := range route.Serving() {
		if lowest == 0 || port < lowest {
			lowest = port
		}
	}
	return lowest
}

func (p *Proxy) explain(w http.ResponseWriter, status int, host, name, detail string) {
	next := "mia ls"
	if name != "" {
		next = "mia env status " + name
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><title>%s — mia</title>
<style>body{font:15px/1.7 ui-monospace,SFMono-Regular,Menlo,monospace;max-width:36rem;margin:14vh auto;padding:0 1.5rem;color:#1a1d23;background:#f7f8fa}
h1{font-size:1rem;margin:0 0 .6rem}p{margin:.5rem 0;color:#4a5160}code{background:#e9ecf1;padding:.12em .4em;border-radius:3px}
@media(prefers-color-scheme:dark){body{color:#e7eaef;background:#14171c}p{color:#a9b1bf}code{background:#242a33}}</style>
<h1>%s</h1><p>%s</p><p>Try <code>%s</code></p>
`, host, host, detail, next)
}

func splitHostPort(authority string, fallback int) (string, int) {
	host, portText, err := net.SplitHostPort(authority)
	if err != nil {
		return strings.ToLower(authority), fallback
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return strings.ToLower(host), fallback
	}
	return strings.ToLower(host), port
}

func serveOver(conn net.Conn, handler http.HandlerFunc) {
	done := make(chan struct{})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 30 * time.Second}
	go server.Serve(&oneShotListener{conn: &signalOnClose{Conn: conn, done: done}, done: done})
	<-done
}

type oneShotListener struct {
	conn net.Conn
	done chan struct{}
	gone bool
}

func (l *oneShotListener) Accept() (net.Conn, error) {
	if l.gone {
		<-l.done
		return nil, io.EOF
	}
	l.gone = true
	return l.conn, nil
}
func (l *oneShotListener) Close() error   { return nil }
func (l *oneShotListener) Addr() net.Addr { return l.conn.LocalAddr() }

type signalOnClose struct {
	net.Conn
	done chan struct{}
	once sync.Once
}

func (c *signalOnClose) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.done) })
	return err
}
