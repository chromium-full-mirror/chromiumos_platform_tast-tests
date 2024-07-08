// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	FwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const deepSleepDelay = 20 * time.Second

func init() {
	testing.AddTest(&testing.Test{
		Func:         Cr50OpenOnBattery,
		Desc:         "Verify opening Cr50 for DUTs that have battery",
		Contacts:     []string{"chromeos-faft@google.com", "tj@semihalf.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		// TODO(b/194908238): This test never passes, when it does add firmware_unstable for stability testing.
		Attr:         []string{"group:firmware"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.GSCUART(), hwdep.Battery()),
		SoftwareDeps: []string{"gsc"},
		Fixture:      fixture.DevMode,
		Timeout:      15 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func Cr50OpenOnBattery(ctx context.Context, s *testing.State) {
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
		servo.OverrideBatt:  servo.CapAlways,   // allow to force battery presence state
		servo.OpenNoDevMode: servo.CapIfOpened, // do not allow opening CCD from normal mode
		servo.OpenNoTPMWipe: servo.CapAlways,   // do not reboot on ccd open
	}); err != nil {
		s.Fatal("Failed to set CCD capability: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Servo.RunGSCCommand(ctx, "bpforce follow_batt_pres"); err != nil {
			s.Fatal("RunGSCCommand() returned an error: ", err)
		}
	}(cleanupCtx)
	s.Log("Check open in dev mode when battery forced connected")
	if err := h.Servo.RunGSCCommand(ctx, "bpforce connect"); err != nil {
		s.Fatal("RunGSCCommand() returned an error: ", err)
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
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	s.Log("Reboot to normal mode")
	if err := ms.RebootToMode(ctx, fwCommon.BootModeNormal); err != nil {
		s.Fatal("Failed to switch to normal mode: ", err)
	}
	s.Log("Check open in normal mode when battery forced connected")
	if err := h.Servo.RunGSCCommand(ctx, "bpforce connect"); err != nil {
		s.Fatal("RunGSCCommand() returned an error: ", err)
	}
	s.Log("Lock CCD")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.LockGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, false, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Try to open CCD and expect that it remains locked")
	if err := FwUtils.VerifyGsctoolCommand(ctx, h, FwUtils.OpenGSC, FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false, true, false); err != nil {
		s.Fatal("FwUtils.VerifyGsctoolCommand() failed: ", err)
	}
	s.Log("Try to open CCD from Cr50 console and expect that it remains locked")
	if err := FwUtils.VerifyCr50Command(ctx, h, "ccd open", FwUtils.CCDLocked, FwUtils.CCDPasswordNone, false); err != nil {
		s.Fatal("FwUtils.VerifyCr50Command failed: ", err)
	}
	s.Log("Check open in normal mode when battery forced disconnected")
	if err := h.Servo.RunGSCCommand(ctx, "bpforce disconnect"); err != nil {
		s.Fatal("RunGSCCommand() returned an error: ", err)
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

	s.Log("Reboot to dev mode")
	if err := ms.RebootToMode(ctx, fwCommon.BootModeDev); err != nil {
		s.Fatal("Failed to switch to normal mode: ", err)
	}
	// Speed up booting in dev mode
	if _, err := fwCommon.ClearAndSetGBBFlags(ctx, s.DUT(), &pb.GBBFlagsState{Set: []pb.GBBFlag{pb.GBBFlag_DEV_SCREEN_SHORT_DELAY}}); err != nil {
		s.Fatal("Error setting gbb flags: ", err)
	}
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Remove ccd watchdog: ", err)
	}
	startDsCount, err := getDeepSleepCount(ctx, h)
	if err != nil {
		s.Fatal("Failed to get deep sleep count: ", err)
	}

	if _, err := h.Servo.PreferDebugHeader(ctx); err != nil {
		s.Fatal("PreferDebugHeader: ", err)
	}

	s.Log("Stopping power supply")
	if err := h.SetDUTPower(ctx, false); err != nil {
		s.Fatal("Failed to remove charger: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Connecting power supply")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to attach charger: ", err)
		}
	}(cleanupCtx)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		attached, err := h.Servo.GetChargerAttached(ctx)
		if err != nil {
			return err
		}
		if attached {
			return errors.New("charger is still attached - use Servo V4 Type-C or supply RPM vars")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 250 * time.Millisecond}); err != nil {
		s.Fatal("Check for charger failed: ", err)
	}

	s.Log("Pressing power button to make DUT into deep sleep mode")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
		s.Fatal("Failed to set a KeypressControl by servo: ", err)
	}
	s.Log("Waiting for DUT to disconnect")
	if err := h.DisconnectDUT(ctx); err != nil {
		s.Fatal("Failed to disconnect DUT: ", err)
	}
	s.Log("Waiting until power state is G3")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		state, err := h.Servo.GetECSystemPowerState(ctx)
		if err != nil {
			if strings.Contains(err.Error(), "Timed out waiting for interfaces to become available") {
				return err
			}
			return testing.PollBreak(errors.Wrap(err, "failed to get power state"))
		}
		if state != "G3" {
			return errors.Errorf("unexpected power state; got %s, want G3", state)
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 3 * time.Second}); err != nil {
		s.Fatal("Failed to wait power state to be G3: ", err)
	}

	if h.Config.Hibernate {
		s.Log("Hibernating EC")
		if err := h.Servo.ECHibernate(ctx, h.Model, servo.UseConsole); err != nil {
			s.Fatal("Failed to run EC command: ", err)
		}
		s.Logf("Sleep %s", deepSleepDelay)
		if err := testing.Sleep(ctx, deepSleepDelay); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		stopDsCount, err := getDeepSleepCount(ctx, h)
		if err != nil {
			s.Fatal("Failed to get deep sleep count: ", err)
		}
		if startDsCount == stopDsCount {
			s.Fatal("Failed to enter deep sleep")
		}

		ccdState, _, err := FwUtils.GetCCDStatePasswd(ctx, h)
		if err != nil {
			s.Fatal("FwUtils.GetCCDStatePasswd() failed: ", err)
		}
		if ccdState != FwUtils.CCDOpened {
			s.Fatal("CCD unexpectedly not opened after deep sleep")
		}

		s.Log("Reboot GSC to get out from EC hibernation")
		if err := h.Servo.RunGSCCommand(ctx, "reboot"); err != nil {
			s.Fatal("RunGSCCommand() returned an error: ", err)
		}
		s.Log("Connecting power supply")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to attach charger: ", err)
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 3*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.FromHibernation); err != nil {
			s.Fatal("Failed to reconnect: ", err)
		}
	} else {
		s.Log("Skipping hibernate, because the DUT doesn't support it")
		s.Log("Setting DUT's power on")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			s.Fatal("Failed to set DUT's power on: ", err)
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 3*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to connect to DUT: ", err)
		}
	}

	s.Log("Verify that CCD does not remain opened after deep sleep")
	if err := FwUtils.CheckExpectedCCDState(ctx, h, FwUtils.CCDLocked, FwUtils.CCDPasswordNone); err != nil {
		s.Fatal("FwUtils.CheckExpectedCCDState() failed: ", err)
	}
}

// getDeepSleepCount gets the deep sleep count from idle command in Cr50 console.
func getDeepSleepCount(ctx context.Context, h *firmware.Helper) (string, error) {
	out, err := h.Servo.RunGSCCommandGetOutput(ctx, "idle", []string{`deep\s*sleep\s*count:\s*(\d+)`})
	if err != nil {
		return "", errors.Wrap(err, "RunGSCCommandGetOutput() returned an error")
	}
	return out[0][1], nil
}
