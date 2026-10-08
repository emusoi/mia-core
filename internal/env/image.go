package env

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Source string

const (
	FromDevcontainer Source = "devcontainer.json"
	FromConfig       Source = "[env] image"
	FromStack        Source = "detected stack"
	FromDefault      Source = "default base"
	FromBuild        Source = "built by `mia env image`"
)

type Image struct {
	Ref    string `json:"ref"`
	Source Source `json:"source"`
}

const DefaultImage = "mcr.microsoft.com/devcontainers/base:ubuntu"

func DiscoverImage(worktree string, configured string) Image {
	if configured != "" {
		return Image{Ref: configured, Source: FromConfig}
	}
	if ref := fromDevcontainer(worktree); ref != "" {
		return Image{Ref: ref, Source: FromDevcontainer}
	}
	if ref := fromStack(worktree); ref != "" {
		return Image{Ref: ref, Source: FromStack}
	}
	return Image{Ref: DefaultImage, Source: FromDefault}
}

var (
	lineComments  = regexp.MustCompile(`(?m)^\s*//.*$`)
	trailingComma = regexp.MustCompile(`,(\s*[}\]])`)
)

func fromDevcontainer(worktree string) string {
	for _, candidate := range []string{
		filepath.Join(worktree, ".devcontainer", "devcontainer.json"),
		filepath.Join(worktree, ".devcontainer.json"),
	} {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		cleaned := trailingComma.ReplaceAllString(lineComments.ReplaceAllString(string(data), ""), "$1")
		var parsed struct {
			Image string `json:"image"`
		}
		if json.Unmarshal([]byte(cleaned), &parsed) == nil && parsed.Image != "" {
			return parsed.Image
		}
	}
	return ""
}

var stacks = []struct {
	marker string
	image  string
}{
	{"go.mod", "docker.io/library/golang:1-bookworm"},
	{"Cargo.toml", "docker.io/library/rust:1-bookworm"},
	{"pnpm-lock.yaml", "mcr.microsoft.com/devcontainers/typescript-node:22"},
	{"package-lock.json", "mcr.microsoft.com/devcontainers/typescript-node:22"},
	{"yarn.lock", "mcr.microsoft.com/devcontainers/typescript-node:22"},
	{"pyproject.toml", "mcr.microsoft.com/devcontainers/python:3"},
	{"requirements.txt", "mcr.microsoft.com/devcontainers/python:3"},
	{"Gemfile", "docker.io/library/ruby:3"},
}

func fromStack(worktree string) string {
	for _, stack := range stacks {
		if _, err := os.Stat(filepath.Join(worktree, stack.marker)); err == nil {
			return stack.image
		}
	}
	return ""
}

func ContainerName(worktree string) string { return "mia-" + worktree }

const Label = "dev.mia.worktree"

func Sanitise(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
}
