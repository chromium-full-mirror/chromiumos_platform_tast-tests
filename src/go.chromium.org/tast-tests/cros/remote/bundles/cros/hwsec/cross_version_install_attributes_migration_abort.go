// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/hwsec/util"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec/fixture"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrossVersionInstallAttributesMigrationAbort,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies aborting the migration of install attributes across versions would not cause data loss",
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@google.com",
		},
		// ChromeOS > Platform > System > Hardware Security > HwSec AP
		BugComponent: "b:1188704",
		Attr:         []string{"group:hw_agnostic"},
		SoftwareDeps: []string{"tpm2_simulator"},
		Timeout:      20 * time.Minute,
		VarDeps:      []string{"vm_board", "vm_image_path"},
		Params: []testing.Param{
			{
				Name:              "ti50_r118",
				Fixture:           "crossVersion.ti50_r118",
				ExtraSoftwareDeps: []string{"no_tpm_dynamic", "gsc"},
			}, {
				Name:              "tpm2_r118",
				Fixture:           "crossVersion.tpm2_r118",
				ExtraSoftwareDeps: []string{"no_tpm_dynamic", "no_gsc"},
			},
		},
	})
}

// verifyInstallAttributes verifies the contents of current install_attributes with the base version.
func verifyInstallAttributes(ctx context.Context, deviceManagementClient *hwsec.DeviceManagementClient, config *util.CrossVersionLoginConfig) error {
	for name, expectedValue := range config.InstallAttrs {
		value, err := deviceManagementClient.InstallAttributesGet(ctx, name)
		if err != nil {
			return errors.Wrapf(err, "failed to get install attributes for %s", name)
		}
		if value != expectedValue {
			return errors.Errorf("mismatched install attributes for %s. expected: %s, found: %s", name, expectedValue, value)
		}
	}
	return nil
}

func CrossVersionInstallAttributesMigrationAbort(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	cmdRunner := hwsecremote.NewCmdRunner(dut)
	helper, err := hwsecremote.NewHelper(cmdRunner, dut)
	if err != nil {
		s.Fatal("Failed to create the helper instance: ", err)
	}
	deviceManagementClient := helper.DeviceManagementClient()
	dc := helper.DaemonController()

	dataPath := s.FixtValue().(*fixture.CrossVersionLoginFixture).DataPath

	for sleepSec := 0; sleepSec < 15; sleepSec += 3 {
		// Remove the existing install attributes on device.
		if err := helper.RemoveAll(ctx, "/var/lib/device_management/"); err != nil {
			s.Fatal("Failed to remove /var/lib/device_management/: ", err)
		}
		if err := helper.RemoveAll(ctx, "/home/.shadow/install_attributes.pb"); err != nil {
			s.Fatal("Failed to remove /home/.shadow/install_attributes.pb: ", err)
		}

		if _, err := cmdRunner.Run(ctx, "sync"); err != nil {
			s.Fatal("Failed to sync the filesystem: ", err)
		}

		// Load the preserved data and sync the filesystem before shtudown the VM.
		if err := helper.LoadLoginData(ctx, dataPath, true /*includeTpm*/, true /*resumeDaemos*/); err != nil {
			s.Fatal("Failed to load login data: ", err)
		}

		if _, err := cmdRunner.Run(ctx, "sync"); err != nil {
			s.Fatal("Failed to sync the filesystem: ", err)
		}

		// Restart lockbox-cache job.
		if err := dc.Restart(ctx, hwsec.LockboxCacheJob); err != nil {
			s.Fatal("Failed to restart lockbox-cache job: ", err)
		}

		// Restart device_management daemon to restore the install_attributes state.
		if err := dc.Restart(ctx, hwsec.DeviceManagementDaemon); err != nil {
			s.Fatal("Failed to restart device_managementd: ", err)
		}

		testing.ContextLogf(ctx, "Sleeping for %v seconds", sleepSec)
		// GoBigSleepLint: We want to interrupt the boot sequency with some delay.
		if err := testing.Sleep(ctx, time.Duration(sleepSec)*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		testing.ContextLog(ctx, "Stop the VM")
		cmd := testexec.CommandContext(ctx, "cros", "vm", "--stop")
		cmd.Env = append(cmd.Env, "TMPDIR=/tmp")
		if out, err := cmd.CombinedOutput(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to stop the vm: ", err, string(out))
		}

		testing.ContextLog(ctx, "Start the VM")
		cmd = testexec.CommandContext(ctx, "cros", "vm", "--start", "--no-wait-for-boot", "--board", s.RequiredVar("vm_board"), "--image-path", s.RequiredVar("vm_image_path"))
		cmd.Env = append(cmd.Env, "TMPDIR=/tmp")
		if out, err := cmd.CombinedOutput(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to start the vm: ", err, string(out))
		}

		if !dut.Connected(ctx) {
			if err := dut.WaitConnect(ctx); err != nil {
				s.Fatal("Failed to connect to the DUT: ", err)
			}
		}

		if !dut.Connected(ctx) {
			if err := dut.WaitConnect(ctx); err != nil {
				s.Fatal("Failed to connect to the DUT: ", err)
			}
		}

		fixtureData := s.FixtValue().(*fixture.CrossVersionLoginFixture)
		for _, config := range fixtureData.ConfigList {
			if len(config.InstallAttrs) == 0 {
				continue
			}
			// Verify consistency of the install-attributes contents.
			if err := verifyInstallAttributes(ctx, deviceManagementClient, &config); err != nil {
				s.Fatal("Failed to verify the consistency of install attributes content: ", err)
			}
		}
	}
}
