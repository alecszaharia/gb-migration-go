package global_blocks

// Helpers to run the cobra command in-process, capture stdout, and install
// test hooks.

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/spf13/viper"
)

// stdoutMu serialises captureStdout calls: os.Stdout is process-global.
var stdoutMu sync.Mutex

// captureStdout redirects os.Stdout to a pipe while fn runs and returns what
// was written (the tool prints with fmt.Print*). Tests using it must not run
// in parallel with other stdout-writing tests.
func captureStdout(t testing.TB, fn func()) string {
	t.Helper()
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	func() {
		// restore even if fn panics or calls t.FailNow
		defer func() {
			os.Stdout = orig
			_ = w.Close()
		}()
		fn()
	}()

	out := <-done
	_ = r.Close()
	return out
}

// runCommand executes the global_blocks cobra command end-to-end in-process
// with args (e.g. "-d", dsn, "-b", "3"), returning RunE's / PreRunE's error.
// Viper is reset first so flags/env from earlier runs do not leak in.
func runCommand(ctx context.Context, args ...string) error {
	viper.Reset()
	cmd := NewCommand("migrations")
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.ExecuteContext(ctx)
}

// runCommandCapture is runCommand with stdout captured.
func runCommandCapture(t testing.TB, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = runCommand(ctx, args...) })
	return out, err
}

// run executes the command against the fixture schema: "-d <fx.DSN>" is
// prepended to args. Stdout is captured and returned.
func (fx *fixture) run(ctx context.Context, args ...string) (string, error) {
	fx.t.Helper()
	return runCommandCapture(fx.t, ctx, append([]string{"-d", fx.DSN}, args...)...)
}

// setTestHook assigns v to the hook variable *p for the rest of the test and
// restores the previous value in t.Cleanup. Tests that set hooks must not
// run in parallel with other tests of this package.
//
//	setTestHook(t, &testHookBeforeBatchCommit, func(rangeIdx int) error { ... })
func setTestHook[T any](t testing.TB, p *T, v T) {
	t.Helper()
	prev := *p
	*p = v
	t.Cleanup(func() { *p = prev })
}
