// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	FwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Cr50Unlock,
		Desc: "Verify unlocking Cr50",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tj@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.DevMode,
		Timeout:      8 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.GSCUART()),
		SoftwareDeps: []string{"gsc"},
	})
}

func Cr50Unlock(ctx context.Context, s *testing.State) {
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
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Reset CCD from Cr50 console")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd reset", FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command() failed: ", err)
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
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Try to unlock CCD using wrong password and intentionally do not wait after executing the command")
	_, err = h.Servo.RunGSCCommandGetOutput(ctx, "ccd unlock "+FwUtils.CCDWrongPassword, []string{`Access\s+Denied`})
	if err != nil {
		s.Fatal("ccd unlock did not deny to unlock CCD using wrong password: ", err)
	}
	s.Log("Try to unlock CCD using the right password and verify that the rate limit prevents unlock")
	_, err = h.Servo.RunGSCCommandGetOutput(ctx, "ccd unlock "+FwUtils.CCDPassword, []string{`Busy`})
	if err != nil {
		s.Fatal("ccd unlock did not deny to unlock CCD using wrong password: ", err)
	}
	testing.Sleep(ctx, FwUtils.WaitAfterCCDSettingChange)
	if err := FwUtils.CheckExpectedCCDState(ctx, h, FwUtils.CCDLocked, FwUtils.CCDPasswordSet); err != nil {
		s.Fatal("FwUtils.CheckExpectedCCDState() failed: ", err)
	}
	s.Log("Run ccd unlock with password from Cr50 console and expect that CCD unlocks")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd unlock "+FwUtils.CCDPassword, FwUtils.CCDUnlocked, FwUtils.CCDPasswordSet, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command() failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Try to unlock CCD using wrong password and expect that it remains locked")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd unlock "+FwUtils.CCDWrongPassword, FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command() failed: ", err)
	}
	s.Log("Unlock CCD from AP while the password is set")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.UnlockGSC, FwUtils.CCDUnlocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Try to unlock CCD from AP using wrong password and expect that it remains locked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.UnlockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordSet, false, true, true); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Unlock CCD from AP while the password is set")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.UnlockGSC, FwUtils.CCDUnlocked, FwUtils.CCDPasswordSet, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Clear password while CCD is unlocked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.ClearGSCPassword, FwUtils.CCDUnlocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
}
