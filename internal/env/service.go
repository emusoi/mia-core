package env

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
)

type Service struct {
	ID        string   `toml:"id" json:"id"`
	Run       []string `toml:"run" json:"run"`
	Health    []string `toml:"health" json:"health,omitempty"`
	Autostart bool     `toml:"autostart" json:"autostart"`
	Restart   string   `toml:"restart" json:"restart,omitempty"`
	Grace     int      `toml:"grace" json:"grace,omitempty"`
}

const (
	RestartNever     = ""
	RestartOnFailure = "on-failure"
	RestartUnhealthy = "on-unhealthy"

	defaultGrace  = 60
	healthStrikes = 3
)

var healthEvery = 10

const runtimeDir = "/run/mia"

func (s Service) Validate() error {
	switch s.Restart {
	case RestartNever, RestartOnFailure:
		return nil
	case RestartUnhealthy:
		if len(s.Health) == 0 {
			return fmt.Errorf("service %s: restart = \"on-unhealthy\" needs a `health` command to judge by", s.ID)
		}
		return nil
	}
	return fmt.Errorf("service %s: restart is \"on-failure\", \"on-unhealthy\" or absent, not %q", s.ID, s.Restart)
}

func (s Service) Script() string {
	pid := shellJoin([]string{runtimeDir + "/" + s.ID + ".pid"})
	log := shellJoin([]string{runtimeDir + "/" + s.ID + ".log"})
	run := shellJoin(s.Run)
	if s.Restart == RestartNever {
		return fmt.Sprintf("mkdir -p %s && echo $$ > %s && exec %s >> %s 2>&1", runtimeDir, pid, run, log)
	}
	grace := s.Grace
	if grace == 0 {
		grace = defaultGrace
	}
	watch := "while kill -0 $child 2>/dev/null; do sleep 1; done"
	if s.Restart == RestartUnhealthy {
		watch = fmt.Sprintf(`started=$(date +%%s); bad=0
  while kill -0 $child 2>/dev/null; do
    sleep %d
    [ $(( $(date +%%s) - started )) -lt %d ] && continue
    if %s >/dev/null 2>&1; then bad=0; else bad=$((bad+1)); fi
    if [ $bad -ge %d ]; then echo "mia: not answering %d times, restarting" >> %s; kill $child 2>/dev/null; break; fi
  done`, healthEvery, grace, shellJoin(s.Health), healthStrikes, healthStrikes, log)
	}
	return fmt.Sprintf(`mkdir -p %s && echo $$ > %s
trap 'kill $child 2>/dev/null; exit 0' TERM INT
while :; do
  %s >> %s 2>&1 &
  child=$!
  %s
  wait $child; code=$?
  [ $code -eq 0 ] && exit 0
  echo "mia: exited $code, restarting" >> %s
  sleep 2
done`, runtimeDir, pid, run, log, watch, log)
}

type Status struct {
	ID      string   `json:"id"`
	Run     []string `json:"run,omitempty"`
	Running bool     `json:"running"`
	Healthy *bool    `json:"healthy,omitempty"`
	Error   string   `json:"error,omitempty"`
}

const hostGateway = "host.containers.internal"

func hostForward(port int) Service {
	at := strconv.Itoa(port)
	return Service{
		ID:        "host-" + at,
		Run:       []string{"socat", "TCP-LISTEN:" + at + ",fork,reuseaddr,bind=127.0.0.1", "TCP:" + hostGateway + ":" + at},
		Autostart: true,
	}
}

func (m Manager) services() []Service {
	all := append([]Service(nil), m.Services...)
	for _, port := range m.Settings.HostPorts {
		all = append(all, hostForward(port))
	}
	return all
}

func (m Manager) serviceByID(id string) (Service, bool) {
	for _, service := range m.services() {
		if service.ID == id {
			return service, true
		}
	}
	return Service{}, false
}

func (m Manager) StartService(record model.Record, id string) error {
	service, ok := m.serviceByID(id)
	if !ok {
		return fmt.Errorf("no service called %q — `mia env service` lists them", id)
	}
	where, name, err := m.running(record)
	if err != nil {
		return err
	}
	if running, _ := m.serviceRunning(where, name, id); running {
		return nil
	}

	if err := service.Validate(); err != nil {
		return err
	}
	return where.Engine.ExecDetached(name, where.Dir, []string{"sh", "-c", service.Script()})
}

func (m Manager) StopService(record model.Record, id string) error {
	where, name, err := m.running(record)
	if err != nil {
		return nil
	}
	pid := shellJoin([]string{runtimeDir + "/" + id + ".pid"})
	_, err = where.Engine.Exec(name, "sh", "-c",
		fmt.Sprintf("[ -f %s ] && kill $(cat %s) 2>/dev/null; rm -f %s; true", pid, pid, pid))
	return err
}

func (m Manager) Logs(record model.Record, id string, lines int) (string, error) {
	where, name, err := m.running(record)
	if err != nil {
		return "", err
	}
	log := shellJoin([]string{runtimeDir + "/" + id + ".log"})
	return where.Engine.Exec(name, "sh", "-c",
		fmt.Sprintf("if [ -s %s ]; then tail -n %d %s; else printf '%%s\\n' %s; fi",
			log, lines, log, shellJoin([]string{"(" + id + " has printed nothing)"})))
}

func (m Manager) serviceRunning(where Location, container, id string) (bool, error) {
	pid := shellJoin([]string{runtimeDir + "/" + id + ".pid"})
	out, err := where.Engine.Exec(container, "sh", "-c",
		fmt.Sprintf("[ -f %s ] && kill -0 $(cat %s) 2>/dev/null && echo yes || echo no", pid, pid))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "yes", nil
}

func (m Manager) ServiceStatus(record model.Record) ([]Status, error) {
	where, err := m.Where(record)
	if err != nil {
		return nil, err
	}
	name := ContainerName(Sanitise(record.Name))
	return m.serviceStatuses(where, name, where.Engine.State(name) == "running"), nil
}

func (m Manager) serviceStatuses(where Location, name string, up bool) []Status {
	services := m.services()
	statuses := make([]Status, 0, len(services))

	for _, service := range services {
		status := Status{ID: service.ID, Run: service.Run}
		if up {
			running, err := m.serviceRunning(where, name, service.ID)
			status.Running = running
			if err != nil {
				status.Error = err.Error()
			} else if status.Running && len(service.Health) > 0 {
				exit, output := where.Engine.ExecStatus(name, where.Dir, []string{"sh", "-c", shellJoin(service.Health)})
				if exit < 0 {
					status.Error = strings.TrimSpace(output)
					if status.Error == "" {
						status.Error = fmt.Sprintf("health check for %s did not complete", service.ID)
					}
				} else {
					healthy := exit == 0
					status.Healthy = &healthy
				}
			}
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].ID < statuses[j].ID })
	return statuses
}

func (m Manager) StartAutostart(record model.Record) []error {
	var problems []error
	for _, service := range m.services() {
		if !service.Autostart {
			continue
		}
		if err := m.StartService(record, service.ID); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", service.ID, err))
		}
	}
	return problems
}

func shellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
