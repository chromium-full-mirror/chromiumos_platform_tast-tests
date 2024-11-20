// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	FwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Cr50Password,
		Desc: "Verify that Cr50 password can be set and cleared",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tj@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.DevMode,
		Timeout:      10 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.GSCUART()),
		SoftwareDeps: []string{"gsc"},
	})
}

func Cr50Password(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}

	err := h.OpenCCD(ctx, true, true)
	if err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}

	defer func() {
		if err := FwUtils.Cr50Cleanup(ctx, h); err != nil {
			s.Fatal("Cleanup failed: ", err)
		}
	}()

	s.Log("Setting password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.SetGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Reset CCD from Cr50 console and expect that password is cleared")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd reset", FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}

	ccdSettings := map[servo.CCDCap]servo.CCDCapState{
		servo.OpenNoLongPP:  servo.CapAlways, // avoid clicking power button to open CCD
		servo.OpenNoTPMWipe: servo.CapAlways, // do not reboot on ccd open
		servo.OpenFromUSB:   servo.CapAlways, // allow opening CCD from Cr50 console
	}
	if err := h.Servo.SetCCDCapability(ctx, ccdSettings); err != nil {
		s.Fatal("Failed to set CCD capability: ", err)
	}

	s.Log("Setting password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.SetGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Setting password while password is set")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.SetGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false, true, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Reboot GSC and expect that password is still set afterwards")
	if err := FwUtils.VerifyCr50Command(ctx, h, "reboot", FwUtils.CCDLocked, FwUtils.CCDPasswordSet, true); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("CCD is locked after reboot, try to open with no password and expect that it remains locked")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd open", FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Run ccd open with password from Cr50 console and expect that ccd opens")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd open "+FwUtils.CCDPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, "lock", FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Try to clear password while CCD is locked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, true, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Run ccd unlock with password from Cr50 console and expect that CCD unlocks")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd unlock "+FwUtils.CCDPassword, FwUtils.CCDUnlocked, FwUtils.CCDPasswordSet, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Clear password while CCD is unlocked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDUnlocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Run ccd testlab open from Cr50 console and expect that CCD opens")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd testlab open", FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Setting password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.SetGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Try to clear password using wrong password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false, true, true); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Clear password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Unlock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, "unlock", FwUtils.CCDUnlocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Set the password while CCD is unlocked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.SetGSCPassword, FwUtils.CCDUnlocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, "lock", FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Try to clear password while CCD is locked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, true, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Unlock CCD while the password is set")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, "unlock", FwUtils.CCDUnlocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Clear password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDUnlocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Set the password while CCD is unlocked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.SetGSCPassword, FwUtils.CCDUnlocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Run ccd testlab open from Cr50 console and expect that CCD opens")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd testlab open", FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Clear password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}

	s.Log("Set OpenFromUSB=IfOpened")
	ccdSettings = map[servo.CCDCap]servo.CCDCapState{
		servo.OpenFromUSB: servo.CapIfOpened, // when password is not set, opening CCD should be possible only with gsctool
	}
	if err := h.Servo.SetCCDCapability(ctx, ccdSettings); err != nil {
		s.Fatal("Failed to set CCD capability: ", err)
	}
	s.Log("Setting password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.SetGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, "lock", FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Run ccd open with password from Cr50 console and expect that CCD opens")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd open "+FwUtils.CCDPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordSet, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Clear password")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, "lock", FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
	s.Log("Try to open CCD from Cr50 console and expect that it remains locked")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd open", FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Open CCD from developer console")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, "open", FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
}
