// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ehide

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"go.chromium.org/tast-tests/cros/common/network/ehideconst"
	"go.chromium.org/tast/core/errors"
)

const stubEhideEnv = "TEST_EHIDE_STUB_MODE"

// TestMain doubles as a stub ehide binary when stubEhideEnv is set (used by
// setupStubEhide via a symlink to os.Executable). Setting the env var to "fail"
// exits with status 1; any other non-empty value is printed to stdout.
func TestMain(m *testing.M) {
	if mode := os.Getenv(stubEhideEnv); mode != "" {
		if len(os.Args) != 2 || os.Args[1] != "state" {
			fmt.Fprintf(os.Stderr, "unexpected args: %q\n", os.Args)
			os.Exit(2)
		}
		if mode == "fail" {
			os.Exit(1)
		}
		fmt.Println(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestGetState(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	for _, tc := range []struct {
		content string
		want    string
	}{
		{content: "on\n", want: ehideconst.EhideStateOn},
		{content: "off\n", want: ehideconst.EhideStateOff},
		{content: "set_up\n", want: "set_up"},
	} {
		path := filepath.Join(t.TempDir(), "ehide_state")
		if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
			t.Errorf("os.WriteFile(%q) with %q failed: %v", path, tc.content, err)
			continue
		}
		if got, err := getState(context.Background(), path); err != nil {
			t.Errorf("getState with %q failed: %v", tc.content, err)
		} else if got != tc.want {
			t.Errorf("getState with %q = %q; want %q", tc.content, got, tc.want)
		}
	}
}

func TestGetStateNoStateFile(t *testing.T) {
	// ehide should not be run, so an empty executable is enough.
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, ehideconst.EhidePath), nil, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	path := filepath.Join(t.TempDir(), "ehide_state")
	if got, err := getState(context.Background(), path); err != nil {
		t.Error("getState failed: ", err)
	} else if got != ehideconst.EhideStateOff {
		t.Errorf("getState = %q; want %q", got, ehideconst.EhideStateOff)
	}
}

func TestGetStateNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	path := filepath.Join(t.TempDir(), "ehide_state")
	if got, err := getState(context.Background(), path); !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("getState = (%q, %v); want %v", got, err, exec.ErrNotFound)
	}
}

func setupStubEhide(t *testing.T, mode string) {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal("Failed to find test executable: ", err)
	}
	binDir := t.TempDir()
	// Symlink the test binary as ehide so TestMain acts as a stub executable.
	if err := os.Symlink(exe, filepath.Join(binDir, ehideconst.EhidePath)); err != nil {
		t.Fatal("Failed to symlink stub ehide: ", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv(stubEhideEnv, mode)
}

func TestGetStateFallback(t *testing.T) {
	setupStubEhide(t, " on ")

	// Passing a directory causes os.ReadFile to fail with EISDIR (non-NotExist)
	// even when running as root, exercising the fallback to "ehide state".
	if got, err := getState(context.Background(), t.TempDir()); err != nil {
		t.Error("getState failed: ", err)
	} else if got != ehideconst.EhideStateOn {
		t.Errorf("getState = %q; want %q", got, ehideconst.EhideStateOn)
	}
}

func TestGetStateFallbackError(t *testing.T) {
	setupStubEhide(t, "fail")

	// A directory path triggers EISDIR, exercising the fallback to "ehide state".
	var exitErr *exec.ExitError
	if got, err := getState(context.Background(), t.TempDir()); !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Errorf("getState = (%q, %v); want *exec.ExitError with exit code 1", got, err)
	}
}
