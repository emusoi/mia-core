package gateway_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/emusoi/mia-core/internal/gateway"
)

type backend struct {
	listeners map[int]net.Listener
}

func newBackend(t *testing.T, ports ...int) *backend {
	t.Helper()
	b := &backend{listeners: map[int]net.Listener{}}
	for _, port := range ports {
		port := port
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		b.listeners[port] = listener
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/stream" {
				flusher := w.(http.Flusher)
				w.Header().Set("Content-Type", "text/event-stream")
				for i := range 3 {
					fmt.Fprintf(w, "data: %d\n\n", i)
					flusher.Flush()
				}
				return
			}
			fmt.Fprintf(w, "port=%d host=%s fwd-host=%s fwd-proto=%s",
				port, r.Host, r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Forwarded-Proto"))
		})}
		go server.Serve(listener)
		t.Cleanup(func() { server.Close() })
	}
	return b
}

func (b *backend) dial(_ context.Context, port int) (net.Conn, error) {
	listener, ok := b.listeners[port]
	if !ok {
		return nil, fmt.Errorf("connection refused")
	}
	return net.Dial("tcp", listener.Addr().String())
}

func start(t *testing.T, b *backend, passHost bool) (*http.Client, *gateway.CA) {
	t.Helper()
	ca, err := gateway.LoadCA(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proxy := &gateway.Proxy{CA: ca, Routes: func() gateway.Table {
		return gateway.Table{"monduli.mia": {
			Host: "monduli.mia", Name: "monduli", Page: 5173, Dial: b.dial, PassHost: passHost,
			Serving: func() []int {
				open := make([]int, 0, len(b.listeners))
				for port := range b.listeners {
					open = append(open, port)
				}
				sort.Ints(open)
				return open
			},
		}}
	}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: proxy}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })

	pool := x509.NewCertPool()
	pem, err := os.ReadFile(ca.CertPath())
	if err != nil {
		t.Fatal(err)
	}
	pool.AppendCertsFromPEM(pem)

	proxyURL := &url.URL{Scheme: "http", Host: listener.Addr().String()}
	return &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{RootCAs: pool},
	}}, ca
}

func TestLocalhostAliasReachesTheConfiguredPageWithoutBrowserProxySettings(t *testing.T) {
	b := newBackend(t, 5173, 8000)
	ca, err := gateway.LoadCA(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proxy := &gateway.Proxy{CA: ca, Routes: func() gateway.Table {
		return gateway.Table{"monduli.mia": {
			Host: "monduli.mia", Name: "monduli", Page: 5173, Dial: b.dial,
		}}
	}}

	request := httptest.NewRequest("GET", "http://monduli.localhost:7842/", nil)
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("localhost alias answered %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "port=5173") {
		t.Fatalf("localhost alias missed the configured page: %s", response.Body.String())
	}
}

func get(t *testing.T, client *http.Client, target string) (int, string) {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(body)
}

func TestThePortPassesThrough(t *testing.T) {
	b := newBackend(t, 5173, 8000)
	client, _ := start(t, b, false)

	for _, port := range []int{5173, 8000} {
		status, body := get(t, client, fmt.Sprintf("https://monduli.mia:%d/", port))
		if status != 200 {
			t.Fatalf(":%d answered %d", port, status)
		}
		if !strings.Contains(body, fmt.Sprintf("port=%d", port)) {
			t.Errorf("https://monduli.mia:%d/ reached the wrong port: %s", port, body)
		}
	}
}

func TestTheAppSeesTheHostItExpects(t *testing.T) {
	b := newBackend(t, 5173)
	client, _ := start(t, b, false)

	_, body := get(t, client, "https://monduli.mia:5173/")
	if !strings.Contains(body, "host=localhost:5173") {
		t.Errorf("the app was handed a Host it never heard of: %s", body)
	}
	if !strings.Contains(body, "fwd-host=monduli.mia:5173") || !strings.Contains(body, "fwd-proto=https") {
		t.Errorf("X-Forwarded-* did not survive: %s", body)
	}
}

func TestPassHostIsTheExceptionAndWorks(t *testing.T) {
	b := newBackend(t, 5173)
	client, _ := start(t, b, true)

	_, body := get(t, client, "https://monduli.mia:5173/")
	if !strings.Contains(body, "host=monduli.mia:5173") {
		t.Errorf("pass_host did not pass the host: %s", body)
	}
}

func TestAFailureSaysWhatAndWhatNext(t *testing.T) {
	b := newBackend(t, 5173)
	client, _ := start(t, b, false)

	status, body := get(t, client, "https://monduli.mia:9999/")
	if status != http.StatusBadGateway {
		t.Fatalf("a dead port answered %d", status)
	}
	for _, want := range []string{"monduli", "9999", "5173", "mia env status"} {
		if !strings.Contains(body, want) {
			t.Errorf("the 502 page never mentions %q: %s", want, body)
		}
	}
}

func TestAnUnknownNameSaysSo(t *testing.T) {
	b := newBackend(t, 5173)
	client, _ := start(t, b, false)

	_, err := client.Get("https://karatu.mia:5173/")
	if err == nil {
		t.Fatal("an unknown name was tunnelled somewhere")
	}
	if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("the refusal did not read as a missing name: %v", err)
	}
}

