package global_blocks

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestHookHelpersNoOpWhenNil(t *testing.T) {
	if testHookBatchStart != nil || testHookBeforeBatchCommit != nil || testHookSplitStoreStep != nil {
		t.Fatal("test hooks must be nil in production")
	}
	if err := callHookBatchStart(1); err != nil {
		t.Fatalf("callHookBatchStart: %v", err)
	}
	if err := callHookBeforeBatchCommit(1); err != nil {
		t.Fatalf("callHookBeforeBatchCommit: %v", err)
	}
	if err := callHookSplitStoreStep(1); err != nil {
		t.Fatalf("callHookSplitStoreStep: %v", err)
	}
}

func TestHookHelpersCallAndRestore(t *testing.T) {
	boom := errors.New("boom")
	var calls []string

	t.Run("set", func(t *testing.T) {
		setTestHook(t, &testHookBatchStart, func(i int) error { calls = append(calls, fmt.Sprint("start", i)); return nil })
		setTestHook(t, &testHookBeforeBatchCommit, func(i int) error { calls = append(calls, fmt.Sprint("commit", i)); return boom })
		setTestHook(t, &testHookSplitStoreStep, func(i int) error { calls = append(calls, fmt.Sprint("split", i)); return boom })

		if err := callHookBatchStart(2); err != nil {
			t.Fatalf("callHookBatchStart: %v", err)
		}
		if err := callHookBeforeBatchCommit(3); !errors.Is(err, boom) {
			t.Fatalf("callHookBeforeBatchCommit = %v, want boom", err)
		}
		if err := callHookSplitStoreStep(4); !errors.Is(err, boom) {
			t.Fatalf("callHookSplitStoreStep = %v, want boom", err)
		}
	})

	if got, want := strings.Join(calls, ","), "start2,commit3,split4"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
	if testHookBatchStart != nil || testHookBeforeBatchCommit != nil || testHookSplitStoreStep != nil {
		t.Fatal("setTestHook did not restore nil hooks")
	}
}

func TestCaptureStdout(t *testing.T) {
	orig := os.Stdout
	out := captureStdout(t, func() {
		fmt.Print("Progress: ")
		fmt.Println("OK")
		fmt.Printf("\033[F\033[2K%d\n", 42)
	})
	if os.Stdout != orig {
		t.Fatal("os.Stdout not restored")
	}
	if want := "Progress: OK\n\033[F\033[2K42\n"; out != want {
		t.Fatalf("captured %q, want %q", out, want)
	}

	// large output must not deadlock on the pipe buffer
	big := captureStdout(t, func() { fmt.Print(strings.Repeat("x", 1<<20)) })
	if len(big) != 1<<20 {
		t.Fatalf("captured %d bytes, want %d", len(big), 1<<20)
	}
}

func TestRequireDBSkipsWithoutDSN(t *testing.T) {
	if os.Getenv(testDSNEnv) != "" {
		t.Skipf("%s set; skip path not exercised", testDSNEnv)
	}
	ran := false
	t.Run("inner", func(t *testing.T) {
		requireDB(t)
		ran = true
	})
	if ran {
		t.Fatal("requireDB did not skip without DSN")
	}
}
