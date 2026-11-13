// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundlemain

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

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
