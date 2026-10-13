package registrationtest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	path := filepath.Join(repoRoot(t), rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return b
}

func mutate(src []byte, off int, v byte) []byte {
	out := append([]byte(nil), src...)
	out[off] = v
	return out
}
