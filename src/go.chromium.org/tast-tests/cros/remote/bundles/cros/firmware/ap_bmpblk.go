// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         APBmpblk,
		Desc:         "Inspects coreboot contents for configuration indicators for the bitmaps used in firmware UI",
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Contacts: []string{
			"chromeos-faft@google.com",
			"jwerner@chromium.org", // Test author
		},
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level4"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		Fixture:      fixture.NormalMode,
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// APBmpblk inspects coreboot contents for indicators of bitmap block configuration.  See https://chromium.googlesource.com/chromiumos/platform/bmpblk for more context.
func APBmpblk(ctx context.Context, s *testing.State) {
	const validityIndicator string = "romstage"
	const applicabilityIndicator string = "vbgfx.bin"
	const misconfigurationIndicator string = "vbgfx_not_scaled"

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}
	bs := h.BiosServiceClient

	coreboot, err := bs.BackupImageSection(ctx, &pb.FWSectionInfo{
		Programmer: pb.Programmer_BIOSProgrammer,
		Section:    pb.ImageSection_EmptyImageSection,
	})
	if err != nil {
		s.Fatal("Failed to backup the firmware image: ", err)
	}
	s.Log("A portion of the firmware ROM containing Coreboot is stored at: ", coreboot.Path)

	layout, err := h.DUT.Conn().CommandContext(ctx, "cbfstool", coreboot.Path, "layout").Output()
	if err != nil {
		s.Log(layout)
		s.Fatal("Failed to execute cbfstool: ", err)
	}
	layouts := string(layout)

	region := bios.COREBOOTImageSection
	// Older devices may store coreboot in a region named BOOT_STUB, instead of COREBOOT.
	if strings.Contains(layouts, "BOOT_STUB") {
		region = bios.BOOTSTUBImageSection
	}

	out, err := h.DUT.Conn().CommandContext(ctx, "cbfstool", coreboot.Path, "print", "-r", string(region)).Output()
	if err != nil {
		s.Log(out)
		s.Fatal("Failed to execute cbfstool: ", err)
	}
	outs := string(out)
	path := filepath.Join(s.OutDir(), "cbfs.txt")
	if err := os.WriteFile(path, out, 0644); err != nil {
		s.Error("Failed to save cbfstool output: ", err)
	}

	if !strings.Contains(outs, validityIndicator) {
		s.Fatalf("Failed validity check.  Output of cbfstool did not contain %q (saved output to %s)",
			validityIndicator, filepath.Base(path))
	}

	if !strings.Contains(outs, applicabilityIndicator) {
		s.Log("This board appears to have no firmware screens")
		return
	}

	if strings.Contains(outs, misconfigurationIndicator) {
		s.Fatalf("Failed inspection for generic configuration.  This build was configured for a generic "+
			"1366x768 display resolution.  Images will get scaled up at runtime and look blurry.  You need to "+
			"explicitly set the panel resolution for this board in bmpblk/images/boards.yaml and add it to "+
			"CROS_BOARDS in the sys-boot/chromeos-bmpblk .ebuild.  Do *not* do this until you are certain of "+
			"the panel resolution that the final product will ship with!  Output of cbfstool did not contain %q "+
			"(saved output to %s).", misconfigurationIndicator, filepath.Base(path))
	}
}
