// Copyright 2023 The ChromiumOS Authors
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

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Cr50Open,
		Desc:         "Verify opening Cr50",
		Contacts:     []string{"chromeos-faft@google.com", "tj@semihalf.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.GSCUART()),
		SoftwareDeps: []string{"gsc"},
		Fixture:      fixture.DevMode,
		Timeout:      15 * time.Minute,
	})
}

func Cr50Open(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	// Reserve ten seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}
	defer func(ctx context.Context) {
		if err := FwUtils.Cr50Cleanup(ctx, h); err != nil {
			s.Error("Cleanup failed: ", err)
		}
	}(cleanupCtx)
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	s.Log("ccd testlab open")
	if err := h.OpenCCD(ctx, true, true); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}
	s.Log("Reset CCD from Cr50 console")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd reset", FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command() failed: ", err)
	}
	if err := h.Servo.SetCCDCapability(ctx, map[servo.CCDCap]servo.CCDCapState{
		servo.OpenNoLongPP:  servo.CapAlways,   // avoid clicking power button to open CCD
		servo.OpenFromUSB:   servo.CapAlways,   // allow opening CCD from Cr50 console
		servo.OpenNoDevMode: servo.CapIfOpened, // do not allow opening CCD from normal mode
		servo.OpenNoTPMWipe: servo.CapIfOpened, // reboot on ccd open
	}); err != nil {
		s.Fatal("Failed to set CCD capability: ", err)
	}
	// Check open only works when the device is in dev mode.
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	// TPM will be wiped while opening CCD, so AP will reboot to normal mode
	s.Log("Open CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.OpenGSC, FwUtils.CCDOpened, FwUtils.CCDPasswordNone, true, true, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Try to open CCD from AP and expect that it remains locked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.OpenGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, true, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Try to open CCD from Cr50 console and expect that it remains locked")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd open", FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Run ccd testlab open from Cr50 console and expect that CCD opens")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd testlab open", FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	if err := h.Servo.SetCCDCapability(ctx, map[servo.CCDCap]servo.CCDCapState{
		servo.OpenNoDevMode: servo.CapAlways, // allow opening CCD from normal mode
	}); err != nil {
		s.Fatal("Failed to set CCD capability: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Open CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.OpenGSC, FwUtils.CCDOpened, FwUtils.CCDPasswordNone, true, true, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Open CCD from Cr50 console")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd open", FwUtils.CCDOpened, FwUtils.CCDPasswordNone, true); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	if err := h.Servo.SetCCDCapability(ctx, map[servo.CCDCap]servo.CCDCapState{
		servo.OpenNoTPMWipe: servo.CapAlways, // do not reboot on ccd open
	}); err != nil {
		s.Fatal("Failed to set CCD capability: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Open CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.OpenGSC, FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Open CCD from Cr50 console")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd open", FwUtils.CCDOpened, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	// Rebooting GSC will cause EC to restart, potentially breaking the servo watchdog.
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Remove ccd watchdog: ", err)
	}
	s.Log("Reboot GSC and expect that it is locked afterwards")
	if err := FwUtils.VerifyCr50Command(ctx, h, "reboot", FwUtils.CCDLocked, FwUtils.CCDPasswordNone, true); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand failed: ", err)
	}
}
