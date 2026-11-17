// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundlemain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/tast/core/errors"

	"go.chromium.org/tast-tests/cros/common/hwsec"
)

type fakeCmdRunner struct {
	hwsec.CmdRunner
	run   func(ctx context.Context, cmd string, args ...string) ([]byte, error)
	calls atomic.Int32
}

func (f *fakeCmdRunner) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	f.calls.Add(1)
	return f.run(ctx, cmd, args...)
}

func TestMainfwTypeCache(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
	}{
		{name: "recovery", out: "recovery"},
		{name: "normal", out: "normal"},
		{name: "empty", out: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cache mainfwTypeCache
			runner := &fakeCmdRunner{
				run: func(ctx context.Context, cmd string, args ...string) ([]byte, error) {
					if cmd != "crossystem" || len(args) != 1 || args[0] != "mainfw_type" {
						return nil, errors.Errorf("unexpected command %q %v", cmd, args)
					}
					return []byte(tc.out), nil
				},
			}

			for i := 0; i < 2; i++ {
				got, err := cache.get(t.Context(), runner)
				if err != nil || got != tc.out {
					t.Errorf("call %d: cache.get() = (%q, %v), want (%q, nil)", i+1, got, err, tc.out)
				}
			}
			if got := runner.calls.Load(); got != 1 {
				t.Errorf("runner calls = %d, want 1", got)
			}
		})
	}
}

func TestMainfwTypeCacheErrorNotCached(t *testing.T) {
	var cache mainfwTypeCache
	wantErr := errors.New("transient failure")
	failFirst := true
	runner := &fakeCmdRunner{
		run: func(ctx context.Context, cmd string, args ...string) ([]byte, error) {
			if failFirst {
				failFirst = false
				return nil, wantErr
			}
			return []byte("normal"), nil
		},
	}

	if got, err := cache.get(t.Context(), runner); !errors.Is(err, wantErr) || got != "" {
		t.Errorf("first cache.get() = (%q, %v), want (%q, %v)", got, err, "", wantErr)
	}
	for i := 0; i < 2; i++ {
		got, err := cache.get(t.Context(), runner)
		if err != nil || got != "normal" {
			t.Errorf("retry %d: cache.get() = (%q, %v), want (%q, nil)", i+1, got, err, "normal")
		}
	}
	if got := runner.calls.Load(); got != 2 {
		t.Errorf("runner calls = %d, want 2", got)
	}
}

func TestMainfwTypeCacheConcurrentCalls(t *testing.T) {
	var cache mainfwTypeCache
	runner := &fakeCmdRunner{
		run: func(ctx context.Context, cmd string, args ...string) ([]byte, error) {
			return []byte("normal"), nil
		},
	}

	const workers = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			got, err := cache.get(t.Context(), runner)
			if err != nil || got != "normal" {
				t.Errorf("cache.get() = (%q, %v), want (%q, nil)", got, err, "normal")
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := runner.calls.Load(); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

func TestInterTestGC(t *testing.T) {
	var gc interTestGC
	calls := 0
	fn := func() {
		calls++
	}

	gc.beforeTest(fn)
	if calls != 0 {
		t.Fatalf("beforeTest() call 1: gc calls = %d, want 0", calls)
	}

	gc.beforeTest(fn)
	if calls != 1 {
		t.Fatalf("beforeTest() call 2: gc calls = %d, want 1", calls)
	}

	gc.beforeTest(fn)
	if calls != 2 {
		t.Fatalf("beforeTest() call 3: gc calls = %d, want 2", calls)
	}
}

func TestInterTestGCConcurrent(t *testing.T) {
	var gc interTestGC
	var calls atomic.Int32

	const workers = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			gc.beforeTest(func() {
				calls.Add(1)
			})
		}()
	}
	close(start)
	wg.Wait()

	if got, want := int(calls.Load()), workers-1; got != want {
		t.Fatalf("concurrent beforeTest gc calls = %d, want %d", got, want)
	}
}

func createTestFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to create %s: %v", p, err)
	}
	return p
}

func stubFreeSpace(values ...uint64) (func() (uint64, error), *int) {
	calls := 0
	fn := func() (uint64, error) {
		idx := calls
		calls++
		if idx >= len(values) {
			return 0, errors.Errorf("unexpected freeSpace call %d (only %d configured)", calls, len(values))
		}
		return values[idx], nil
	}
	return fn, &calls
}

func TestEnsureDiskSpaceNoPurgeNeeded(t *testing.T) {
	const threshold = uint64(1000)
	for _, tc := range []struct {
		name string
		free uint64
	}{
		{name: "above_threshold", free: 1500},
		{name: "at_threshold", free: threshold},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deleteCoreCalls := 0
			purgeableCalls := 0
			freeSpace, freeCalls := stubFreeSpace(tc.free)

			got, err := ensureDiskSpaceWith(
				context.Background(),
				func(context.Context) error {
					deleteCoreCalls++
					return nil
				},
				freeSpace,
				threshold,
				func() []string {
					purgeableCalls++
					return nil
				},
			)
			if err != nil {
				t.Fatalf("ensureDiskSpaceWith() err = %v, want nil", err)
			}
			if got != tc.free {
				t.Errorf("ensureDiskSpaceWith() = %d, want %d", got, tc.free)
			}
			if deleteCoreCalls != 1 {
				t.Errorf("deleteCoreDumps calls = %d, want 1", deleteCoreCalls)
			}
			if *freeCalls != 1 {
				t.Errorf("freeSpace calls = %d, want 1", *freeCalls)
			}
			if purgeableCalls != 0 {
				t.Errorf("purgeable calls = %d, want 0", purgeableCalls)
			}
		})
	}
}

func TestEnsureDiskSpaceInitialFreeSpaceError(t *testing.T) {
	wantErr := errors.New("statfs failed")
	deleteCoreCalls := 0
	purgeableCalls := 0

	got, err := ensureDiskSpaceWith(
		context.Background(),
		func(context.Context) error {
			deleteCoreCalls++
			return nil
		},
		func() (uint64, error) {
			return 0, wantErr
		},
		1000,
		func() []string {
			purgeableCalls++
			return nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Errorf("ensureDiskSpaceWith() err = %v, want %v", err, wantErr)
	}
	if got != 0 {
		t.Errorf("ensureDiskSpaceWith() = %d, want 0", got)
	}
	if deleteCoreCalls != 1 {
		t.Errorf("deleteCoreDumps calls = %d, want 1", deleteCoreCalls)
	}
	if purgeableCalls != 0 {
		t.Errorf("purgeable calls = %d, want 0", purgeableCalls)
	}
}

func TestEnsureDiskSpacePurgesUntilThreshold(t *testing.T) {
	dir := t.TempDir()
	f1 := createTestFile(t, dir, "f1")
	f2 := createTestFile(t, dir, "f2")
	f3 := createTestFile(t, dir, "f3")

	freeSpace, freeCalls := stubFreeSpace(400, 800, 1000)
	purgeableCalls := 0

	got, err := ensureDiskSpaceWith(
		context.Background(),
		func(context.Context) error { return nil },
		freeSpace,
		1000,
		func() []string {
			purgeableCalls++
			return []string{f1, f2, f3}
		},
	)
	if err != nil {
		t.Fatalf("ensureDiskSpaceWith() err = %v, want nil", err)
	}
	if got != 1000 {
		t.Errorf("ensureDiskSpaceWith() = %d, want 1000", got)
	}
	if purgeableCalls != 1 {
		t.Errorf("purgeable calls = %d, want 1", purgeableCalls)
	}
	if *freeCalls != 3 {
		t.Errorf("freeSpace calls = %d, want 3", *freeCalls)
	}
	for _, p := range []string{f1, f2} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("os.Stat(%q) err = %v, want %v", p, err, os.ErrNotExist)
		}
	}
	if _, err := os.Stat(f3); err != nil {
		t.Errorf("os.Stat(%q) err = %v, want nil", f3, err)
	}
}

