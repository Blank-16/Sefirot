package main

import (
	"errors"
	"os/exec"
	"time"

	"github.com/Kodecable/crosspty"
)

type terminalProcess interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	Wait() (int, error)
	Resize(rows, cols uint16) error
}

type crossTerminalProcess struct{ pty crosspty.Pty }

func startTerminalProcess(cmd *exec.Cmd) (terminalProcess, error) {
	cfg := crosspty.CommandConfig{
		Argv: cmd.Args,
		Dir:  cmd.Dir,
		Env:  cmd.Env,
		Size: crosspty.TermSize{Rows: 30, Cols: 100},
		CloseConfig: crosspty.CloseConfig{
			CloseTimeout: 4 * time.Second,
			KillDelay:    2 * time.Second,
			KillMode:     crosspty.KillModeKillGroupOnClose,
			KillExitCode: 1,
		},
	}
	pty, err := crosspty.Start(cfg)
	if err != nil {
		return nil, err
	}
	return &crossTerminalProcess{pty: pty}, nil
}

func (p *crossTerminalProcess) Read(b []byte) (int, error)  { return p.pty.Read(b) }
func (p *crossTerminalProcess) Write(b []byte) (int, error) { return p.pty.Write(b) }
func (p *crossTerminalProcess) Close() error                { return p.pty.Close() }
func (p *crossTerminalProcess) Wait() (int, error) {
	code := p.pty.Wait()
	if code < 0 {
		return code, errors.New("terminal process exited without a retrievable exit code")
	}
	return code, nil
}
func (p *crossTerminalProcess) Resize(rows, cols uint16) error {
	return p.pty.Resize(crosspty.TermSize{Rows: rows, Cols: cols})
}

func terminalBackendAvailable() bool { return true }
