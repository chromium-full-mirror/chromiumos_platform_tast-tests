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
	"go.chromium.org/tast-tests/cros/common/tbdep"
	rbios "go.chromium.org/tast-tests/cros/remote/firmware/bios"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/dut"
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
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
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
		Params: []testing.Param{
			{
				Name:      "normal_mode",
				ExtraAttr: []string{"firmware_meets_kpi"},
				Fixture:   fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val: &rbios.CorruptTestVal{
					SectionA: bios.SignedAMDFWAImageSection, SectionB: bios.SignedAMDFWBImageSection,
				},
			},
			{
				Name:    "dev_mode",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val: &rbios.CorruptTestVal{
					SectionA: bios.SignedAMDFWAImageSection, SectionB: bios.SignedAMDFWBImageSection,
				},
			},
		},
	})
}

func corruptSignedAMDFWSection(ctx context.Context, dut *dut.DUT, corruptBiosRemoteImage, backupBiosRemoteImage, remoteTempDir string) error {
	testing.ContextLog(ctx, "Corrupting SIGNED_AMDFW sections")

	futilityInstance, err := futility.NewLocalBuilder(dut).Build()
	if err != nil {
		return errors.Wrap(err, "failed to setup futility instance")
	}

	// - Get the body sizes
	sections, out, err := futilityInstance.DumpFmap(ctx, backupBiosRemoteImage, []string{string(bios.SignedAMDFWAImageSection), string(bios.SignedAMDFWBImageSection)})
	if err != nil {
		return errors.Wrapf(err, "failed getting section sizes: %s", string(out))
	}

	if len(sections) == 0 {
		testing.ContextLog(ctx, "Output doesn't match regex: ", string(out))
		return errors.New("no sections matching SignedAMDFW")
	}

	// - Create corrupt bodies for A & B
	for _, m := range sections {
		_, err = dut.Conn().CommandContext(ctx, "dd", fmt.Sprintf("of=%s/%s_corrupt.bin", remoteTempDir, m.Name), "if=/dev/random", fmt.Sprintf("bs=%d", m.Size), "count=1").Output(ssh.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to create corrupt file")
		}
	}
	// - Generate a new image that contains those bodies
	out, err = futilityInstance.LoadFmap(ctx, backupBiosRemoteImage, corruptBiosRemoteImage, map[string]string{
		string(bios.SignedAMDFWAImageSection): fmt.Sprintf("%s/%s_corrupt.bin", remoteTempDir, bios.SignedAMDFWAImageSection),
		string(bios.SignedAMDFWBImageSection): fmt.Sprintf("%s/%s_corrupt.bin", remoteTempDir, bios.SignedAMDFWBImageSection),
	})
	if err != nil {
		return errors.Wrapf(err, "failed load flashmap sections: %s", string(out))
	}
	return nil
}

func CorruptBothSignedAMDFWAB(ctx context.Context, s *testing.State) {
	backupManager := s.FixtValue().(*fixture.Value).BackupManager
	h := s.FixtValue().(*fixture.Value).Helper

	param := s.Param().(*rbios.CorruptTestVal)
	if err := rbios.CorruptFWSectionTest(ctx, backupManager, h, param, corruptSignedAMDFWSection, "RW firmware vendor blob verification failure"); err != nil {
		s.Fatal("Test failed: ", err)
	}
}
