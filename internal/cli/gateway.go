package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/emusoi/mia-core/internal/gateway"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/session"
)

func cmdGateway(args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	if devBuild() && sub != "status" {
		return fail(fmt.Errorf("miadev leaves the gateway to mia — `mia gateway %s`", sub))
	}
	switch sub {
	case "serve":
		return gatewayServe()
	case "start":
		if err := EnsureGateway(); err != nil {
			return fail(err)
		}
		fmt.Printf("gateway listening on %s\n", gateway.Addr)
		return exitOK
	case "stop":
		return gatewayStop()
	case "install":
		return gatewayInstall()
	case "uninstall":
		return gatewayUninstall()
	case "status":
		return gatewayStatus()
	case "trust":
		return gatewayTrust()
	case "untrust":
		return gatewayUntrust()
	case "setup":
		return gatewaySetup()
	default:
		return usageErr("mia gateway <status|start|stop|install|uninstall|setup|trust|untrust>")
	}
}

func gatewayServe() int {
	session.UseControl()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := os.WriteFile(pidPath(), []byte(fmt.Sprint(os.Getpid())), 0o644); err != nil {
		return fail(err)
	}
	defer os.Remove(pidPath())
	fmt.Fprintf(os.Stderr, "mia gateway on %s\n", gateway.Addr)
	if err := gateway.Serve(ctx); err != nil {
		return fail(err)
	}
	return exitOK
}

func pidPath() string { return filepath.Join(gateway.StateDir(), "daemon.pid") }

func serviceUnit() string {
	config, err := os.UserConfigDir()
	if runtime.GOOS != "linux" || err != nil {
		return ""
	}
	return filepath.Join(config, "systemd", "user", "mia-gateway.service")
}

func serviceInstalled() bool {
	unit := serviceUnit()
	if unit == "" {
		return false
	}
	_, err := os.Stat(unit)
	return err == nil
}

func systemctl(args ...string) error {
	out, err := run.Local("systemctl").Combined(append([]string{"--user"}, args...)...)
	if err != nil {
		return fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), strings.TrimSpace(out))
	}
	return nil
}

func waitForGateway() error {
	for range 100 {
		if gateway.Running() {
			return nil
		}
		time.Sleep(60 * time.Millisecond)
	}
	return fmt.Errorf("the gateway did not come up — `journalctl --user -u mia-gateway` says why")
}

func gatewayInstall() int {
	unit := serviceUnit()
	if unit == "" {
		return fail(fmt.Errorf("gateway install sets up a systemd user service, which needs Linux — on this machine `mia gateway start` runs it"))
	}
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	body := fmt.Sprintf("[Unit]\nDescription=mia gateway\nAfter=network-online.target\n\n[Service]\nExecStart=%s gateway serve\nRestart=always\nRestartSec=2\nEnvironment=PATH=%s\n\n[Install]\nWantedBy=default.target\n", self, os.Getenv("PATH"))
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(unit, []byte(body), 0o644); err != nil {
		return fail(err)
	}
	stopGateway()
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "mia-gateway"}, {"restart", "mia-gateway"}} {
		if err := systemctl(args...); err != nil {
			return fail(err)
		}
	}
	if err := waitForGateway(); err != nil {
		return fail(err)
	}
	fmt.Printf("gateway installed as a user service — %s\nit starts with this machine and comes back if it stops; `loginctl enable-linger $USER` keeps it running while you are logged out\n", unit)
	return exitOK
}

func gatewayUninstall() int {
	if !serviceInstalled() {
		fmt.Println("no gateway service installed")
		return exitOK
	}
	_ = systemctl("disable", "--now", "mia-gateway")
	if err := os.Remove(serviceUnit()); err != nil {
		return fail(err)
	}
	_ = systemctl("daemon-reload")
	fmt.Println("gateway service removed")
	return exitOK
}

func devBuild() bool {
	self, err := os.Executable()
	return err == nil && filepath.Base(self) == "miadev"
}

