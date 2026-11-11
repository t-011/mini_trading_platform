// Package exchangetest starts a real matching engine for tests.
package exchangetest

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// engineCount makes every engine's symbol unique within a test process. A
// Client exits the process when its ring files are replaced, so two engines
// must never reuse one symbol's rings.
var engineCount atomic.Int64

// The engine is built once per test process, into a dir that outlives any one test.
var (
	buildOnce    sync.Once
	engineBinary string
	buildErr     error
)

func buildEngine() (string, error) {
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "exchangetest")
		if err != nil {
			buildErr = err
			return
		}
		engineBinary = filepath.Join(dir, "me")
		build := exec.Command("c++", "-std=c++17", "-O2", "-o", engineBinary, "../../engine/matching_engine.cpp")
		if output, err := build.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build engine: %v\n%s", err, output)
		}
	})
	return engineBinary, buildErr
}

// RemoveBuild deletes the engine binary built for this test process.
func RemoveBuild() {
	if engineBinary != "" {
		os.RemoveAll(filepath.Dir(engineBinary))
	}
}

// Engine is a running engine process serving one symbol.
type Engine struct {
	Symbol   string
	t        *testing.T // the test that started it; owns the engine for its whole life
	ringDir  string
	walDir   string
	process  *exec.Cmd
	exited   chan struct{} // closed once the process has exited
	crashing atomic.Bool   // set by Crash, so its own kill isn't reported
}

// StartEngine builds engine/matching_engine.cpp and serves one throwaway
// symbol, returning it. The engine is killed and its rings (in ringDir, pass
// exchange.RingDir) removed when the test ends. Skips the test if there's no C++ compiler.
//
// Callers must sit two directories below the repo root (internal/<pkg>),
// since the source path is relative to the test's working directory, and
// need a TestMain that calls RemoveBuild after m.Run.
func StartEngine(t *testing.T, ringDir string) string {
	t.Helper()
	return StartEngineWithWAL(t, ringDir, t.TempDir()).Symbol // fresh, empty book every test
}

// StartEngineWithWAL is StartEngine with the WAL dir chosen by the caller,
// for tests that crash the engine and restart it on the same WAL.
func StartEngineWithWAL(t *testing.T, ringDir, walDir string) *Engine {
	t.Helper()
	if _, err := exec.LookPath("c++"); err != nil {
		t.Skip("no C++ compiler")
	}
	engine := &Engine{
		Symbol:  fmt.Sprintf("TEST%d_%d", os.Getpid(), engineCount.Add(1)),
		t:       t,
		ringDir: ringDir,
		walDir:  walDir,
	}
	t.Cleanup(func() {
		engine.Crash()
		os.Remove(filepath.Join(ringDir, engine.Symbol+".in"))
		os.Remove(filepath.Join(ringDir, engine.Symbol+".out"))
	})
	engine.start(t)
	return engine
}

// Crash kills the engine with SIGKILL: no fsync, ring files left behind.
func (engine *Engine) Crash() {
	if engine.process == nil {
		return
	}
	engine.crashing.Store(true)
	engine.process.Process.Kill()
	<-engine.exited
	engine.process = nil
}

// Restart starts a new engine process on the same symbol, rings and WAL. It
// replays the WAL and creates new ring files, so the caller must Dial again.
// It reuses the starting test's t, so a later engine exit is never reported
// on a subtest that has already finished.
func (engine *Engine) Restart() {
	engine.t.Helper()
	engine.Crash()
	engine.start(engine.t)
}

func (engine *Engine) start(t *testing.T) {
	t.Helper()
	binary, err := buildEngine()
	if err != nil {
		t.Fatal(err)
	}
	process := exec.Command(binary, "--serve", engine.Symbol)
	process.Env = append(os.Environ(), "MINI_EXCHANGE_DIR="+engine.ringDir,
		"MINI_EXCHANGE_WAL_DIR="+engine.walDir)
	process.Stderr = os.Stderr // shows why, if it exits before ready
	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}

	// the engine prints "ready" once its rings exist and its threads run
	lines := bufio.NewScanner(stdout)
	for lines.Scan() {
		if lines.Text() != "ready" {
			continue
		}
		engine.process = process
		engine.exited = make(chan struct{})
		engine.crashing.Store(false)
		// an engine dying mid-test would otherwise only show up as timeouts
		go func(exited chan struct{}) {
			status := process.Wait()
			if !engine.crashing.Load() {
				t.Errorf("engine exited: %v", status)
			}
			close(exited)
		}(engine.exited)
		return
	}
	process.Wait()
	t.Fatalf("engine exited before it was ready: %v", lines.Err())
}
