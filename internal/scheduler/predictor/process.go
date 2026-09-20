package predictor

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

var ErrRestartBackoff = errors.New("predictor worker restart backoff active")

type WorkerProcess struct {
	Command string
	Args    []string
	Env     []string
	Backoff time.Duration

	mu        sync.Mutex
	cmd       *exec.Cmd
	starts    int
	nextStart time.Time
}

func (p *WorkerProcess) Ensure(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return nil
	}
	if !p.nextStart.IsZero() && time.Now().Before(p.nextStart) {
		return ErrRestartBackoff
	}
	cmd := exec.CommandContext(ctx, p.Command, p.Args...)
	cmd.Env = append(cmd.Environ(), p.Env...)
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	p.starts++
	go p.wait(cmd)
	return nil
}

func (p *WorkerProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func (p *WorkerProcess) StartCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.starts
}

func (p *WorkerProcess) wait(cmd *exec.Cmd) {
	_ = cmd.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == cmd {
		p.cmd = nil
		p.nextStart = time.Now().Add(p.backoff())
	}
}

func (p *WorkerProcess) backoff() time.Duration {
	if p.Backoff > 0 {
		return p.Backoff
	}
	return time.Second
}