func TestStreamsAreNotBuffered(t *testing.T) {
	b := newBackend(t, 5173)
	client, _ := start(t, b, false)

	response, err := client.Get("https://monduli.mia:5173/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	buffer := make([]byte, 64)
	n, err := response.Body.Read(buffer)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if !strings.Contains(string(buffer[:n]), "data: 0") {
		t.Errorf("the first event did not arrive on its own: %q", string(buffer[:n]))
	}
}

func TestTheAuthorityCannotSignTheInternet(t *testing.T) {
	ca, err := gateway.LoadCA(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pem, err := os.ReadFile(ca.CertPath())
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem)

	leaf, err := ca.Certificate("bank.example.com")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parsed.Verify(x509.VerifyOptions{Roots: pool, DNSName: "bank.example.com"}); err == nil {
		t.Fatal("the authority signed a certificate for a name outside .mia and it verified")
	}

	leaf, err = ca.Certificate("monduli.mia")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ = x509.ParseCertificate(leaf.Certificate[0])
	if _, err := parsed.Verify(x509.VerifyOptions{Roots: pool, DNSName: "monduli.mia"}); err != nil {
		t.Fatalf("a .mia certificate did not verify against its own authority: %v", err)
	}
}

func TestRequestsToOnePortShareTheirConnection(t *testing.T) {
	b := newBackend(t, 5173)
	var dials atomic.Int32
	proxy := &gateway.Proxy{Routes: func() gateway.Table {
		return gateway.Table{"monduli.mia": {
			Host: "monduli.mia", Name: "monduli", Page: 5173,
			Dial: func(ctx context.Context, port int) (net.Conn, error) {
				dials.Add(1)
				return b.dial(ctx, port)
			},
		}}
	}}

	for range 3 {
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, httptest.NewRequest("GET", "http://monduli.mia:5173/", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("answered %d: %s", response.Code, response.Body.String())
		}
	}
	if n := dials.Load(); n != 1 {
		t.Fatalf("3 requests dialed the environment %d times, want 1", n)
	}
}

func TestANameTheCachedTableHasNotSeenIsLookedUpFresh(t *testing.T) {
	b := newBackend(t, 5173)
	route := gateway.Route{Host: "sinza.mia", Name: "sinza", Page: 5173, Dial: b.dial}
	var fresh atomic.Int32
	var mu sync.Mutex
	cached := gateway.Table{}
	proxy := &gateway.Proxy{
		Routes: func() gateway.Table {
			mu.Lock()
			defer mu.Unlock()
			return cached
		},
		Fresh: func() gateway.Table {
			fresh.Add(1)
			mu.Lock()
			defer mu.Unlock()
			cached = gateway.Table{"sinza.mia": route}
			return cached
		},
	}

	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest("GET", "http://sinza.mia:5173/", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("a just-started environment answered %d: %s", response.Code, response.Body.String())
	}
	if n := fresh.Load(); n != 1 {
		t.Fatalf("built the table fresh %d times, want 1", n)
	}
}