func EnsureGateway() error {
	if devBuild() {
		if _, ok := gateway.ServingBuild(); ok {
			return nil
		}
		return fmt.Errorf("no gateway is running — `mia gateway start`; miadev leaves it to mia")
	}
	if serviceInstalled() {
		if serving, ok := gateway.ServingBuild(); ok && serving == gateway.Build() {
			return nil
		}
		if err := systemctl("restart", "mia-gateway"); err != nil {
			return err
		}
		return waitForGateway()
	}
	if serving, ok := gateway.ServingBuild(); ok {
		if serving == gateway.Build() {
			return nil
		}
		fmt.Fprintln(os.Stderr, "mia: replacing a gateway from an older build")
		stopGateway()
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(gateway.StateDir(), 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(gateway.StateDir(), "daemon.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()

	if _, err := run.Local(self).Detached(log, "gateway", "serve"); err != nil {
		return err
	}
	for range 50 {
		if gateway.Running() {
			return nil
		}
		time.Sleep(60 * time.Millisecond)
	}
	return fmt.Errorf("the gateway did not come up — %s", filepath.Join(gateway.StateDir(), "daemon.log"))
}

func gatewayStop() int {
	if serviceInstalled() {
		if err := systemctl("stop", "mia-gateway"); err != nil {
			return fail(err)
		}
		fmt.Println("gateway stopped — it is a service: `mia gateway start` brings it back, and so does the next boot")
		return exitOK
	}
	if !stopGateway() {
		fmt.Println("no gateway running")
		return exitOK
	}
	fmt.Println("gateway stopped")
	return exitOK
}

func stopGateway() bool {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return false
	}
	var pid int
	fmt.Sscan(string(data), &pid)
	if pid > 0 {
		syscall.Kill(pid, syscall.SIGTERM)
	}
	os.Remove(pidPath())
	for range 30 {
		if !gateway.Running() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}

func gatewayStatus() int {
	if !gateway.Running() {
		fmt.Printf("gateway  not running — `mia gateway start`\n")
		return exitOK
	}
	fmt.Printf("gateway   listening on %s\n", gateway.Addr)
	if serving, ok := gateway.ServingBuild(); ok && serving != gateway.Build() {
		fmt.Println("build     a different mia build is serving — `mia gateway start` replaces it with this one")
	}
	fmt.Printf("pac       http://%s/proxy.pac\n", gateway.Addr)
	fmt.Printf("ca        %s  (%s)\n", filepath.Join(gateway.StateDir(), "ca.pem"), trustState())
	table := gateway.LiveRoutes()
	if len(table) == 0 {
		fmt.Println("routes    none — no environment is running")
		return exitOK
	}
	for host := range table {
		fmt.Printf("route     %s\n", host)
	}
	return exitOK
}

func gatewayTrust() int {
	if _, err := gateway.LoadCA(gateway.StateDir()); err != nil {
		return fail(err)
	}
	path := filepath.Join(gateway.StateDir(), "ca.pem")
	if runtime.GOOS != "darwin" {
		fmt.Printf(`mia's certificate authority is at
  %s

It signs names ending in .mia and, by a name constraint inside the certificate,
nothing else. Add it to your browser's trusted authorities to stop the warnings.
`, path)
		return exitOK
	}

	fmt.Printf(`This adds mia's certificate authority to your LOGIN keychain.

  %s

What it can do:   sign certificates for names ending in .mia
What it cannot:   sign anything else — the constraint is in the certificate
Where the key is: %s, readable only by you, and it never leaves this machine
To undo:          mia gateway untrust

macOS will ask for your password. Continue? [y/N] `, path, filepath.Join(gateway.StateDir(), "ca.key"))

	var answer string
	fmt.Scanln(&answer)
	if !strings.EqualFold(strings.TrimSpace(answer), "y") {
		fmt.Println("nothing was changed")
		return exitOK
	}

	home, _ := os.UserHomeDir()
	keychain := filepath.Join(home, "Library", "Keychains", "login.keychain-db")
	if err := run.Local("security").Interactive("add-trusted-cert", "-r", "trustRoot", "-k", keychain, path); err != nil {
		return fail(fmt.Errorf("could not add the certificate: %w", err))
	}
	fmt.Println("trusted. Firefox keeps its own store — Settings → Privacy → Certificates → Import.")
	return exitOK
}

func gatewayUntrust() int {
	path := filepath.Join(gateway.StateDir(), "ca.pem")
	if runtime.GOOS != "darwin" {
		fmt.Printf("remove %s from your browser's trusted authorities\n", path)
		return exitOK
	}
	if err := run.Local("security").Interactive("remove-trusted-cert", path); err != nil {
		return fail(err)
	}
	fmt.Println("removed. Every .mia address will warn again until you trust it once more.")
	return exitOK
}

func trustState() string {
	if runtime.GOOS != "darwin" {
		return "trust it in your browser"
	}
	path := filepath.Join(gateway.StateDir(), "ca.pem")
	if _, err := run.Local("security").Combined("verify-cert", "-c", path); err == nil {
		return "trusted"
	}
	return "not trusted — `mia gateway trust`"
}

func gatewaySetup() int {
	if err := EnsureGateway(); err != nil {
		return fail(err)
	}
	fmt.Printf(`mia gateway is running on %s.

Two things, once:

1. Trust the certificate authority
     mia gateway trust

2. Point the browser at the proxy configuration file
     System Settings → Network → your connection → Details → Proxies
     → Automatic proxy configuration →  http://%s/proxy.pac

   Chrome and Safari follow the system setting. Firefox has its own:
     Settings → Network Settings → Automatic proxy configuration URL

Only names ending in .mia go through mia. Everything else is untouched.
Then any environment is at  https://<name>.mia:<the port it serves on>/
`, gateway.Addr, gateway.Addr)
	return exitOK
}
