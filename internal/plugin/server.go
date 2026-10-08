package plugin

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

var (
	FirstBackoff = time.Second
	MaxBackoff   = time.Minute
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type reply struct {
	result json.RawMessage
	err    error
}

type Server struct {
	plugin  Plugin
	context Context
	refresh func()

	mu      sync.Mutex
	stdin   io.WriteCloser
	process *os.Process
	next    int
	pending map[int]chan reply
	problem string
	stopped bool
	done    chan struct{}
}

func (p Plugin) Serve(c Context, refresh func()) *Server {
	s := &Server{plugin: p, context: c, refresh: refresh, pending: map[int]chan reply{}, done: make(chan struct{})}
	go s.supervise()
	return s
}

func (s *Server) supervise() {
	defer close(s.done)
	backoff := FirstBackoff
	for {
		started := time.Now()
		err := s.runOnce()
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}
		s.problem = fmt.Sprintf("serve stopped (%v); starting again in %s", err, backoff)
		s.mu.Unlock()
		if time.Since(started) > MaxBackoff {
			backoff = FirstBackoff
		}
		time.Sleep(backoff)
		backoff = min(backoff*2, MaxBackoff)
		s.mu.Lock()
		stopped := s.stopped
		s.mu.Unlock()
		if stopped {
			return
		}
	}
}

func (s *Server) runOnce() error {
	env, err := s.plugin.Env(s.context)
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(s.plugin.DataDir(s.context), "serve.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(s.plugin.Path, "serve")
	cmd.Env = env
	cmd.Stderr = log
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.mu.Lock()
	s.stdin, s.process, s.problem = stdin, cmd.Process, ""
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		_ = cmd.Process.Kill()
	}

	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for lines.Scan() {
		var message rpcMessage
		if json.Unmarshal(lines.Bytes(), &message) != nil {
			continue
		}
		switch {
		case message.ID != nil:
			s.deliver(*message.ID, message)
		case message.Method == "refresh" && s.refresh != nil:
			s.refresh()
		}
	}
	err = cmd.Wait()
	s.mu.Lock()
	s.stdin, s.process = nil, nil
	for id, waiting := range s.pending {
		waiting <- reply{err: errors.New("serve stopped before answering")}
		delete(s.pending, id)
	}
	s.mu.Unlock()
	if err == nil {
		err = errors.New("serve exited")
	}
	return err
}

func (s *Server) deliver(id int, message rpcMessage) {
	s.mu.Lock()
	waiting, ok := s.pending[id]
	delete(s.pending, id)
	s.mu.Unlock()
	if !ok {
		return
	}
	if message.Error != nil {
		waiting <- reply{err: fmt.Errorf("%s: %s", s.plugin.Name, message.Error.Message)}
		return
	}
	waiting <- reply{result: message.Result}
}

func (s *Server) Call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	deadline := time.Now().Add(timeout)
	s.mu.Lock()
	for s.stdin == nil {
		problem := s.problem
		s.mu.Unlock()
		if problem != "" || time.Now().After(deadline) {
			if problem == "" {
				problem = "serve did not start in " + timeout.String()
			}
			return nil, errors.New(problem)
		}
		time.Sleep(20 * time.Millisecond)
		s.mu.Lock()
	}
	s.next++
	id := s.next
	waiting := make(chan reply, 1)
	s.pending[id] = waiting
	data, err := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err == nil {
		_, err = s.stdin.Write(append(data, '\n'))
	}
	if err != nil {
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()
	select {
	case got := <-waiting:
		return got.result, got.err
	case <-time.After(time.Until(deadline)):
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, fmt.Errorf("serve took longer than %s to answer %s", timeout, method)
	}
}

func (s *Server) Stop() {
	s.mu.Lock()
	s.stopped = true
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	process := s.process
	s.mu.Unlock()
	if process != nil {
		go func() {
			time.Sleep(time.Second)
			_ = process.Kill()
		}()
	}
	<-s.done
}

func (s *Server) Notify(method string, params any) error {
	data, err := json.Marshal(rpcMessage{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stdin == nil {
		return errors.New("serve is not running")
	}
	_, err = s.stdin.Write(append(data, '\n'))
	return err
}
