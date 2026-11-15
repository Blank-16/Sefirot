//go:build windows

package main

import (
	"os"
	"os/exec"
)

func defaultShellCommand() *exec.Cmd {
	for _, candidate := range []struct {
		name string
		args []string
	}{{"pwsh.exe", []string{"-NoLogo"}}, {"powershell.exe", []string{"-NoLogo"}}, {"cmd.exe", nil}} {
		if path, err := exec.LookPath(candidate.name); err == nil {
			return exec.Command(path, candidate.args...)
		}
	}
	return exec.Command("cmd.exe")
}

func terminationSignals() []os.Signal { return []os.Signal{os.Interrupt} }
