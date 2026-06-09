// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hooks contains code for support adding custom hooks to root fixture.
package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/remote/kdump"
)

func init() {
	addHook(&Hook{
		Name:         "kdumpHook",
		Desc:         "Enable kdump feature to generate additional dumps for kernel crashes",
		Contacts:     []string{"chromeos-platform-stability-team@google.com", "jasongustaman@google.com"},
		BugComponent: "b:1672909",
		Impl:         &kdumpHook{},
	})
}

var kdumpEnable = testing.RegisterVarString(
	"hooks.kdump.enable",
	"",
	"A variable to decide whether kdump feature should be enabled",
)

type kdumpHook struct {
	dut     *dut.DUT
	cleanup func(ctx context.Context) error
}

// isKdumpSupported checks if kdump feature is supported by the device.
// Kdump is supported on devices with x86 architecture (b/451780016).
func isKdumpSupported(ctx context.Context, d *dut.DUT) (bool, error) {
	arch, err := d.Conn().CommandContext(ctx, "uname", "-m").Output()
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(string(arch), "x86"), nil
}

// fetchKdumpFiles copies kdump crash dumps to the fixture out directory and
// removes them from the DUT.
func fetchKdumpFiles(ctx context.Context, d *dut.DUT, outDir string) {
	files, err := kdump.ListKdumpFiles(ctx, d)
	if err != nil {
		testing.ContextLog(ctx, "Failed to fetch kdump files: ", err)
		return
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		testing.ContextLog(ctx, "Failed to create dir: ", err)
	}
	for _, file := range files {
		src := filepath.Join(kdump.KdumpDir, file)
		dst := filepath.Join(outDir, file)
		if err := linuxssh.GetFile(ctx, d.Conn(), src, dst, linuxssh.DereferenceSymlinks); err != nil {
			testing.ContextLogf(ctx, "Failed to copy %s from the DUT to the host %s: %v", src, dst, err)
		}
		if err := os.Remove(src); err != nil {
			testing.ContextLogf(ctx, "Failed to remove %s from the DUT: %v", src, err)
		}
	}
}

// SetUp enables kdump if the corresponding var is set. The setup is in a
// best-effort way so it won't return an err to avoid affecting the test
// execution.
func (h *kdumpHook) SetUp(ctx context.Context, s *HookState) error {
	if kdumpEnable.Value() != "true" {
		return nil
	}

	h.dut = s.DUT()

	// If this hook runs right after the DUT boot, it's possible that the
	// following shell commands will fail directly because the connection may
	// not be stabilized. Check and wait for a health connection at first to
	// reduce the chance of failures. See b/451777272#comment29 for details.
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := h.dut.WaitConnect(waitCtx); err != nil {
		testing.ContextLog(ctx, "Failed to wait for DUT connection ready: ", err)
		return nil
	}

	if supported, err := isKdumpSupported(ctx, h.dut); err != nil {
		testing.ContextLog(ctx, "Failed to check whether kdump is supported: ", err)
		return nil
	} else if !supported {
		return nil
	}
	cleanup, err := kdump.EnableKdump(ctx, h.dut)
	if err != nil {
		testing.ContextLog(ctx, "Failed to enable kdump: ", err)
		return nil
	}
	h.cleanup = cleanup
	return nil
}

// Reset checks the status of kdump and re-enables it if it is disabled.
func (h *kdumpHook) Reset(ctx context.Context) error {
	if h.cleanup == nil {
		return nil
	}
	// Only the first cleanup is necessary. It is expected for the cleanup
	// to revert to the state of kdump before the test runs.
	if _, err := kdump.EnableKdump(ctx, h.dut); err != nil {
		testing.ContextLog(ctx, "Failed to enable kdump: ", err)
	}
	return nil
}

// PreTest does nothing.
func (h *kdumpHook) PreTest(ctx context.Context, s *HookTestState) error {
	return nil
}

// PostTest does nothing.
func (h *kdumpHook) PostTest(ctx context.Context, s *HookTestState) error {
	return nil
}

// TearDown disables kdump if it was previously enabled by the fixture setup.
func (h *kdumpHook) TearDown(ctx context.Context, s *HookState) error {
	if h.cleanup == nil {
		return nil
	}
	// Move kdump dump data to fixture out directory so that it is uploaded
	// as test result.
	fetchKdumpFiles(ctx, h.dut, s.OutDir())
	if err := h.cleanup(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to disable kdump: ", err)
	}
	return nil
}
