package env

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/emusoi/mia-core/internal/model"
)

func (m Manager) fingerprint(record model.Record, where Location, image Image) string {
	name := ContainerName(Sanitise(record.Name))
	volumes := m.privateVolumes(record, where, name)
	caches, _ := m.cacheVolumes()
	maps.Copy(volumes, caches)
	payload := struct {
		Image    string            `json:"image"`
		Setup    []string          `json:"setup,omitempty"`
		Volumes  map[string]string `json:"volumes,omitempty"`
		Pass     map[string]string `json:"pass,omitempty"`
		Services []Service         `json:"services,omitempty"`
	}{Image: image.Ref, Setup: m.Settings.Setup, Volumes: volumes, Pass: m.Settings.Passing(), Services: m.services()}
	data, _ := json.Marshal(payload)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (m Manager) drift(record model.Record, where Location, image Image, state string) (bool, error) {
	if state == "" {
		return false, nil
	}
	name := ContainerName(Sanitise(record.Name))
	recorded, err := where.Engine.Label(name, ConfigLabel)
	if err != nil {
		return false, err
	}
	return recorded != m.fingerprint(record, where, image), nil
}

func (m Manager) Drift(record model.Record) (bool, error) {
	where, err := m.Where(record)
	if err != nil {
		return false, err
	}
	image, _ := m.imageFor(where, record)
	name := ContainerName(Sanitise(record.Name))
	return m.drift(record, where, image, where.Engine.State(name))
}
