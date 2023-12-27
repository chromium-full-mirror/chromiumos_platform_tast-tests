// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package factory

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/factory/fixture"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type extraCmdParams struct {
	extraBuildParams []string
}

// These devices will be supported by runtime probe, the progress is tracked in b/230576848.
var storageNotProbable = []string{"anahera", "bobba", "chronicler", "dewatt", "pico6"}

// The plan for these device is undecided, the progress is tracked in b/317670344.
var displayPanelNotProbable = []string{"starmie"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HWIDDatabase,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test the flow from collecting materials, generating HWID database, to verifying the database is valid",
		Contacts:     []string{"chromeos-factory-eng@google.com", "lschyi@google.com"},
		// ChromeOS > Platform > Enablement > Factory
		BugComponent: "b:167224",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      2 * time.Minute,
		Fixture:      fixture.EnsureToolkit,
		// Skip "nyan_kitty" due to slow reboot speed.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("kitty")),
		SoftwareDeps: append([]string{"factory_flow"}, fixture.EnsureToolkitSoftwareDeps...),
		Params: []testing.Param{
			testing.Param{
				Name:              "probe_by_default",
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(append(storageNotProbable, displayPanelNotProbable...)...), hwdep.ChromeEC()),
				Val: extraCmdParams{
					extraBuildParams: []string{
						"--add-firmware-components",
					},
				},
			},
			testing.Param{
				Name:              "allow_probe_no_storage",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(storageNotProbable...)),
				Val: extraCmdParams{
					extraBuildParams: []string{
						"--add-firmware-components",
						"--auto-accept-essential-prompt",
						"storage",
					},
				},
			},
			testing.Param{
				Name:              "allow_probe_no_display_panel",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(displayPanelNotProbable...)),
				Val: extraCmdParams{
					extraBuildParams: []string{
						"--add-firmware-components",
						"--auto-accept-essential-prompt",
						"display_panel",
					},
				},
			},
		},
	})
}

func HWIDDatabase(ctx context.Context, s *testing.State) {
	testExtraCmdParams := s.Param().(extraCmdParams)

	conn := s.DUT().Conn()

	buildArgs := []string{"build-database"}
	buildArgs = append(buildArgs, testExtraCmdParams.extraBuildParams...)
	buildDatabaseCmd := conn.CommandContext(ctx, "hwid", buildArgs...)
	if err := buildDatabaseCmd.Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to build the HWID database: ", err)
	}

	// Only verifies the format, probed components. As the database is built
	// locally and checksum is not the testing target, skip the check of
	// checksum here.
	verifyDatabaseCmd := conn.CommandContext(ctx, "hwid", "verify-database", "--no-verify-checksum")
	if err := verifyDatabaseCmd.Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Verify built HWID database failed: ", err)
	}
}
