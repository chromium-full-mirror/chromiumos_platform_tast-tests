// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package factory

import (
	"context"
	"encoding/json"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/factory/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	testCrosRegionsPath  = "/usr/share/misc/cros-regions.json"
	localCrosRegionsPath = "/usr/local/factory/py_pkg/cros/factory/test/l10n/cros-regions.json"
)

type extraCmdParams struct {
	extraBuildParams []string
}

// These devices will be supported by runtime probe, the progress is tracked in b/230576848.
var storageNotProbable = []string{"anahera", "bobba", "chronicler", "dewatt", "pico6"}

// The plan for these device is undecided, the progress is tracked in b/317670344.
var displayPanelNotProbable = []string{"starmie"}

// Wifi is an essential component and should be probed in this test. The following platforms requires custom probe statement. The progress is tracked in b/261355069.
var customProbePlatforms = []string{"trogdor", "strongbad"}

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
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("kitty"), hwdep.SkipOnPlatform(customProbePlatforms...)),
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

// setUpEcManifest copies testing EC component manifest to stateful partition. If the manifest doesn't exist, creates an empty one.
func setUpEcManifest(ctx context.Context, s *testing.State, conn *ssh.Conn) error {
	ecVersion, err := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain).Version(ctx)
	if err != nil {
		s.Fatal("Failed to get EC version: ", err)
	}

	imageName, err := conn.CommandContext(ctx, "cros_config", "/firmware", "image-name").Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to get image name")
	}

	fakeManifest, err := json.Marshal(map[string]string{
		"ec_version": ecVersion,
	})
	if err != nil {
		return errors.Wrap(err, "failed to construct manifest")
	}

	manifestDir := "/usr/local/factory/cme/" + string(imageName)
	if err := conn.CommandContext(ctx, "mkdir", "-p", manifestDir).Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to create manifest dir: %s", manifestDir)
	}

	testManifestPath := "/usr/share/cme/" + string(imageName) + "/component_manifest.json"
	localManifestPath := manifestDir + "/component_manifest.json"

	setUpCrosRegionsCmd := conn.CommandContext(ctx, "cp", testManifestPath, localManifestPath)
	if err := setUpCrosRegionsCmd.Run(ssh.DumpLogOnError); err != nil {
		s.Log("No component manifest found in test image. Use fake manifest")
	} else {
		return nil
	}

	if err := linuxssh.WriteFile(ctx, conn, localManifestPath, fakeManifest, 0644); err != nil {
		return errors.Wrap(err, "failed to write manifest")
	}
	return nil
}

func HWIDDatabase(ctx context.Context, s *testing.State) {
	testExtraCmdParams := s.Param().(extraCmdParams)

	conn := s.DUT().Conn()

	// Create a empty EC component manifest with matched EC version in stateful partition.
	if err := setUpEcManifest(ctx, s, conn); err != nil {
		s.Fatal("Failed to set up EC manifest: ", err)
	}

	// Use cros-regions.json in test image instead of release partition.
	setUpCrosRegionsCmd := conn.CommandContext(ctx, "cp", testCrosRegionsPath, localCrosRegionsPath)
	if err := setUpCrosRegionsCmd.Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to set up cros-regions.json: ", err)
	}

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
