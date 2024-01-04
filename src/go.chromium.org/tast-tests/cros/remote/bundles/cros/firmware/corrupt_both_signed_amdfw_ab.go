// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
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
			"aleena", "barla", "careena", "kasumi", "kasumi360", "liara", "treeya",
			// guybrush
			"dewatt", "nipperkin",
			// zork
			"berknip", "dirinboz", "ezkinil", "gumboz", "jelboz360", "morphius", "vilboz", "vilboz14", "vilboz360", "woomax",
		)),
		Timeout:      50 * time.Minute,
		Vars:         []string{"firmware.skipFlashUSB"},
		SoftwareDeps: []string{"crossystem", "flashrom", "amd_cpu"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal_mode",
				Fixture: fixture.NormalMode,
				Val: &corruptTestVal{
					bios.SignedAMDFWAImageSection, bios.SignedAMDFWBImageSection,
				},
			},
			{
				Name:    "dev_mode",
				Fixture: fixture.DevMode,
				Val: &corruptTestVal{
					bios.SignedAMDFWAImageSection, bios.SignedAMDFWBImageSection,
				},
			},
		},
	})
}

func CorruptSignedAMDFWSection(ctx context.Context, s *testing.State, h *firmware.Helper, corruptBiosRemoteImage, backupBiosRemoteImage, remoteTempDir string) error {
	s.Log("Corrupting SIGNED_AMDFW sections")
	// - Get the body sizes
	out, err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", "-p", backupBiosRemoteImage, string(bios.SignedAMDFWAImageSection), string(bios.SignedAMDFWBImageSection)).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Error("Failed getting section sizes: ", err)
		return err
	}

	fmapRe := regexp.MustCompile(`(?m)^(\S+) \d+ (\d+)`)
	matches := fmapRe.FindAllSubmatch(out, -1)
	if matches == nil {
		s.Error("Output doesn't match regex: ", string(out))
		return errors.New("No sections matching SignedAMDFW")
	}

	// - Create corrupt bodies for A & B
	for _, m := range matches {
		out, err = h.DUT.Conn().CommandContext(ctx, "dd", fmt.Sprintf("of=%s/%s_corrupt.bin", remoteTempDir, string(m[1])), "if=/dev/random", fmt.Sprintf("bs=%s", string(m[2])), "count=1").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Error("Failed creating corrupt file: ", err)
			return err
		}
	}
	// - Generate a new image that contains those bodies
	err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", corruptBiosRemoteImage, backupBiosRemoteImage,
		fmt.Sprintf("%s:%s/%s_corrupt.bin", bios.SignedAMDFWAImageSection, remoteTempDir, bios.SignedAMDFWAImageSection),
		fmt.Sprintf("%s:%s/%s_corrupt.bin", bios.SignedAMDFWBImageSection, remoteTempDir, bios.SignedAMDFWBImageSection),
	).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Error("Failed futility load_fmap: ", err)
		return err
	}
	return nil
}

func CorruptBothSignedAMDFWAB(ctx context.Context, s *testing.State) {
	corruptFWSectionTest(ctx, s, CorruptSignedAMDFWSection, "RW firmware vendor blob verification failure")
}
