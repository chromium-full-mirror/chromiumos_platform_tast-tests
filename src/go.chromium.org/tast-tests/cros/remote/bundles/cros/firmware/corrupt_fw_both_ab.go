// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
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
		Func: CorruptFWBothAB,
		Desc: "Corrupt both copies of AP firmware, verify broken screen, restore backup via servo",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      25 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "body_normal",
				ExtraAttr: []string{"firmware_meets_kpi"},
				Fixture:   fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val: &rbios.CorruptTestVal{
					SectionA: bios.FWBodyAImageSection, SectionB: bios.FWBodyBImageSection,
				},
			},
			{
				Name:              "body_dev",
				Fixture:           fixture.BootModeFixtureWithAPBackup(fixture.DevModeGBB),
				ExtraHardwareDeps: hwdep.D(hwdep.DevRecEventlog()),
				Val: &rbios.CorruptTestVal{
					SectionA: bios.FWBodyAImageSection, SectionB: bios.FWBodyBImageSection,
				},
			},
			{
				Name:      "sig_normal",
				ExtraAttr: []string{"firmware_meets_kpi"},
				Fixture:   fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val: &rbios.CorruptTestVal{
					SectionA: bios.FWSignAImageSection, SectionB: bios.FWSignBImageSection,
				},
			},
			{
				Name:              "sig_dev",
				ExtraHardwareDeps: hwdep.D(hwdep.DevRecEventlog()),
				Fixture:           fixture.BootModeFixtureWithAPBackup(fixture.DevModeGBB),
				Val: &rbios.CorruptTestVal{
					SectionA: bios.FWSignAImageSection, SectionB: bios.FWSignBImageSection,
				},
			},
		},
	})
}

func corruptFWBodySection(ctx context.Context, dut *dut.DUT, corruptBiosRemoteImage, backupBiosRemoteImage, remoteTempDir string) error {
	testing.ContextLog(ctx, "Corrupting FW bodies")
	// Copy BIOS image to corrupt it
	out, err := dut.Conn().CommandContext(ctx, "cp", backupBiosRemoteImage, corruptBiosRemoteImage).Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "failed to copy image for corruption: %s", out)
	}

	for _, section := range []string{string(bios.FWBodyAImageSection), string(bios.FWBodyBImageSection)} {
		if _, err := dut.Conn().CommandContext(ctx, "cbfstool", corruptBiosRemoteImage, "remove", "-r", section, "-n", "fallback/payload").Output(ssh.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "failed to remove %s section", section)
		}
		// Add new file to make sure checksum will differ
		if _, err := dut.Conn().CommandContext(ctx, "cbfstool", corruptBiosRemoteImage, "add-int", "-r", section, "-n", "fallback/new_int", "-i", "6677").Output(ssh.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "failed to add-int %s section", section)
		}
	}
	return nil
}

func CorruptFWBothAB(ctx context.Context, s *testing.State) {
	backupManager := s.FixtValue().(*fixture.Value).BackupManager
	h := s.FixtValue().(*fixture.Value).Helper

	param := s.Param().(*rbios.CorruptTestVal)
	if err := rbios.CorruptFWSectionTest(ctx, backupManager, h, param, corruptFWBodySection, "RW firmware unable to verify firmware body"); err != nil {
		s.Fatal("Test failed: ", err)
	}
}
