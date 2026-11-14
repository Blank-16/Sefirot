package instance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSecondProcessIsRefusedWhileFirstIsAlive(t *testing.T) {
	dir := t.TempDir()
	first, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	var held *HeldError
	if _, err := Acquire(dir); !errors.As(err, &held) || held.PID != os.Getpid() {
		t.Fatalf("second acquire must report the holder, got %v", err)
	}
	first.Release()
	if _, statErr := os.Stat(filepath.Join(dir, lockName)); !os.IsNotExist(statErr) {
		t.Fatal("release must remove the lock file")
	}
	again, err := Acquire(dir)
	if err != nil {
		t.Fatalf("lock must be free after release: %v", err)
	}
	again.Release()
}

func TestStaleLockFromCrashedProcessIsTakenOver(t *testing.T) {
	dir := t.TempDir()
	crashed := time.Now().Add(-10 * time.Minute)
	l, err := acquire(dir, func() time.Time { return crashed }, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	close(l.stop)
	l.wg.Wait()
	fresh, err := Acquire(dir)
	if err != nil {
		t.Fatalf("a lock without a recent heartbeat must be reclaimed: %v", err)
	}
	fresh.Release()
}

func TestHeartbeatKeepsLockAliveAndTakeoverIsDetected(t *testing.T) {
	dir := t.TempDir()
	l, err := acquire(dir, time.Now, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	time.Sleep(100 * time.Millisecond)
	rec, err := read(l.path)
	if err != nil {
		t.Fatal(err)
	}
	beat, _ := time.Parse(time.RFC3339Nano, rec.Heartbeat)
	if time.Since(beat) > 80*time.Millisecond {
		t.Fatalf("heartbeat not refreshed: %v ago", time.Since(beat))
	}
	other := &Lock{path: l.path, token: "intruder", now: time.Now}
	deadline := time.After(2 * time.Second)
	for {
		if err := other.write(rec.Started); err != nil {
			t.Fatal(err)
		}
		select {
		case <-l.Lost():
			return
		case <-deadline:
			t.Fatal("owner must notice that another process took the lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestPathWarningOnlyForRiskyLocations(t *testing.T) {
	if w := PathWarning(t.TempDir()); w != "" {
		t.Fatalf("ordinary directory flagged: %q", w)
	}
}
