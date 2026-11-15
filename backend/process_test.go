package main

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestTerminalPTYLifecycle(t *testing.T) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-NoLogo", "-Command", "Write-Output 'sefirot-pty-ok'; Start-Sleep -Seconds 1")
	} else {
		cmd = exec.Command("sh", "-c", "printf 'sefirot-pty-ok\\n'; sleep 1")
	}
	cmd.Dir = t.TempDir()
	cmd.Env = setEnvironmentValue(setEnvironmentValue(setEnvironmentValue(os.Environ(), "TERM", "xterm-256color"), "COLORTERM", "truecolor"), "SEFIROT_URL", "http://127.0.0.1:1234")

	p, err := startTerminalProcess(cmd)
	if err != nil {
		t.Fatalf("start PTY: %v", err)
	}
	if err := p.Resize(40, 120); err != nil {
		_ = p.Close()
		t.Fatalf("resize PTY: %v", err)
	}
	output, readErr := io.ReadAll(p)
	if readErr != nil {
		t.Fatalf("read PTY: %v", readErr)
	}
	code, waitErr := p.Wait()
	if waitErr != nil {
		t.Fatalf("wait PTY: %v", waitErr)
	}
	if code != 0 {
		t.Fatalf("unexpected exit code: %d; output: %s", code, output)
	}
	if !strings.Contains(string(output), "sefirot-pty-ok") {
		t.Fatalf("PTY output missing marker: %q", output)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("close PTY: %v", err)
	}
}
