package wuffs_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const wasm2goHost = "github.com/lbe/wasm2go-wasi-host"

// TestGoModuleScaffoldCompiles verifies that the Go module scaffold compiles
// with wasm2go-generated guest code after the wasm rebuild prerequisite.
func TestGoModuleScaffoldCompiles(t *testing.T) {
	t.Run("go.mod has wasm2go-wasi-host as direct require", func(t *testing.T) {
		t.Helper()
		data, err := os.ReadFile("go.mod")
		if err != nil {
			t.Fatalf("reading go.mod: %v", err)
		}
		mod := string(data)

		if !strings.Contains(mod, wasm2goHost) {
			t.Errorf("go.mod missing require for %s", wasm2goHost)
		}
		for _, line := range strings.Split(mod, "\n") {
			if strings.Contains(line, wasm2goHost) && strings.Contains(line, "// indirect") {
				t.Errorf("%s must be a direct require, not indirect: %s", wasm2goHost, line)
			}
		}
	})

	t.Run("generated wasm2go guest code exists", func(t *testing.T) {
		t.Helper()
		const guest = "internal/wuffswasm/wuffs.go"
		if _, err := os.Stat(guest); os.IsNotExist(err) {
			t.Errorf("%s does not exist (wasm2go output not generated)", guest)
		}
	})

	t.Run("go build ./... succeeds", func(t *testing.T) {
		t.Helper()
		cmd := exec.Command("go", "build", "./...")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("go build ./... failed: %v\nOutput: %s", err, out)
		}
	})
}
