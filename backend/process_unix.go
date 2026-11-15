//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func defaultShellCommand() *exec.Cmd {
	if shell := os.Getenv("SHELL"); shell != "" {
		if _, err := exec.LookPath(shell); err == nil {
			return exec.Command(shell, "-l")
		}
	}
	for _, shell := range []string{"zsh", "bash", "sh"} {
		if path, err := exec.LookPath(shell); err == nil {
			return exec.Command(path, "-l")
		}
	}
	return exec.Command("sh")
}

func terminationSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
