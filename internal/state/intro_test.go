package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIntroRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := WriteIntro(dir, IntroState{LastSeenIntroVersion: "1.2.3"}); err != nil {
		t.Fatalf("WriteIntro: %v", err)
	}
	got, err := ReadIntro(dir)
	if err != nil {
		t.Fatalf("ReadIntro: %v", err)
	}
	if got.LastSeenIntroVersion != "1.2.3" {
		t.Errorf("LastSeenIntroVersion = %q, want 1.2.3", got.LastSeenIntroVersion)
	}
	if got.SchemaVersion != IntroSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, IntroSchemaVersion)
	}
	info, err := os.Stat(IntroPath(dir))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("intro.json mode = %v, want 0600", perm)
	}
}

func TestReadIntroMissingReturnsZero(t *testing.T) {
	got, err := ReadIntro(t.TempDir())
	if err != nil {
		t.Fatalf("ReadIntro(missing) error: %v", err)
	}
	if got != (IntroState{}) {
		t.Errorf("missing file should return zero value, got %+v", got)
	}
}

func TestReadIntroCorruptErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(IntroPath(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadIntro(dir); err == nil {
		t.Fatal("corrupt intro.json should error")
	}
}

func TestResolveDirOverride(t *testing.T) {
	got, err := ResolveDir([]string{"DANGIT_STATE_DIR=/tmp/dangit-state"})
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean("/tmp/dangit-state") {
		t.Errorf("ResolveDir override = %q", got)
	}
}
