// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CorruptBothSignedAMDFWAB,
		Desc:         "Servo based both A and B signed AMDFW corruption test. This test corrupts both A and B SIGNED_AMDFW FMAP section, and restores it via servo",
		Contacts:     []string{"chromeos-faft@google.com", "kramasub@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level3", "firmware_ro"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnModel(
			// AMD devices before skyrim don't have the separate signed AMDFW section.
			// grunt
			"aleena", "barla", "careena", "kasumi", "kasumi360", "liara", "treeya", "treeya360",
			// guybrush
			"dewatt", "nipperkin",
			// zork
			"berknip", "dirinboz", "ezkinil", "gumboz", "jelboz360", "morphius", "vilboz", "vilboz14", "vilboz360", "woomax",
		)),
		Timeout:      50 * time.Minute,
		Vars:         []string{"firmware.skipFlashUSB"},
		SoftwareDeps: []string{"crossystem", "flashrom", "amd_cpu"},
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal_mode",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val: &corruptTestVal{
					bios.SignedAMDFWAImageSection, bios.SignedAMDFWBImageSection,
				},
			},
			{
				Name:    "dev_mode",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val: &corruptTestVal{
					bios.SignedAMDFWAImageSection, bios.SignedAMDFWBImageSection,
				},
			},
		},
	})
}

func CorruptSignedAMDFWSection(ctx context.Context, s *testing.State, h *firmware.Helper, corruptBiosRemoteImage, backupBiosRemoteImage, remoteTempDir string) error {
	s.Log("Corrupting SIGNED_AMDFW sections")

	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		s.Fatal("Failed to setup futility instance: ", err)
	}

	// - Get the body sizes
	sections, out, err := futilityInstance.DumpFmap(ctx, backupBiosRemoteImage, []string{string(bios.SignedAMDFWAImageSection), string(bios.SignedAMDFWBImageSection)})
	if err != nil {
		s.Error("Failed getting section sizes: ", err, "\nOutput:\n", string(out))
		return err
	}

	if len(sections) == 0 {
		s.Error("Output doesn't match regex: ", string(out))
		return errors.New("no sections matching SignedAMDFW")
	}

	// - Create corrupt bodies for A & B
	for _, m := range sections {
		out, err = h.DUT.Conn().CommandContext(ctx, "dd", fmt.Sprintf("of=%s/%s_corrupt.bin", remoteTempDir, m.Name), "if=/dev/random", fmt.Sprintf("bs=%d", m.Size), "count=1").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Error("Failed creating corrupt file: ", err)
			return err
		}
	}
	// - Generate a new image that contains those bodies
	out, err = futilityInstance.LoadFmap(ctx, backupBiosRemoteImage, corruptBiosRemoteImage, map[string]string{
		string(bios.SignedAMDFWAImageSection): fmt.Sprintf("%s/%s_corrupt.bin", remoteTempDir, bios.SignedAMDFWAImageSection),
		string(bios.SignedAMDFWBImageSection): fmt.Sprintf("%s/%s_corrupt.bin", remoteTempDir, bios.SignedAMDFWBImageSection),
	})
	if err != nil {
		s.Error("Failed to load flashmap sections: ", err, "\nOutput:\n", string(out))
		return err
	}
	return nil
}

func CorruptBothSignedAMDFWAB(ctx context.Context, s *testing.State) {
	corruptFWSectionTest(ctx, s, CorruptSignedAMDFWSection, "RW firmware vendor blob verification failure")
}
