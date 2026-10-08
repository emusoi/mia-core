package config

import "github.com/emusoi/mia-core/internal/paths"

func Dir() string { return paths.Dir() }

func GlobalPath() string { return paths.In("config.toml") }

func NamesPath() string { return paths.In("names.json") }

func RuntimesPath() string { return paths.In("runtimes.json") }
