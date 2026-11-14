// Package instance keeps two SEFIROT processes from using one data directory.
//
// SQLite's WAL mode is unsafe when the same database is opened by processes that
// do not share a lock manager, which is exactly the situation when a Windows
// build and a WSL build point at the same folder. Native file locks do not
// bridge that gap, so the guard is a lock file with a heartbeat that both sides
// can read through the shared filesystem.
package instance

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	lockName          = ".sefirot.lock"
	HeartbeatInterval = 5 * time.Second
	StaleAfter        = 20 * time.Second
)

type record struct {
	PID       int    `json:"pid"`
	Host      string `json:"host"`
	OS        string `json:"os"`
	Token     string `json:"token"`
	Started   string `json:"started"`
	Heartbeat string `json:"heartbeat"`
}

type HeldError struct {
	PID  int
	Host string
	OS   string
	Age  time.Duration
}

func (e *HeldError) Error() string {
	return fmt.Sprintf("this data directory is in use by another SEFIROT process (pid %d on %s, %s, active %s ago). Close it first, or use a different --data-dir. Opening one database from Windows and WSL at the same time can corrupt it.", e.PID, e.Host, e.OS, e.Age.Round(time.Second))
}

type Lock struct {
	path  string
	token string
	stop  chan struct{}
	once  sync.Once
	wg    sync.WaitGroup
	lost  chan struct{}
	now   func() time.Time
}

func newToken() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func read(path string) (record, error) {
	var r record
	raw, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(raw, &r)
	return r, err
}

func (l *Lock) write(started string) error {
	host, _ := os.Hostname()
	rec := record{PID: os.Getpid(), Host: host, OS: runtime.GOOS, Token: l.token, Started: started, Heartbeat: l.now().UTC().Format(time.RFC3339Nano)}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), ".sefirot.lock-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(raw)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return werr
	}
	if err := os.Rename(tmp.Name(), l.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Acquire takes the lock for dir or returns *HeldError when a live process holds it.
func Acquire(dir string) (*Lock, error) { return acquire(dir, time.Now, HeartbeatInterval) }

func acquire(dir string, now func() time.Time, interval time.Duration) (*Lock, error) {
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	l := &Lock{path: filepath.Join(dir, lockName), token: token, stop: make(chan struct{}), lost: make(chan struct{}), now: now}
	if existing, err := read(l.path); err == nil && existing.Token != "" {
		if beat, perr := time.Parse(time.RFC3339Nano, existing.Heartbeat); perr == nil {
			if age := now().Sub(beat); age < StaleAfter && age > -StaleAfter {
				return nil, &HeldError{PID: existing.PID, Host: existing.Host, OS: existing.OS, Age: age}
			}
		}
	}
	started := now().UTC().Format(time.RFC3339Nano)
	if err := l.write(started); err != nil {
		return nil, err
	}
	// Two processes may both have judged the lock stale; the later rename wins,
	// so confirm after a short pause that this process still owns it.
	time.Sleep(150 * time.Millisecond)
	if cur, err := read(l.path); err != nil || cur.Token != token {
		return nil, errors.New("another SEFIROT process claimed this data directory at the same moment")
	}
	l.wg.Add(1)
	go l.beat(started, interval)
	return l, nil
}

func (l *Lock) beat(started string, interval time.Duration) {
	defer l.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			if cur, err := read(l.path); err == nil && cur.Token != l.token {
				select {
				case <-l.lost:
				default:
					close(l.lost)
				}
				return
			}
			_ = l.write(started)
		}
	}
}

// Lost is closed if another process took over the lock while this one was running.
func (l *Lock) Lost() <-chan struct{} { return l.lost }

// Release stops the heartbeat and removes the lock file if this process still owns it.
func (l *Lock) Release() {
	l.once.Do(func() {
		close(l.stop)
		l.wg.Wait()
		if cur, err := read(l.path); err == nil && cur.Token == l.token {
			_ = os.Remove(l.path)
		}
	})
}

// PathWarning describes why a data directory is a poor home for SQLite, or "".
func PathWarning(dir string) string {
	slash := filepath.ToSlash(dir)
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(slash)
		if strings.HasPrefix(lower, "//wsl$/") || strings.HasPrefix(lower, "//wsl.localhost/") {
			return "the data directory is inside a WSL filesystem reached from Windows; use a Windows folder for the Windows build and keep a separate directory for WSL"
		}
		if strings.HasPrefix(slash, "//") {
			return "the data directory is on a network share; SQLite write-ahead logging is unreliable there"
		}
		return ""
	}
	if runtime.GOOS == "linux" && len(slash) > 6 && strings.HasPrefix(slash, "/mnt/") && slash[6] == '/' {
		if raw, err := os.ReadFile("/proc/version"); err == nil && strings.Contains(strings.ToLower(string(raw)), "microsoft") {
			return "the data directory is a Windows drive mounted into WSL; SQLite write-ahead logging is unreliable there and is unsafe if a Windows build opens the same folder. Use a directory on the Linux filesystem"
		}
	}
	return ""
}
