package env

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/model"
)

var (
	waitEvery   = 250 * time.Millisecond
	waitTimeout = time.Minute
)

func (m Manager) Wait(record model.Record) error {
	timeout := waitTimeout
	for _, service := range m.services() {
		if service.Autostart && service.Grace > 0 {
			timeout = max(timeout, time.Duration(service.Grace)*time.Second)
		}
	}
	deadline := time.Now().Add(timeout)
	for {
		pending, err := m.unready(record)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("environment not ready after %s: %s", timeout, strings.Join(pending, ", "))
		}
		time.Sleep(waitEvery)
	}
}

func (m Manager) unready(record model.Record) ([]string, error) {
	var pending []string
	autostart := map[string]bool{}
	for _, service := range m.services() {
		if service.Autostart {
			autostart[service.ID] = true
		}
	}
	if len(autostart) > 0 {
		statuses, err := m.ServiceStatus(record)
		if err != nil {
			return nil, err
		}
		for _, status := range statuses {
			if !autostart[status.ID] {
				continue
			}
			switch {
			case status.Error != "":
				pending = append(pending, status.ID+" ("+status.Error+")")
			case !status.Running:
				pending = append(pending, status.ID+" (not running)")
			case status.Healthy != nil && !*status.Healthy:
				pending = append(pending, status.ID+" (unhealthy)")
			}
		}
	}
	if len(m.Settings.Ports) > 0 {
		ports, err := m.PortsOf(record)
		if err != nil {
			return nil, err
		}
		for _, port := range m.Settings.Ports {
			if !slices.Contains(ports.Listening, port) {
				pending = append(pending, fmt.Sprintf("port %d", port))
			}
		}
	}
	return pending, nil
}
