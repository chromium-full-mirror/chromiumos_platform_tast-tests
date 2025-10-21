// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtimeprobe

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/runtimeprobe/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RuntimeHWIDDefault,
		Desc: "Runtime HWID file does not exist after rebooting DUT by default",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"allenshihmc@google.com",
		},
		BugComponent: "b:606088",
		SoftwareDeps: []string{"reboot", "racc"},
		HardwareDeps: utils.RuntimeHWIDRefreshDeps(),
		Fixture:      fixture.CleanupRuntimeHWID,
		Attr:         []string{"group:racc", "racc_config_installed"},
	})
}

func RuntimeHWIDDefault(ctx context.Context, s *testing.State) {
	const (
		runtimeHWIDFilePath = "/var/cache/hardware_verifier/runtime_hwid"
	)

	d := s.DUT()
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	if err := utils.WaitServiceState(ctx, d, "system-services", "start/running"); err != nil {
		s.Fatal("Service system-services timed out: ", err)
	}

	if err := utils.WaitServiceState(ctx, d, "hardware_verifier", "stop/waiting"); err != nil {
		s.Fatal("Service hardware_verifier timed out: ", err)
	}

	// Checks that the Runtime HWID file doesn't exist on DUT.
	if err := d.Conn().CommandContext(ctx, "test", "-f", runtimeHWIDFilePath).Run(); err == nil {
		s.Fatalf("File %q exists after reboot", runtimeHWIDFilePath)
	}

	cmd := []string{
		"runtime_hwid_tool", "get", "--verbosity=1",
	}
	out, err := d.Conn().CommandContext(ctx, cmd[0], cmd[1:]...).Output()
	if err != nil {
		s.Fatal("Failed to invoke runtime_hwid_tool: ", err)
	}
	runtimeHwidToolOut := strings.TrimSpace(string(out))

	out, err = d.Conn().CommandContext(ctx, "crossystem", "hwid").Output()
	if err != nil {
		s.Fatal("Failed to run \"crossystem hwid\": ", err)
	}
	factoryHwid := strings.TrimSpace(string(out))

	if runtimeHwidToolOut != factoryHwid {
		s.Fatalf("runtime_hwid_tool output mismatch: got %q, want %q", runtimeHwidToolOut, factoryHwid)
	}
}
