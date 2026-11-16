// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	gotesting "testing"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func TestIsARCSignedInPrefPersistedMissingFile(t *gotesting.T) {
	path := filepath.Join(t.TempDir(), "Preferences")
	got, err := isARCSignedInPrefPersisted(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("isARCSignedInPrefPersisted(%q) returned err = %v, want os.ErrNotExist", path, err)
	}
	if got {
		t.Errorf("isARCSignedInPrefPersisted(%q) = %v, want false", path, got)
	}
}

func TestIsARCSignedInPrefPersisted(t *gotesting.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    bool
		wantErr bool
	}{
		{
			name:    "empty_file",
			content: "",
			wantErr: true,
		},
		{
			name:    "empty_json",
			content: `{}`,
			want:    false,
		},
		{
			name:    "arc_without_signed_in",
			content: `{"arc":{"packages":{}}}`,
			want:    false,
		},
		{
			name:    "signed_in_false",
			content: `{"arc":{"signedin":false}}`,
			want:    false,
		},
		{
			name:    "signed_in_true",
			content: `{"arc":{"signedin":true,"packages":{}}}`,
			want:    true,
		},
		{
			name:    "non_boolean_signed_in",
			content: `{"arc":{"signedin":"true"}}`,
			wantErr: true,
		},
		{
			name:    "invalid_json",
			content: `{"arc":`,
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			path := filepath.Join(t.TempDir(), "Preferences")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatalf("os.WriteFile(%q) failed: %v", path, err)
			}
			got, err := isARCSignedInPrefPersisted(path)
			if (err != nil) != tc.wantErr {
				t.Errorf("isARCSignedInPrefPersisted(%q) returned err = %v, wantErr = %v", tc.content, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("isARCSignedInPrefPersisted(%q) = %v, want %v", tc.content, got, tc.want)
			}
		})
	}
}

func TestWaitForARCSignedInPrefPersistedAtPathAlreadyPersisted(t *gotesting.T) {
	path := filepath.Join(t.TempDir(), "Preferences")
	if err := os.WriteFile(path, []byte(`{"arc":{"signedin":true}}`), 0600); err != nil {
		t.Fatalf("os.WriteFile(%q) failed: %v", path, err)
	}
	err := waitForARCSignedInPrefPersistedAtPath(context.Background(), path, &testing.PollOptions{
		Interval: time.Millisecond,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Errorf("waitForARCSignedInPrefPersistedAtPath(%q) returned err = %v, want nil", path, err)
	}
}

func TestWaitForARCSignedInPrefPersistedAtPathRetriesTransientStates(t *gotesting.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Preferences")
	tmpPath := filepath.Join(dir, "Preferences.tmp")

	done := make(chan struct{})
	writeErrCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	// Ensure the writer goroutine exits before t.TempDir() cleanup runs.
	defer wg.Wait()
	defer close(done)

	go func() {
		defer wg.Done()
		stages := []string{
			`{"arc":`,
			`{"arc":{"signedin":false}}`,
			`{"arc":{"signedin":true}}`,
		}
		for _, content := range stages {
			select {
			case <-done:
				return
			case <-time.After(15 * time.Millisecond):
			}
			if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil {
				writeErrCh <- err
				return
			}
			if err := os.Rename(tmpPath, path); err != nil {
				writeErrCh <- err
				return
			}
		}
	}()

	err := waitForARCSignedInPrefPersistedAtPath(context.Background(), path, &testing.PollOptions{
		Interval: 5 * time.Millisecond,
		Timeout:  5 * time.Second,
	})
	select {
	case writeErr := <-writeErrCh:
		t.Fatalf("async Preferences update failed: %v", writeErr)
	default:
	}
	if err != nil {
		t.Errorf("waitForARCSignedInPrefPersistedAtPath(%q) returned err = %v, want nil", path, err)
	}
}

func TestWaitForARCSignedInPrefPersistedAtPathTimeout(t *gotesting.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{
			name:    "signed_in_false",
			content: `{"arc":{"signedin":false}}`,
		},
		{
			name:    "invalid_json",
			content: `{"arc":`,
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			path := filepath.Join(t.TempDir(), "Preferences")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatalf("os.WriteFile(%q) failed: %v", path, err)
			}
			err := waitForARCSignedInPrefPersistedAtPath(context.Background(), path, &testing.PollOptions{
				Interval: time.Millisecond,
				Timeout:  20 * time.Millisecond,
			})
			if err == nil {
				t.Errorf("waitForARCSignedInPrefPersistedAtPath(%q) with %q returned nil, want timeout error", path, tc.content)
			}
		})
	}
}

func TestWaitForARCSignedInPrefPersistedAtPathCanceled(t *gotesting.T) {
	path := filepath.Join(t.TempDir(), "Preferences")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitForARCSignedInPrefPersistedAtPath(ctx, path, &testing.PollOptions{
		Interval: time.Millisecond,
		Timeout:  time.Second,
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("waitForARCSignedInPrefPersistedAtPath(%q) with canceled ctx returned err = %v, want context.Canceled", path, err)
	}
}
