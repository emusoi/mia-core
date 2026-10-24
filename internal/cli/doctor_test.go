package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

func TestDoctorReportsHealthyLocalPrerequisitesAsJSON(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\necho arm64\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, err := os.CreateTemp(t.TempDir(), "doctor")
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = out
	code := Run([]string{"doctor", "--json"})
	os.Stdout = stdout
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if code != exitOK {
		t.Fatalf("doctor exited %d", code)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	var report doctorReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("doctor was not JSON: %v\n%s", err, data)
	}
	status := map[string]string{}
	for _, check := range report.Checks {
		status[check.Name] = check.Status
	}
	for _, name := range []string{"git", "tmux", "container engine", "configuration"} {
		if status[name] != "ok" {
			t.Errorf("%s status = %q; report: %+v", name, status[name], report)
		}
	}
	if !report.OK {
		t.Fatalf("healthy local checks produced a failed report: %+v", report)
	}
}

func TestDoctorJSONFailureExitsNonzero(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(realGit, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\necho arm64\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	out, err := os.CreateTemp(t.TempDir(), "doctor-failure")
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = out
	code := Run([]string{"doctor", "--json"})
	os.Stdout = stdout
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if code != exitFailed {
		t.Fatalf("doctor without tmux exited %d, want %d", code, exitFailed)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	var report doctorReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatal("doctor without tmux reported success")
	}
	found := false
	for _, check := range report.Checks {
		found = found || check.Name == "tmux" && check.Status == "fail"
	}
	if !found {
		t.Fatalf("missing tmux was not named: %+v", report)
	}
}
