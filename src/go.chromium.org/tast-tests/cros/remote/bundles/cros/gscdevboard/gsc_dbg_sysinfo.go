// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

const (
	// All 128 bits should be erased in the DBG rollback infomask
	dbgImageRollbackBits = 128
	// DBG images should be signed with 1 as the epoch, so tests can update
	// to them even when we increment minor versions.
	dbgImageEpoch = "1"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCDBGSysinfo,
		Desc:    "Verify sysinfo output is correct for a DBG image",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Fixture:      fixture.SystemDevboard,
	})
}

func GSCDBGSysinfo(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("(Re)starting GSC")
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Simulate the typing of "sysinfo" command on GSC console.
	sysinfo, err := i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to run sysinfo")

	// Rudimentary validation of output: find and print "DEV_ID:" line.
	s.Log("DEV_ID: ", sysinfo.Devid)

	version, err := i.VersionInfo(ctx)
	if err != nil {
		s.Fatal("Unable to get version output: ", err)
	}
	activeRW := version.ActiveRw()
	s.Log("RW_VER: ", activeRW.Version)
	epoch := strings.Split(activeRW.Version, ".")[0]
	s.Log("epoch: ", epoch)
	s.Log("Rollback info: ", sysinfo.RWRollback)

	if !activeRW.Debug {
		s.Fatal("Test is only valid on DBG images")
	}
	if sysinfo.ProdKeyladder {
		s.Errorf("Found prod Key Ladder in a DBG image: %+v", sysinfo)
	}
	if epoch != dbgImageEpoch {
		s.Errorf("Epoch is not %s in RW version %s", dbgImageEpoch, activeRW.Version)
	}

	if version.RwA.Debug && sysinfo.RWRollbackBits.SlotA.Bits != dbgImageRollbackBits {
		s.Errorf("Slot A reporting incorrect number of rollback bits: wanted %d got %d", dbgImageRollbackBits, sysinfo.RWRollbackBits.SlotA.Bits)
	}
	if version.RwB.Debug && sysinfo.RWRollbackBits.SlotB.Bits != dbgImageRollbackBits {
		s.Errorf("Slot B reporting incorrect number of rollback bits: wanted %d got %d", dbgImageRollbackBits, sysinfo.RWRollbackBits.SlotB.Bits)
	}
	startBits := sysinfo.RWRollbackBits.Flash.Bits
	if startBits == dbgImageRollbackBits {
		s.Fatal("128 bits blown in flash")
	}

	// This is dangerous. If the DBG image is incorrectly built and it
	// erases all of the rollback bits, then it'll lock out all prod
	// releases and EFI images. It might not be possible to recover this
	// device. It's still better to explicitly test it on one board while
	// qualifying DBG images instead of accidentally hitting it on all
	// faft-gsc devices.
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
	err = tpm.TpmvInvalidateInactiveRW()
	th.MustSucceed(err, "failed to send invalidate RW")

	// The DBG image should not let you blow bits
	if sysinfo.RWRollbackBits.Flash.Bits != startBits {
		s.Fatal("DBG image updated rollback bits")
	}
	if sysinfo.RWRollbackBits.Flash.Bits == dbgImageRollbackBits {
		s.Fatal("128 bits blown in flash after second update")
	}
	s.Log("Rollback info: ", sysinfo.RWRollback)
	debugImage := f.ImagePath
	if debugImage == "" {
		s.Log("No image path given. Cannot flash it twice")
		return
	}
	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	th.MustSucceed(err, "failed to get debug image version")

	// Flash the DBG image into the inactive region. Verify it doesn't
	// blow all rollback bits.
	if err = b.UpdateOnce(ctx, i, debugImage, debugVer); err != nil {
		s.Fatal("Failed to flash inactive slot: ", err)
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	// Simulate the typing of "sysinfo" command on GSC console.
	sysinfo, err = i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to run sysinfo")
	s.Log("Flashed DBG image in inactive slot")
	s.Log("Rollback info: ", sysinfo.RWRollback)

	if sysinfo.RWRollbackBits.SlotA.Bits != dbgImageRollbackBits {
		s.Errorf("Slot A reporting incorrect number of rollback bits after second update: wanted %d got %d", dbgImageRollbackBits, sysinfo.RWRollbackBits.SlotA.Bits)
	}
	if sysinfo.RWRollbackBits.SlotB.Bits != dbgImageRollbackBits {
		s.Errorf("Slot B reporting incorrect number of rollback bits after second update: wanted %d got %d", dbgImageRollbackBits, sysinfo.RWRollbackBits.SlotB.Bits)
	}
	if sysinfo.RWRollbackBits.Flash.Bits == dbgImageRollbackBits {
		s.Fatal("128 bits blown in flash after second update")
	}
}
