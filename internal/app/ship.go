package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/emusoi/mia-core/internal/runtime"
)

// shipped is where a machine keeps which build of mia it was given last.
const shippedStamp = "~/.local/share/mia/shipped"

const installScript = `mkdir -p ~/.local/bin ~/.local/share/mia &&
cat > ~/.local/bin/mia.new && chmod +x ~/.local/bin/mia.new &&
mv ~/.local/bin/mia.new ~/.local/bin/mia && printf %s "$1" > ` + shippedStamp

// Ship makes the machine run this build of mia, built for its platform, so
// a worktree there is run by the same mia as here. It does nothing when the
// machine already has this build.
func Ship(machine runtime.Machine) error {
	stamp, err := buildStamp()
	if err != nil {
		return err
	}
	out, err := runtime.Run(machine.SSH, "sh", "-c", "uname -sm; cat "+shippedStamp+" 2>/dev/null; true")
	if err != nil {
		return err
	}
	platform, had, _ := strings.Cut(strings.TrimSpace(out), "\n")
	if had == stamp {
		return nil
	}
	goos, goarch, err := platformOf(platform)
	if err != nil {
		return fmt.Errorf("%s: %w", machine.Name, err)
	}
	binary, done, err := binaryFor(goos, goarch)
	if err != nil {
		return err
	}
	defer done()
	file, err := os.Open(binary)
	if err != nil {
		return err
	}
	defer file.Close()
	fmt.Fprintf(os.Stderr, "shipping this mia to %s (%s/%s)…\n", machine.Name, goos, goarch)
	install := runtime.Command(machine.SSH, "sh", "-c", installScript, "sh", stamp)
	install.Stdin = file
	if out, err := install.CombinedOutput(); err != nil {
		return fmt.Errorf("install mia on %s: %v %s", machine.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// buildStamp names this build of mia: rebuilding it changes the stamp.
func buildStamp() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %d %d", path, info.Size(), info.ModTime().UnixNano()), nil
}

func platformOf(uname string) (goos, goarch string, err error) {
	fields := strings.Fields(uname)
	if len(fields) != 2 {
		return "", "", fmt.Errorf("cannot read the platform from %q", uname)
	}
	goos = map[string]string{"Linux": "linux", "Darwin": "darwin"}[fields[0]]
	goarch = map[string]string{"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[fields[1]]
	if goos == "" || goarch == "" {
		return "", "", fmt.Errorf("mia is not built for %s", uname)
	}
	return goos, goarch, nil
}

// binaryFor is this mia when the platform is this one; otherwise it is built
// from the checkout this mia was built from.
// ponytail: needs that checkout and go here; a release per platform lifts that.
func binaryFor(goos, goarch string) (string, func(), error) {
	if goos == goruntime.GOOS && goarch == goruntime.GOARCH {
		path, err := os.Executable()
		return path, func() {}, err
	}
	_, here, _, ok := goruntime.Caller(0)
	source := filepath.Dir(filepath.Dir(filepath.Dir(here)))
	if _, err := os.Stat(filepath.Join(source, "go.mod")); !ok || err != nil {
		return "", nil, fmt.Errorf("cannot build mia for %s/%s: its source is not on this computer", goos, goarch)
	}
	temp, err := os.CreateTemp("", "mia-ship-*")
	if err != nil {
		return "", nil, err
	}
	temp.Close()
	done := func() { os.Remove(temp.Name()) }
	build := exec.Command("go", "build", "-o", temp.Name(), "./cmd/mia")
	build.Dir = source
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
	if out, err := build.CombinedOutput(); err != nil {
		done()
		return "", nil, fmt.Errorf("build mia for %s/%s in %s: %v %s", goos, goarch, source, err, strings.TrimSpace(string(out)))
	}
	return temp.Name(), done, nil
}