func TestEnsureDiskSpaceDeleteCoreDumpsErrorContinues(t *testing.T) {
	dir := t.TempDir()
	f1 := createTestFile(t, dir, "f1")

	freeSpace, freeCalls := stubFreeSpace(500, 1200)
	purgeableCalls := 0

	got, err := ensureDiskSpaceWith(
		context.Background(),
		func(context.Context) error { return errors.New("core dump error") },
		freeSpace,
		1000,
		func() []string {
			purgeableCalls++
			return []string{f1}
		},
	)
	if err != nil {
		t.Fatalf("ensureDiskSpaceWith() err = %v, want nil", err)
	}
	if got != 1200 {
		t.Errorf("ensureDiskSpaceWith() = %d, want 1200", got)
	}
	if purgeableCalls != 1 {
		t.Errorf("purgeable calls = %d, want 1", purgeableCalls)
	}
	if *freeCalls != 2 {
		t.Errorf("freeSpace calls = %d, want 2", *freeCalls)
	}
	if _, err := os.Stat(f1); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(%q) err = %v, want %v", f1, err, os.ErrNotExist)
	}
}

func TestEnsureDiskSpaceEmptyPurgeable(t *testing.T) {
	freeSpace, freeCalls := stubFreeSpace(500)
	purgeableCalls := 0

	got, err := ensureDiskSpaceWith(
		context.Background(),
		func(context.Context) error { return nil },
		freeSpace,
		1000,
		func() []string {
			purgeableCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ensureDiskSpaceWith() err = %v, want nil", err)
	}
	if got != 500 {
		t.Errorf("ensureDiskSpaceWith() = %d, want 500", got)
	}
	if purgeableCalls != 1 {
		t.Errorf("purgeable calls = %d, want 1", purgeableCalls)
	}
	if *freeCalls != 1 {
		t.Errorf("freeSpace calls = %d, want 1", *freeCalls)
	}
}

func TestEnsureDiskSpaceRemoveErrorContinues(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	f1 := createTestFile(t, dir, "f1")

	freeSpace, freeCalls := stubFreeSpace(500, 1100)

	got, err := ensureDiskSpaceWith(
		context.Background(),
		func(context.Context) error { return nil },
		freeSpace,
		1000,
		func() []string {
			return []string{missing, f1}
		},
	)
	if err != nil {
		t.Fatalf("ensureDiskSpaceWith() err = %v, want nil", err)
	}
	if got != 1100 {
		t.Errorf("ensureDiskSpaceWith() = %d, want 1100", got)
	}
	if *freeCalls != 2 {
		t.Errorf("freeSpace calls = %d, want 2", *freeCalls)
	}
	if _, err := os.Stat(f1); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(%q) err = %v, want %v", f1, err, os.ErrNotExist)
	}
}

func TestEnsureDiskSpaceMidLoopFreeSpaceError(t *testing.T) {
	dir := t.TempDir()
	f1 := createTestFile(t, dir, "f1")
	f2 := createTestFile(t, dir, "f2")

	wantErr := errors.New("statfs failed mid-loop")
	freeSpaceCalls := 0

	got, err := ensureDiskSpaceWith(
		context.Background(),
		func(context.Context) error { return nil },
		func() (uint64, error) {
			freeSpaceCalls++
			if freeSpaceCalls == 1 {
				return 500, nil
			}
			return 0, wantErr
		},
		1000,
		func() []string {
			return []string{f1, f2}
		},
	)
	if !errors.Is(err, wantErr) {
		t.Errorf("ensureDiskSpaceWith() err = %v, want %v", err, wantErr)
	}
	if got != 0 {
		t.Errorf("ensureDiskSpaceWith() = %d, want 0", got)
	}
	if _, err := os.Stat(f1); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(%q) err = %v, want %v", f1, err, os.ErrNotExist)
	}
	if _, err := os.Stat(f2); err != nil {
		t.Errorf("os.Stat(%q) err = %v, want nil", f2, err)
	}
}

