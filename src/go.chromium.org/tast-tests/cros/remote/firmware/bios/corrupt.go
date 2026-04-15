// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bios

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// CorruptTestVal defines the sections that should be corrupted by
type CorruptTestVal struct {
	SectionA bios.ImageSection
	SectionB bios.ImageSection
}

// CorruptFWSectionTest performs common test logic for all tests that corrupt the BIOS sections.
func CorruptFWSectionTest(ctx context.Context, backupManager *fixture.FirmwareBackupManager, h *firmware.Helper, param *CorruptTestVal, corruptFMAPSection func(context.Context, *dut.DUT, string, string, string) error, failureReason string) (retErr error) {
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "failed to init servo")
	}

	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		return errors.Wrap(err, "failed to setup futility instance")
	}

	sectionA := string(param.SectionA)
	sectionB := string(param.SectionB)
	shouldRestoreFirmware := false
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()

	out, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed creating remote temp dir")
	}
	remoteTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		err := h.DUT.Conn().CommandContext(cleanupContext, "rm", "-rf", remoteTempDir).Run(ssh.DumpLogOnError)
		if err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "failed deleting remote temp dir"))
		}
	}()

	backupBiosRemoteImage := fmt.Sprintf("%s/bios_backup.bin", remoteTempDir)
	if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, backupBiosRemoteImage); err != nil {
		return errors.Wrap(err, "failed to copy firmware backup image to DUT")
	}

	out, err = h.ServoProxy.OutputCommand(ctx, false, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwservoXXXXXX")
	if err != nil {
		return errors.Wrap(err, "failed to create servo temp dir")
	}
	servoTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		if err := h.ServoProxy.RunCommand(cleanupContext, false, "rm", "-rf", servoTempDir); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "failed deleting servo temp dir"))
		}
	}()

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		return errors.Wrap(err, "failed to clear event log")
	}

	restoreFirmware := func(ctx context.Context) error {
		testing.ContextLog(ctx, "Restoring AP firmware via servo")

		backupOnServoProxy := fmt.Sprintf("%s/bios_backup.bin", servoTempDir)
		if err := backupManager.CopyBackupToServoProxy(ctx, h.ServoProxy, fixture.FirmwareAP, backupOnServoProxy); err != nil {
			return errors.Wrap(err, "failed to copy backup to ServoProxy")
		}

		// futility doesn't know that you can't flash AP over C2D2, so switch the active controller to CCD.
		if hasC2D2, err := h.Servo.HasC2D2(ctx); err != nil {
			return errors.Wrap(err, "failed check c2d2")
		} else if hasC2D2 {
			if err := h.Servo.RequireCCD(ctx); err != nil {
				return errors.Wrap(err, "failed enable CCD")
			}
		}
		if out, err := h.ServoProxy.OutputCommand(ctx, true, "futility", "update", "--servo", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()),
			"--mode=recovery", "--wp=1", "--host_only", "-i", backupOnServoProxy); err != nil {
			return errors.Wrapf(err, "failed restoring firmware via servo: %s", string(out))
		}
		// In b/314059450 it was discovered that some devices don't come back on after futility update.
		if hasEC, err := h.Servo.HasControl(ctx, string(servo.ECSystemPowerState)); err != nil {
			testing.ContextLog(ctx, "Error checking for chrome ec: ", err)
		} else if hasEC {
			testing.ContextLog(ctx, "Waiting for DUT to power on")
			if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout,
				"G3", "S0"); err != nil {
				return errors.Wrap(err, "failed to get power state after restoring firmware")
			}
			powerState, err := h.Servo.GetECSystemPowerState(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get power state")
			}
			if powerState == "G3" {
				testing.ContextLog(ctx, "DUT is in G3, pressing power key to wake")
				err = h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn))
				if err != nil {
					return errors.Wrap(err, "failed to press power")
				}
				if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout,
					"S0"); err != nil {
					return errors.Wrap(err, "failed to wait for S0 after power button")
				}
			}
		}

		if err := h.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed to WaitConnect after reset")
		}
		shouldRestoreFirmware = false
		return nil
	}
	defer func() {
		if shouldRestoreFirmware {
			retErr = errors.Join(retErr, restoreFirmware(cleanupContext))
		}
	}()

	// futility won't flash an image that has an invalid signature blocks, so to corrupt the data we need to:
	// - Create copy of image and modify body by removing (or modifying) CBFS file "fallback/payload" to make firmware fail verification.
	// - Sign resulting image
	// - Extract the sections we want to test
	// - Create yet another image that contains those sections
	// - Flash it.

	corruptBiosRemoteImage := fmt.Sprintf("%s/corrupt_bodies.bin", remoteTempDir)
	if err := corruptFMAPSection(ctx, h.DUT, corruptBiosRemoteImage, backupBiosRemoteImage, remoteTempDir); err != nil {
		return errors.Wrap(err, "failed to corrupt FMAP sections")
	}

	// - Sign it.
	if out, err := futilityInstance.SignBIOS(ctx, futility.NewSignBIOSOptions(corruptBiosRemoteImage)); err != nil {
		return errors.Wrapf(err, "failed to sign corrupted firmware image: %s", string(out))
	}

	// - Extract the sections we want to test
	if out, err := futilityInstance.DumpFmapExtract(ctx, corruptBiosRemoteImage, map[string]string{
		sectionA: fmt.Sprintf("%s/%s.bin", remoteTempDir, sectionA),
		sectionB: fmt.Sprintf("%s/%s.bin", remoteTempDir, sectionB),
	}); err != nil {
		return errors.Wrapf(err, "failed to extract sections from corrupted firmware image: %s", string(out))
	}

	// - Create yet another image that contains those sections
	allCorruptBiosImageOnDut := fmt.Sprintf("%s/corrupt.bin", remoteTempDir)
	if out, err := futilityInstance.LoadFmap(ctx, backupBiosRemoteImage, allCorruptBiosImageOnDut, map[string]string{
		sectionA: fmt.Sprintf("%s/%s.bin", remoteTempDir, sectionA),
		sectionB: fmt.Sprintf("%s/%s.bin", remoteTempDir, sectionB),
	}); err != nil {
		return errors.Wrapf(err, "failed to load corrupted sections into fimrware image: %s", string(out))
	}

	testing.ContextLogf(ctx, "Flashing corrupt sections: %q %q", sectionA, sectionB)
	// - Flash it.
	shouldRestoreFirmware = true
	err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--wp=1", "--host_only", "-i", allCorruptBiosImageOnDut).Run(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed flashing corrupt fw")
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed creating mode switcher")
	}
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.SkipWaitConnect); err != nil {
		return errors.Wrap(err, "failed to reboot after corrupting")
	}
	waitContext, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	testing.ContextLogf(ctx, "Waiting %s(DelayRebootToPing) for DUT not to boot", h.Config.DelayRebootToPing)
	if err := h.WaitConnect(waitContext); err == nil {
		return errors.Wrap(err, "DUT is unexpectedly up, corruption failed")
	}
	if err := restoreFirmware(ctx); err != nil {
		return errors.Wrap(err, "restoreFirmware failed")
	}

	testing.ContextLog(ctx, "Checking eventlog for evidence of broken screen")

	// Sometimes events are missing if you check too quickly after boot.
	var events []reporters.Event
	if err := testing.Poll(ctx, func(context.Context) error {
		var err error
		events, err = h.Reporter.EventlogList(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}
		if len(events) == 0 {
			return errors.New("no new events found")
		}
		found := false
		for _, event := range events {
			if strings.Contains(event.Message, failureReason) {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("missing expected recovery reason %q in event log: %v", failureReason, events)
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute, Interval: 5 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed gathering events")
	}
	return nil
}
