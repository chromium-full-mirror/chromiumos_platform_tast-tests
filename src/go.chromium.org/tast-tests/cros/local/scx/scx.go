// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package scx provides wrappers for loading/unloading the scx schedulers
// feature for tast tests.
package scx

import (
	"context"
	"fmt"
	"os"
	"path"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast/core/testing"
)

// Type indicates the type of scx scheduler to be used.
type Type string

const (
	// TypeScxOff is the default scx scheduler as off.
	TypeScxOff Type = ""
	// TypeScxCentral is the scx_central scheduler.
	TypeScxCentral Type = "scx_central"
	// TypeScxNest is the scx_nest scheduler.
	TypeScxNest Type = "scx_nest"
	// TypeScxQmap is the scx_qmap scheduler.
	TypeScxQmap Type = "scx_qmap"
	// TypeScxSimple is the scx_simple scheduler
	TypeScxSimple Type = "scx_simple"
	// TypeScxUserland is the scx_simple scheduler
	TypeScxUserland Type = "scx_userland"
)

func scxBinPath(scheduler Type) string {
	return path.Join("/usr/bin", string(scheduler))
}

// Load loads scx scheduler to switch to eBPF scheduler.
func Load(ctx context.Context, outdir string, scheduler Type) error {
	logFile := path.Join(outdir, string(scheduler))

	schedBin := scxBinPath(scheduler)
	if _, err := os.Stat(schedBin); err != nil {
		return err
	}

	// Run scx scheduler as a background process and save log files.
	script := fmt.Sprintf("nohup %s > %s.log 2>%s.err &", schedBin, logFile, logFile)
	testing.ContextLog(ctx, "Starting scx: ", script)
	cmd := testexec.CommandContext(ctx, "/bin/sh", "-e", "-c", script)
	cmd.SysProcAttr.Setpgid = false

	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

// Unload Unloads the scx scheduler to switch to kernel scheduler.
func Unload(ctx context.Context, scheduler Type) error {
	testing.ContextLog(ctx, "Stopping scx: ", string(scheduler))
	if err := testexec.CommandContext(ctx, "pkill", string(scheduler)).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}

// IsLoaded returns true if scx scheduler is running, otherwise false
func IsLoaded(scheduler Type) bool {
	if _, err := procutil.FindUnique(procutil.ByExe(scxBinPath(scheduler))); err != nil {
		return false
	}
	return true
}
