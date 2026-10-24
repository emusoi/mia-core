package container

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ComposeService struct {
	ContainerName string            `json:"container_name"`
	Image         string            `json:"image"`
	NetworkMode   string            `json:"network_mode"`
	Ports         []json.RawMessage `json:"ports"`
}

type ComposeConfig struct {
	Services map[string]ComposeService `json:"services"`
}

func (e Engine) ComposeReady() error {
	_, err := e.run("compose", "version")
	return err
}

func (e Engine) ComposeConfig(project, file string) (ComposeConfig, error) {
	out, err := e.run("compose", "-p", project, "-f", file, "config", "--format", "json")
	if err != nil {
		return ComposeConfig{}, err
	}
	var config ComposeConfig
	if err := json.Unmarshal([]byte(out), &config); err != nil {
		return ComposeConfig{}, fmt.Errorf("compose config was not JSON: %w", err)
	}
	return config, nil
}

func (e Engine) ComposeUp(project, file string) error {
	_, err := e.run("compose", "-p", project, "-f", file, "up", "-d", "--remove-orphans")
	return err
}

func (e Engine) ComposeID(project, file, service string) (string, error) {
	out, err := e.run("compose", "-p", project, "-f", file, "ps", "-q", service)
	if err != nil {
		return "", err
	}
	ids := strings.Fields(out)
	if len(ids) > 1 {
		return "", fmt.Errorf("compose service %s has more than one container; mia needs exactly one", service)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}

func (e Engine) ComposeStop(project, file string) error {
	_, err := e.run("compose", "-p", project, "-f", file, "stop")
	return err
}

func (e Engine) ComposeRemove(project, file string) error {
	_, err := e.run("compose", "-p", project, "-f", file, "down", "--remove-orphans")
	return err
}