func TestEnsureDiskSpaceExhaustsPurgeable(t *testing.T) {
	dir := t.TempDir()
	f1 := createTestFile(t, dir, "f1")
	f2 := createTestFile(t, dir, "f2")

	freeSpace, freeCalls := stubFreeSpace(400, 600, 800)

	got, err := ensureDiskSpaceWith(
		context.Background(),
		func(context.Context) error { return nil },
		freeSpace,
		1000,
		func() []string {
			return []string{f1, f2}
		},
	)
	if err != nil {
		t.Fatalf("ensureDiskSpaceWith() err = %v, want nil", err)
	}
	if got != 800 {
		t.Errorf("ensureDiskSpaceWith() = %d, want 800", got)
	}
	if *freeCalls != 3 {
		t.Errorf("freeSpace calls = %d, want 3", *freeCalls)
	}
	for _, p := range []string{f1, f2} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("os.Stat(%q) err = %v, want %v", p, err, os.ErrNotExist)
		}
	}
}

func TestMissingTPMManagerClient(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if got, err := hwsecGetTPMStatus(t.Context()); !errors.Is(err, exec.ErrNotFound) || got != nil {
		t.Errorf("hwsecGetTPMStatus() = (%+v, %v), want (nil, %v)", got, err, exec.ErrNotFound)
	}
	if got, err := hwsecGetDACounter(t.Context()); !errors.Is(err, exec.ErrNotFound) || got != 0 {
		t.Errorf("hwsecGetDACounter() = (%d, %v), want (0, %v)", got, err, exec.ErrNotFound)
	}
	if err := hwsecCheckTPMState(t.Context(), nil, 0); !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("hwsecCheckTPMState() = %v, want %v", err, exec.ErrNotFound)
	}
}

func TestRunParallel(t *testing.T) {
	runParallel()

	const workers = 6
	var arrived atomic.Int32
	var ran [workers]atomic.Int32
	allStarted := make(chan struct{})

	fns := make([]func(), workers)
	for i := 0; i < workers; i++ {
		i := i
		fns[i] = func() {
			if arrived.Add(1) == workers {
				close(allStarted)
			}
			select {
			case <-allStarted:
			case <-time.After(5 * time.Second):
				t.Errorf("runParallel() concurrent goroutines = %d, want %d (timed out)", arrived.Load(), workers)
			}
			ran[i].Add(1)
		}
	}

	runParallel(fns...)
	for i := range ran {
		if got := ran[i].Load(); got != 1 {
			t.Errorf("runParallel(): worker %d ran %d times, want 1", i, got)
		}
	}
}

func TestRunParallelPanic(t *testing.T) {
	var siblingDone atomic.Bool
	var gotPanic any

	func() {
		defer func() {
			gotPanic = recover()
		}()
		runParallel(
			func() {
				panic("first boom")
			},
			func() {
				panic("second boom")
			},
			func() {
				siblingDone.Store(true)
			},
		)
	}()

	if !siblingDone.Load() {
		t.Error("runParallel() returned before sibling worker finished")
	}
	msg, ok := gotPanic.(string)
	if !ok {
		t.Fatalf("recovered panic = %#v (%T), want string", gotPanic, gotPanic)
	}
	if !strings.Contains(msg, "first boom") && !strings.Contains(msg, "second boom") {
		t.Errorf("recovered panic = %q, want worker panic message", msg)
	}
	if !strings.Contains(msg, "TestRunParallelPanic") {
		t.Errorf("recovered panic = %q, want worker stack trace containing TestRunParallelPanic", msg)
	}
}

func TestRunParallelGoexit(t *testing.T) {
	var siblingDone atomic.Bool
	runParallel(
		func() {
			runtime.Goexit()
		},
		func() {
			siblingDone.Store(true)
		},
	)
	if !siblingDone.Load() {
		t.Error("runParallel() did not wait for sibling worker when another called runtime.Goexit")
	}
}
