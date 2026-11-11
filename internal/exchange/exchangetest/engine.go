// Package exchangetest starts a real matching engine for tests.
package exchangetest

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// StartEngine builds engine/matching_engine.cpp and serves one throwaway
// symbol, returning it. The engine is killed and its rings (in ringDir, pass
// exchange.RingDir) removed when the test ends. Skips the test if there's no C++ compiler.
//
// Callers must sit two directories below the repo root (internal/<pkg>),
// since the source path is relative to the test's working directory.
func StartEngine(t *testing.T, ringDir string) string {
	t.Helper()
	if _, err := exec.LookPath("c++"); err != nil {
		t.Skip("no C++ compiler")
	}
	engineBinary := filepath.Join(t.TempDir(), "me")
	build := exec.Command("c++", "-std=c++17", "-O2", "-o", engineBinary, "../../engine/matching_engine.cpp")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build engine: %v\n%s", err, output)
	}

	symbol := fmt.Sprintf("TEST%d", os.Getpid())
	engine := exec.Command(engineBinary, "--serve", symbol)
	engine.Env = append(os.Environ(), "MINI_EXCHANGE_DIR="+ringDir)
	stdout, _ := engine.StdoutPipe()
	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		engine.Process.Kill()
		engine.Wait()
		os.Remove(filepath.Join(ringDir, symbol+".in"))
		os.Remove(filepath.Join(ringDir, symbol+".out"))
	})

	// the engine prints "ready" once its rings exist and its threads run
	lines := bufio.NewScanner(stdout)
	for lines.Scan() {
		if lines.Text() == "ready" {
			return symbol
		}
	}
	t.Fatalf("engine exited before it was ready: %v", lines.Err())
	return ""
}
