// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

/*
This file implements functions to check or switch the DUT's boot mode.
*/

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const (
	// cmdTimeout is a short duration used for sending commands.
	cmdTimeout = 6 * time.Second

	// offTimeout is the timeout to wait for the DUT to be unreachable after powering off.
	offTimeout = 3 * time.Minute

	// PowerStateTimeout is the timeout to wait for the DUT reach a powerstate.
	PowerStateTimeout = 120 * time.Second

	// PowerStateInterval is the interval to wait before polling DUT powerstate.
	PowerStateInterval = 1 * time.Second

	// UsbVisibleTime is the time to wait after making the USB stick visible to DUT
	UsbVisibleTime = 5 * time.Second

	// UsbDisableTime is the time to wait for USB to be disabled.
	UsbDisableTime = 5 * time.Second
)

// ModeSwitcher enables booting the DUT into different firmware boot modes (normal, dev, rec).
type ModeSwitcher struct {
	Helper *Helper
}

// NewModeSwitcher creates a new ModeSwitcher. It relies on a firmware Helper to track dependent objects, such as servo and RPC client.
func NewModeSwitcher(ctx context.Context, h *Helper) (*ModeSwitcher, error) {
	if err := h.RequireConfig(ctx); err != nil {
		return nil, errors.Wrap(err, "requiring firmware config")
	}
	return &ModeSwitcher{
		Helper: h,
	}, nil
}

// ModeSwitchOption allows mode-switching methods to exhibit different behaviors.
type ModeSwitchOption int

const (
	// AllowGBBForce allows the DUT to force rebooting into dev mode via GBB flags.
	// This way of switching is more reliable, but is not appropriate for all tests.
	AllowGBBForce ModeSwitchOption = iota

	// AssumeGBBFlagsCorrect skips setting the GBB flags when switching modes.
	// This can save some time if the GBB flags are known to be in the desired state.
	AssumeGBBFlagsCorrect ModeSwitchOption = iota

	// CopyTastFiles copies the Tast files from the DUT before rebooting, and writes them back to the DUT afterwards.
	// This is necessary if you want to use any gRPC services.
	CopyTastFiles ModeSwitchOption = iota

	// SkipModeCheckAfterReboot can be passed in as an option to ModeAwareReboot, skipping
	// boot mode check after resetting DUT. One instance where this can be useful is
	// when verifying that FWMP prevents DUT from booting into dev mode.
	SkipModeCheckAfterReboot ModeSwitchOption = iota

	// VerifyECRO verifies that the EC is in RO after it boots up.
	VerifyECRO ModeSwitchOption = iota

	// VerifyGSCNoBoot verifies that the GSC's ec_comm command reports a boot mode of NO_BOOT.
	// Should be used with reset type APOff.
	VerifyGSCNoBoot ModeSwitchOption = iota

	// WaitSoftwareSync causes the mode switcher to sleep the `SoftwareSyncUpdate` time before finishing the boot.
	WaitSoftwareSync ModeSwitchOption = iota

	// SkipWaitConnect can be passed in as an option to ModeAwareReboot, skipping
	// waiting for establish connection after resetting DUT. It is useful when
	// system cannot boot due to a corrupted firmware.
	SkipWaitConnect ModeSwitchOption = iota

	// AssumeRecoveryMode cause skip checking current boot mode and assume that recovery is current boot mode.
	AssumeRecoveryMode ModeSwitchOption = iota

	// ExpectDevModeAfterReboot expect developer mode after reboot from recovery.
	ExpectDevModeAfterReboot ModeSwitchOption = iota

	// CheckToNoGoodScreen checks that DUT will not boot from an invalid USB,
	// but instead boot to the NOGOOD screen.
	CheckToNoGoodScreen ModeSwitchOption = iota

	// RebootForGBBFlagsChanged indicates that a reboot is required
	// if gbb flags were changed. This would be helpful in mode transition by
	// canceling gbb flag restriction first.
	RebootForGBBFlagsChanged ModeSwitchOption = iota
)

// msOptsContain determines whether a slice of ModeSwitchOptions contains a specific Option.
func msOptsContain(opts []ModeSwitchOption, want ModeSwitchOption) bool {
	for _, o := range opts {
		if o == want {
			return true
		}
	}
	return false
}

// RebootToMode reboots the DUT into the specified boot mode.
// This has the side-effect of disconnecting the RPC client.
// Requires `SoftwareDeps: []string{"crossystem", "flashrom"},`.
func (ms ModeSwitcher) RebootToMode(ctx context.Context, toMode fwCommon.BootMode, opts ...ModeSwitchOption) (errReturn error) {
	h := ms.Helper
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "requiring servo")
	}
	if err := h.RequireConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to require config at the start of RebootToMode")
	}
	waitConnectOpt := []WaitConnectOption{ResetEthernetDongle}
	if !h.DUT.Connected(ctx) {
		connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		if err := h.WaitConnect(connectCtx, waitConnectOpt...); err != nil {
			return errors.Wrap(err, "failed to ssh at the start of RebootToMode")
		}
	}
	fromMode, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		return errors.Wrap(err, "determining boot mode at the start of RebootToMode")
	}

	// Unless AssumeGBBFlagsCorrect is passed, fix the GBB flags for the desired boot mode.
	if !msOptsContain(opts, AssumeGBBFlagsCorrect) {
		flags := fwpb.GBBFlagsState{}
		if msOptsContain(opts, AllowGBBForce) {
			switch toMode {
			case fwCommon.BootModeDev:
				flags.Clear = append(flags.Clear, fwpb.GBBFlag_FORCE_DEV_BOOT_USB)
				flags.Set = append(flags.Set, fwpb.GBBFlag_FORCE_DEV_SWITCH_ON, fwpb.GBBFlag_DEV_SCREEN_SHORT_DELAY)
			case fwCommon.BootModeUSBDev:
				flags.Set = append(flags.Set, fwpb.GBBFlag_FORCE_DEV_BOOT_USB, fwpb.GBBFlag_FORCE_DEV_SWITCH_ON, fwpb.GBBFlag_DEV_SCREEN_SHORT_DELAY)
			default:
				flags.Clear = append(flags.Clear, fwpb.GBBFlag_FORCE_DEV_SWITCH_ON, fwpb.GBBFlag_DEV_SCREEN_SHORT_DELAY, fwpb.GBBFlag_FORCE_DEV_BOOT_USB)
			}
		} else {
			flags.Clear = append(flags.Clear, fwpb.GBBFlag_FORCE_DEV_SWITCH_ON, fwpb.GBBFlag_DEV_SCREEN_SHORT_DELAY, fwpb.GBBFlag_FORCE_DEV_BOOT_USB)
		}
		gbbFlagChanged, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &flags)
		if err != nil {
			return errors.Wrap(err, "setting GBB flags")
		}
		if gbbFlagChanged {
			opts = append(opts, RebootForGBBFlagsChanged)
		}
	}

	// When booting to a different image, such as normal vs. recovery, the new image might
	// not have Tast host files installed. So, store those files on the test server and reinstall later.
	fromModeUsb := false
	toModeUsb := false
	if fromMode == fwCommon.BootModeRecovery || fromMode == fwCommon.BootModeUSBDev {
		fromModeUsb = true
	}
	if toMode == fwCommon.BootModeRecovery || toMode == fwCommon.BootModeUSBDev {
		toModeUsb = true
	}

	if fromModeUsb != toModeUsb && !h.DoesServerHaveTastHostFiles() && msOptsContain(opts, CopyTastFiles) {
		if err := h.CopyTastFilesFromDUT(ctx); err != nil {
			return errors.Wrap(err, "copying Tast files from DUT to test server")
		}
		// Remember which image the Tast files came from.
		if fromModeUsb {
			h.dutUsbHasTastFiles = true
		} else {
			h.dutInternalStorageHasTastFiles = true
		}
	}

	defer func() {
		// Send Tast files back to DUT.
		if errReturn == nil {
			needSync := (toModeUsb != fromModeUsb) && msOptsContain(opts, CopyTastFiles)
			if toModeUsb {
				needSync = needSync && !h.dutUsbHasTastFiles
			} else {
				needSync = needSync && !h.dutInternalStorageHasTastFiles
			}
			if needSync {
				if err := h.SyncTastFilesToDUT(ctx); err != nil {
					errReturn = errors.Wrapf(err, "syncing Tast files to DUT after booting to %s", toMode)
					return
				}
				if toModeUsb {
					h.dutUsbHasTastFiles = true
				} else {
					h.dutInternalStorageHasTastFiles = true
				}
			}
		}
	}()

	if msOptsContain(opts, RebootForGBBFlagsChanged) {
		// If the dut was in dev mode with gbb as 0x108, and fixture.Dev wants to
		// transition the dut from rec to dev, for gbb flag change to 0x100, some
		// devices would get stuck at the "developer mode is already enabled" screen.
		// Applying a warm reset would generally help, putting the dut eventually
		// in dev mode with gbb flag set as 0x100, and satisfying the fixture.
		// However, if the boot mode that the dut had even earlier before 0x108
		// wasn't developer mode, but normal mode, then a warm reset would put the
		// dut in normal mode instead with gbb flag 0x100, failing the fixture.
		// Reboot here first for gbb flag change, check for the dut's current mode,
		// and then decide if mode transition is required.
		testing.ContextLog(ctx, "Resetting DUT due to GBB flag change")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			return errors.Wrap(err, "failed to cold reset dut")
		}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, waitConnectOpt...); err != nil {
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
	}

	fromMode, err = h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current boot mode")
	}
	if fromMode == toMode {
		testing.ContextLogf(ctx, "DUT is now in %s mode", toMode)
		return nil
	}

	// Booting from rec to anything else will cause EC to restart, potentally breaking the servo watchdog.
	if fromMode == fwCommon.BootModeRecovery {
		if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
			return errors.Wrap(err, "failed to remove watchdog for ccd")
		}
	}

	switch toMode {
	case fwCommon.BootModeNormal:
		testing.ContextLog(ctx, "Disabling dev request")
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "disable_dev_request=1").Run(ssh.DumpLogOnError); err != nil {
			return errors.Wrap(err, "sending disable dev request")
		}
		if err := ms.PowerOff(ctx); err != nil {
			return errors.Wrap(err, "powering off DUT")
		}
		testing.ContextLog(ctx, "Sleeping for 20 seconds")
		// GoBigSleepLint: b/268492022 Ensure DUT fully off before setting it on.
		if err := testing.Sleep(ctx, 20*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep for 20 seconds")
		}
		if ok, err := h.Servo.HasControl(ctx, string(servo.ImageUSBKeyPwr)); err != nil {
			return errors.Wrap(err, "failed checking control ImageUSBKeyPwr")
		} else if ok {
			if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
				return errors.Wrap(err, "disable usb for normal")
			}
		}
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			return err
		}
		if fromMode != fwCommon.BootModeNormal {
			if err := ms.FwScreenToNormalMode(ctx, opts...); err != nil {
				return errors.Wrap(err, "moving from firmware screen to normal mode")
			}
		} else {
			// Reconnect to the DUT.
			testing.ContextLog(ctx, "Reestablishing connection to DUT")
			connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
			defer cancel()
			if err := h.WaitConnect(connectCtx, waitConnectOpt...); err != nil {
				return errors.Wrapf(err, "failed to reconnect to DUT after booting to %s", toMode)
			}
		}
	case fwCommon.BootModeRecovery:
		// Recovery mode requires the DUT to boot the image on the USB.
		// Thus, the servo must show the USB to the DUT.
		if err := ms.EnableRecMode(ctx, servo.USBMuxDUT); err != nil {
			return err
		}
		// Reconnect to the DUT.
		testing.ContextLog(ctx, "Reestablishing connection to DUT")
		connectCtx, cancel := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
		defer cancel()
		if err := h.WaitConnect(connectCtx, waitConnectOpt...); err != nil {
			return errors.Wrapf(err, "failed to reconnect to DUT after booting to %s", toMode)
		}
	case fwCommon.BootModeDev:
		testing.ContextLog(ctx, "Disabling dev_boot_usb, disabling dev_boot_signed_only, enabling dev_request")
		if err := h.DUT.Conn().CommandContext(
			ctx, "crossystem", "dev_boot_usb=0", "dev_boot_signed_only=0", "dev_default_boot=disk", "disable_dev_request=0").Run(ssh.DumpLogOnError); err != nil {
			return errors.Wrap(err, "disabling dev_boot_usb")
		}
		if msOptsContain(opts, AllowGBBForce) {
			// 1. Set the GBB flag which forces dev mode upon reboot.
			//    This was handled earlier in this function, prior to terminating the RPC connection.
			// 2. Reboot the DUT.
			if err := h.DUT.Reboot(ctx); err != nil {
				return errors.Wrap(err, "rebooting DUT to force dev mode via GBB")
			}
			break
		}
		transitionToDev := true
		// Recovery -> Dev sometimes gets stuck on the recovery screen. Try a normal reboot first.
		// Even if it doesn't get us back to Dev, rebooting from Normal -> Dev is less flaky.
		if fromMode == fwCommon.BootModeRecovery || fromMode == fwCommon.BootModeUSBDev {
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
				return err
			}
			// Depending on how we got to to dev mode, we might end up in normal mode or the recovery
			// menu, so navigate to dev mode, but it that fails, fall through to the next attempt below.
			if err := ms.FwScreenToDevMode(ctx, opts...); err == nil {
				newMode, err := h.Reporter.CurrentBootMode(ctx)
				if err != nil {
					return errors.Wrap(err, "determining boot mode after simple reboot")
				}
				testing.ContextLogf(ctx, "Warm reset finished, DUT in %s", newMode)
				transitionToDev = newMode != fwCommon.BootModeDev
			}
		}
		if transitionToDev {
			// 1. Set power_state to 'rec', but don't show the DUT a USB image to boot from.
			// 2. From the firmware screen that appears, press keys to transition to dev mode.
			//    The specific keypresses will depend on the DUT's ModeSwitcherType.
			if err := ms.EnableRecMode(ctx, servo.USBMuxOff); err != nil {
				return err
			}
			if err := ms.FwScreenToDevMode(ctx, opts...); err != nil {
				return errors.Wrap(err, "moving from firmware screen to dev mode")
			}
		}
	case fwCommon.BootModeUSBDev:
		transitionToDev := true
		transitionToDevUsb := true
		if msOptsContain(opts, AllowGBBForce) || fromMode == fwCommon.BootModeDev {
			transitionToDev = false
		}
		// Recovery -> Dev sometimes gets stuck on the recovery screen. Try a normal reboot first.
		// Even if it doesn't get us back to Dev, rebooting from Normal -> Dev is less flaky.
		if fromMode == fwCommon.BootModeRecovery {
			testing.ContextLog(ctx, "Rebooting to leave recovery mode")
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
				return err
			}
			// Depending on how we got to to rec mode, we might end up in normal mode or the recovery
			// menu, so navigate to dev mode, but it that fails, fall through to the next attempt below.
			if err := ms.FwScreenToDevMode(ctx, opts...); err == nil {
				newMode, err := h.Reporter.CurrentBootMode(ctx)
				if err != nil {
					return errors.Wrap(err, "determining boot mode after simple reboot")
				}
				testing.ContextLogf(ctx, "Warm reset finished, DUT in %s", newMode)
				switch newMode {
				case fwCommon.BootModeDev:
					transitionToDev = false
				case fwCommon.BootModeUSBDev:
					transitionToDev = false
					transitionToDevUsb = false
				}
			}
		}
		if transitionToDev {
			// 1. Set power_state to 'rec', but don't show the DUT a USB image to boot from.
			// 2. From the firmware screen that appears, press keys to transition to dev mode.
			//    The specific keypresses will depend on the DUT's ModeSwitcherType.
			testing.ContextLog(ctx, "Rebooting to enter dev mode first")
			if err := ms.EnableRecMode(ctx, servo.USBMuxOff); err != nil {
				return err
			}
			if err := ms.FwScreenToDevMode(ctx, opts...); err != nil {
				return errors.Wrap(err, "moving from firmware screen to dev mode")
			}
			newMode, err := h.Reporter.CurrentBootMode(ctx)
			if err != nil {
				return errors.Wrap(err, "determining boot mode after reboot to dev")
			}
			testing.ContextLogf(ctx, "Reboot to dev finished, DUT in %s", newMode)

		}
		if transitionToDevUsb {
			// 1. Set power_state to 'rec', but don't show the DUT a USB image to boot from.
			// 2. From the firmware screen that appears, press keys to transition to dev mode.
			//    The specific keypresses will depend on the DUT's ModeSwitcherType.
			testing.ContextLog(ctx, "Enabling dev_boot_usb, disabling dev_boot_signed_only")
			if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=1", "dev_boot_signed_only=0", "dev_default_boot=usb").Run(ssh.DumpLogOnError); err != nil {
				return errors.Wrap(err, "enabling dev_boot_usb")
			}
			testing.ContextLog(ctx, "Enabling USB")
			if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
				return err
			}
			testing.ContextLogf(ctx, "Sleeping %s to let USB become visible to DUT", UsbVisibleTime)
			// GoBigSleepLint: It takes some time for usb mux state to take effect.
			if err := testing.Sleep(ctx, UsbVisibleTime); err != nil {
				return err
			}
			testing.ContextLog(ctx, "Rebooting")
			powerOffCtx, cancel := context.WithTimeout(ctx, cmdTimeout)
			defer cancel()
			if err := h.CloseRPCConnection(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close rpc connection: ", err)
			}
			// Since the DUT will power off, deadline exceeded is expected here.
			if err := h.DUT.Conn().CommandContext(powerOffCtx, "reboot").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				return errors.Wrap(err, "DUT poweroff")
			}

			offCtx, cancel := context.WithTimeout(ctx, offTimeout)
			defer cancel()
			if err := ms.Helper.DUT.WaitUnreachable(offCtx); err != nil {
				return errors.Wrap(err, "waiting for DUT to be unreachable after reboot")
			}
			if err := ms.fwScreenToUSBDevMode(ctx, opts...); err != nil {
				return errors.Wrap(err, "moving from firmware screen to usb dev mode")
			}
		}
		// Reconnect to the DUT.
		testing.ContextLog(ctx, "Reestablishing connection to DUT")
		connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		if err := h.WaitConnect(connectCtx, waitConnectOpt...); err != nil {
			return errors.Wrapf(err, "failed to reconnect to DUT after booting to %s", toMode)
		}
	default:
		return errors.Errorf("unsupported firmware boot mode: %s", toMode)
	}

	// Verify successful reboot.
	if curr, err := h.Reporter.CurrentBootMode(ctx); err != nil {
		return errors.Wrapf(err, "checking boot mode after reboot to %s", toMode)
	} else if curr != toMode {
		if curr == fwCommon.BootModeNormal && toMode == fwCommon.BootModeRecovery {
			testing.ContextLog(ctx, "Recovery boot ended up in normal, you may have a bad USB key or a truncated image")
			// Check for usb on the servo host.
			usbdev, err := h.CheckUSBOnServoHost(ctx)
			if err != nil {
				return errors.Wrap(err, "checking for the USB on servo host")
			}
			// Format usb before exiting to ensure that a valid image would be installed
			// during the next usb setup.
			if err := h.FormatUSB(ctx, usbdev); err != nil {
				return errors.Wrap(err, "failed to format the USB")
			}
			return errors.New("recovery boot ended up in normal. The usb has been formatted")
		}
		return errors.Errorf("incorrect boot mode after RebootToMode: got %s; want %s", curr, toMode)
	}
	testing.ContextLogf(ctx, "DUT is now in %s mode", toMode)
	return nil
}

// ResetType is an enum of ways to reset a DUT: warm and cold.
type ResetType string

// There are two ResetTypes: warm and cold.
const (
	// WarmReset uses the Servo control power_state=warm_reset.
	WarmReset ResetType = "warm"

	// ColdReset uses the Servo control power_state=reset.
	// It is identical to setting the power_state to off, then on.
	// It also resets the EC, as by the 'cold_reset' signal.
	ColdReset ResetType = "cold"

	// APOff reboots the EC with the ap-off flag.
	APOff ResetType = "ap-off"
)

// Each ResetType is associated with a particular servo.PowerStateValue.
var resetTypePowerState = map[ResetType]servo.PowerStateValue{
	WarmReset: servo.PowerStateWarmReset,
	ColdReset: servo.PowerStateReset,
}

// ModeAwareReboot resets the DUT with awareness of the DUT boot mode.
// Dev mode will be retained, but rec mode will default back to normal mode.
// This has the side-effect of disconnecting the RPC connection.
func (ms *ModeSwitcher) ModeAwareReboot(ctx context.Context, resetType ResetType, opts ...ModeSwitchOption) error {
	h := ms.Helper
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "requiring servo")
	}

	var fromMode fwCommon.BootMode
	if msOptsContain(opts, AssumeRecoveryMode) {
		fromMode = fwCommon.BootModeRecovery
	} else {
		var err error
		fromMode, err = h.Reporter.CurrentBootMode(ctx)
		if err != nil {
			return errors.Wrap(err, "determining boot mode at the start of ModeAwareReboot")
		}
	}

	// Memorize the boot ID, so that we can compare later.
	origBootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		return errors.Wrap(err, "determining boot ID before reboot")
	}

	// Perform sync prior to reboot, then close the RPC connection.
	if err := h.DUT.Conn().CommandContext(ctx, "sync").Run(ssh.DumpLogOnError); err != nil {
		testing.ContextLogf(ctx, "Failed to sync DUT: %s", err)
	}

	h.CloseRPCConnection(ctx)

	if fromMode == fwCommon.BootModeUSBDev {
		// The USB stick should already be visible, but set the direction just to be sure.
		testing.ContextLog(ctx, "Enabling USB")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			return err
		}
		testing.ContextLogf(ctx, "Sleeping %s to let USB become visible to DUT", UsbVisibleTime)
		// GoBigSleepLint: It takes some time for usb mux state to take effect.
		if err := testing.Sleep(ctx, UsbVisibleTime); err != nil {
			return err
		}
	}

	// Reset DUT
	if resetType == APOff {
		testing.ContextLog(ctx, "Reboot EC ap-off")
		if err := h.Servo.RunECCommand(ctx, "reboot ap-off"); err != nil {
			return errors.Wrap(err, "failed to reboot EC")
		}
		// GoBigSleepLint: It takes a little time before the boot mode can be checked.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		if msOptsContain(opts, VerifyGSCNoBoot) {
			if err := h.Servo.CheckGSCBootMode(ctx, []string{"NO_BOOT", "NoBoot"}); err != nil {
				return errors.Wrap(err, "gsc boot mode")
			}
		}
	} else {
		powerState, ok := resetTypePowerState[resetType]
		if !ok {
			return errors.Errorf("no power state associated with resetType %v", resetType)
		}
		if err := h.Servo.SetPowerState(ctx, powerState); err != nil {
			return err
		}
		// Verify DUT becomes unreachable after triggering a reset.
		waitDisconnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 10*time.Second)
		defer cancelWaitConnect()

		if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
			if resetType == WarmReset {
				if wrerr := h.Servo.SetStringAndCheck(ctx, servo.WarmReset, "off"); wrerr != nil {
					return errors.Wrapf(err, "failed to set %s to off after sending 'power_state:warm_reset': %v", servo.WarmReset, wrerr)
				}
			}
			return errors.Wrapf(err, "failed to reset DUT: %s", resetType)
		}
	}

	if msOptsContain(opts, VerifyECRO) {
		if activeCopy, err := h.Servo.GetString(ctx, "ec_active_copy"); err != nil {
			return errors.Wrap(err, "failed to get ec_active_copy")
		} else if activeCopy != "RO" {
			return errors.Errorf("EC active copy incorrect, got %q want RO", activeCopy)
		}
	}

	// If AP off, finish booting
	if resetType == APOff {
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurShortPress); err != nil {
			return errors.Wrap(err, "failed to press powerkey")
		}
	}

	if msOptsContain(opts, SkipWaitConnect) {
		if msOptsContain(opts, WaitSoftwareSync) {
			testing.ContextLogf(ctx, "Sleeping %s (SoftwareSyncUpdate)", h.Config.SoftwareSyncUpdate)
			// GoBigSleepLint: The caller expects that the software sync is done before returning.
			if err := testing.Sleep(ctx, h.Config.SoftwareSyncUpdate); err != nil {
				return errors.Wrapf(err, "sleeping for %s (SoftwareSyncUpdate)", h.Config.SoftwareSyncUpdate)
			}
		}

		testing.ContextLog(ctx, "Skip waiting to establish connection to DUT")
		return nil
	}

	// If in dev mode, bypass the TO_DEV screen.
	if fromMode == fwCommon.BootModeDev {
		if err := ms.FwScreenToDevMode(ctx, opts...); err != nil {
			return errors.Wrap(err, "fw screen to developer mode")
		}
	} else if fromMode == fwCommon.BootModeUSBDev {
		if err := ms.fwScreenToUSBDevMode(ctx, opts...); err != nil {
			return errors.Wrap(err, "bypassing fw screen")
		}
	} else {
		waitConnectOpt := []WaitConnectOption{ResetEthernetDongle}
		testing.ContextLog(ctx, "Reestablishing connection to DUT")
		connectTime := h.Config.DelayRebootToPing
		if msOptsContain(opts, WaitSoftwareSync) {
			connectTime += h.Config.SoftwareSyncUpdate
		}
		connectCtx, cancel := context.WithTimeout(ctx, connectTime)
		defer cancel()
		if err := h.WaitConnect(connectCtx, waitConnectOpt...); err != nil {
			return errors.Wrap(err, "failed to connect to DUT")
		}
	}
	if bootID, err := h.Reporter.BootID(ctx); err != nil {
		return errors.Wrap(err, "reporting boot ID")
	} else if bootID == origBootID {
		return errors.Errorf("new boot ID == old boot ID: %s", bootID)
	}

	// Verify successful reboot.
	// Dev mode should be preserved, but recovery mode will be lost in the reset (will go back to normal or dev mode).
	var expectMode fwCommon.BootMode
	if fromMode == fwCommon.BootModeRecovery {
		if msOptsContain(opts, ExpectDevModeAfterReboot) {
			expectMode = fwCommon.BootModeDev
		} else {
			expectMode = fwCommon.BootModeNormal
		}
	} else {
		expectMode = fromMode
	}
	if curr, err := h.Reporter.CurrentBootMode(ctx); err != nil {
		return errors.Wrapf(err, "checking boot mode after resetting from %s", fromMode)
	} else if curr != expectMode && !msOptsContain(opts, SkipModeCheckAfterReboot) {
		return errors.Errorf("incorrect boot mode after resetting DUT: got %s; want %s", curr, expectMode)
	}
	return nil
}

// FwScreenToNormalMode moves the DUT from the firmware bootup screen to Normal mode.
// This should be called immediately after powering on.
// The actual behavior depends on the ModeSwitcherType.
func (ms *ModeSwitcher) FwScreenToNormalMode(ctx context.Context, opts ...ModeSwitchOption) error {
	h := ms.Helper
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "requiring servo")
	}
	// Keep pressing the normal mode keys until connected, but wait a little longer for the connect each time.
	connectTimeout := 2 * time.Second
	waitConnectOpt := []WaitConnectOption{ResetEthernetDongle}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		switch h.Config.ModeSwitcherType {
		case KeyboardDevSwitcher:
			// Press both SPACE and ENTER so that we can handle both with and without ENTER_TRIGGERS_TONORM, and the menu UI which doesn't use SPACE.
			testing.ContextLog(ctx, "Pressing SPACE")
			if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
				return errors.Wrap(err, "pressing SPACE on firmware screen while disabling dev mode")
			}
			testing.ContextLogf(ctx, "Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
			// GoBigSleepLint: Sleeping for model specific time.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
			}
			testing.ContextLog(ctx, "Pressing ENTER")
			if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
				return errors.Wrap(err, "pressing Enter on firmware screen while disabling dev mode")
			}
			testing.ContextLogf(ctx, "Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
			// GoBigSleepLint: Sleeping for model specific time.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
			}
			testing.ContextLog(ctx, "Pressing ENTER")
			if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
				return errors.Wrap(err, "pressing Enter on confirm screen while disabling dev mode")
			}
		case MenuSwitcher:
			// 1. Sleep for [FirmwareScreen] seconds.
			// 2. Press Ctrl+S.
			// 3. Sleep for [KeypressDelay] seconds.
			// 4. Press enter.
			testing.ContextLog(ctx, "Pressing Ctrl-S")
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlS, servo.DurTab); err != nil {
				return errors.Wrap(err, "pressing Ctrl-S on firmware screen while disabling dev mode")
			}
			testing.ContextLogf(ctx, "Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
			// GoBigSleepLint: Sleeping for model specific time.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
			}
			testing.ContextLog(ctx, "Pressing ENTER")
			if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
				return errors.Wrap(err, "pressing Enter on confirm screen while disabling dev mode")
			}
		case TabletDetachableSwitcher:
			// 1. Wait until the firmware screen appears.
			// 2. Hold volume_up for 100ms to highlight the previous menu item (Enable Root Verification).
			// 3. Sleep for [KeypressDelay] seconds to confirm keypress.
			// 4. Press power to select Enable Root Verification.
			// 5. Sleep for [KeypressDelay] seconds to confirm keypress.
			// 6. Wait until the TO_NORM screen appears.
			// 7. Press power to select Confirm Enabling Verified Boot.
			if err := h.Servo.SetInt(ctx, servo.VolumeUpHold, 100); err != nil {
				return errors.Wrap(err, "changing menu selection to 'Enable Root Verification'")
			}
			// GoBigSleepLint: Sleeping for model specific time.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
			}
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
				return errors.Wrap(err, "selecting menu option 'Enable Root Verification'")
			}
			// GoBigSleepLint: Sleeping for model specific time.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
			}
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
				return errors.Wrap(err, "selecting menu option 'Confirm Enabling Verified Boot'")
			}
		default:
			return errors.Errorf("unsupported ModeSwitcherType %s for FwScreenToNormalMode", h.Config.ModeSwitcherType)
		}
		ctx, cancel := context.WithTimeout(ctx, connectTimeout)
		defer cancel()
		connectTimeout += time.Second
		if err := h.WaitConnect(ctx, waitConnectOpt...); err != nil {
			waitConnectOpt = nil
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: h.Config.DelayRebootToPing + h.Config.FirmwareScreen}); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	return nil
}

// FwScreenToDevMode moves the DUT from the firmware bootup screen to Dev mode.
// This should be called immediately after powering on.
// The actual behavior depends on the ModeSwitcherType.
func (ms *ModeSwitcher) FwScreenToDevMode(ctx context.Context, opts ...ModeSwitchOption) error {
	h := ms.Helper
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "requiring servo")
	}

	preSleepTime := h.Config.FirmwareScreen
	if msOptsContain(opts, WaitSoftwareSync) {
		preSleepTime += h.Config.SoftwareSyncUpdate
	}

	if msOptsContain(opts, CheckToNoGoodScreen) {
		testing.ContextLogf(ctx, "Sleeping %s (preSleepTime)", preSleepTime)
		// GoBigSleepLint: Sleeping for model specific time.
		if err := testing.Sleep(ctx, preSleepTime); err != nil {
			return errors.Wrapf(err, "sleeping for %s (preSleepTime) to wait for INSERT screen", preSleepTime)
		}
		preSleepTime = 0

		// Enable USB connection to DUT.
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			return errors.Wrap(err, "failed to set 'usb3_mux_sel:dut_sees_usbkey'")
		}
		testing.ContextLog(ctx, "Checking if DUT reaches the NOGOOD screen")
		if err := h.WaitDUTConnectDuringBootFromUSB(ctx, false); err != nil {
			return errors.Wrap(err, "failed to check NOGOOD screen")
		}
		// Remove USB from DUT.
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
			return errors.Wrap(err, "failed to power off usbkey")
		}
		// GoBigSleepLint: It takes some time for usb mux state to take effect.
		if err := testing.Sleep(ctx, UsbDisableTime); err != nil {
			return errors.Wrap(err, "failed to sleep after setting usb mux state disable")
		}
		testing.ContextLog(ctx, "Booting to the recovery screen with the bad USB powered off")
	}

	switch h.Config.ModeSwitcherType {
	case MenuSwitcher:
		// Same as KeyboardDevSwitcher.
		fallthrough
	case KeyboardDevSwitcher:
		// 1. Wait until the firmware screen appears.
		// 2. Press Ctrl-D to move to the confirm screen.
		// 3. Wait until the confirm screen appears.
		// 4. Push some button depending on the DUT's config: toggle the rec button, press power, or press enter.
		connectTimeout := 2 * time.Second
		waitConnectOpt := []WaitConnectOption{ResetEthernetDongle}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			testing.ContextLog(ctx, "Pressing CTRL-D")
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
				return err
			}
			testing.ContextLogf(ctx, "Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
			// GoBigSleepLint: Sleeping for model specific time.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return err
			}
			if h.Config.RecButtonDevSwitch {
				testing.ContextLog(ctx, "Toggling RecMode")
				if err := h.Servo.ToggleOnOff(ctx, servo.RecMode); err != nil {
					return err
				}
			} else if h.Config.PowerButtonDevSwitch {
				testing.ContextLog(ctx, "Sleeping for 5 seconds")
				// GoBigSleepLint: Frequent presses of the power key might power off the dut accidentally.
				// Add a short delay to ensure that it doesn't get enforced at the wrong time, for example,
				// on the "OS verification is OFF" screen, or when the dut is already past the firmware
				// screens, and on the way to ChromeOS.
				if err := testing.Sleep(ctx, 5*time.Second); err != nil {
					return err
				}
				testing.ContextLog(ctx, "Pressing power key")
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
					return err
				}
				testing.ContextLog(ctx, "Sleeping for 5 seconds")
				// GoBigSleepLint: On drallion devices, servo.PowerKey press takes times to become effective.
				// Add a short delay here to avoid pressing the esc key too early, and bringing the DUT back
				// to the insert screen, where pressing servo.PowerKey would power off the machine.
				if err := testing.Sleep(ctx, 5*time.Second); err != nil {
					return err
				}
				// For wilco devices, pressing the power button twice on the welcome page will power-off the DUT.
				// Press esc key to cancel the power menu so that the dut won't get powered off accidentally.
				testing.ContextLog(ctx, "Pressing esc key")
				if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
					return errors.Wrap(err, "failed to press the esc key")
				}
			} else if h.Config.IsDetachable {
				// When transitioning from normal mode to dev mode, TO_DEV screen allows only truested input to
				// press `Confirm`. For the detachable with menu UI, the power key is the only trusted input.
				testing.ContextLog(ctx, "Pressing power key")
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
					return err
				}
			} else {
				testing.ContextLog(ctx, "Pressing enter key")
				if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
					return err
				}
			}
			testing.ContextLog(ctx, "Set DFP mode")
			if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
				testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %s", err)
			}
			ctx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()
			connectTimeout += time.Second
			if err := h.WaitConnect(ctx, waitConnectOpt...); err != nil {
				waitConnectOpt = nil
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: h.Config.DelayRebootToPing + preSleepTime}); err != nil {
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
	case TabletDetachableSwitcher:
		// 1. Wait [FirmwareScreen] seconds for the INSERT screen to appear.
		// 2. Hold both VolumeUp and VolumeDown for 100ms to trigger TO_DEV screen.
		// 3. Wait [KeypressDelay] seconds to confirm keypress.
		// 4. Hold VolumeUp for 100ms to change menu selection to 'Confirm enabling developer mode'.
		// 5. Wait [KeypressDelay] seconds to confirm keypress.
		// 6. Press PowerKey to select menu item.
		// 7. Wait [KeypressDelay] seconds to confirm keypress.
		// 8. Wait [FirmwareScreen] seconds to transition screens.
		testing.ContextLogf(ctx, "Sleeping %s (preSleepTime)", preSleepTime)
		// GoBigSleepLint: Sleeping for model specific time.
		if err := testing.Sleep(ctx, preSleepTime); err != nil {
			return errors.Wrapf(err, "sleeping for %s (preSleepTime) to wait for INSERT screen", preSleepTime)
		}

		if err := h.Servo.SetInt(ctx, servo.VolumeUpDownHold, 100); err != nil {
			return errors.Wrap(err, "triggering TO_DEV screen")
		}
		// GoBigSleepLint: Sleeping for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "sleeping for %s (KeypressDelay) to confirm triggering TO_DEV screen", h.Config.KeypressDelay)
		}
		if err := h.Servo.SetInt(ctx, servo.VolumeUpHold, 100); err != nil {
			return errors.Wrap(err, "changing menu selection to 'Confirm enabling developer mode' on TO_DEV screen")
		}
		// GoBigSleepLint: Sleeping for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "sleeping for %s (KeypressDelay) to confirm changing menu selection on TO_DEV screen", h.Config.KeypressDelay)
		}
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
			return errors.Wrap(err, "selecting menu item 'Confirm enabling developer mode' on TO_DEV screen")
		}
		// Reconnect to the DUT.
		waitConnectOpt := []WaitConnectOption{ResetEthernetDongle}
		connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		if err := h.WaitConnect(connectCtx, waitConnectOpt...); err != nil {
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
	default:
		return errors.Errorf("booting to dev mode: unsupported ModeSwitcherType: %s", h.Config.ModeSwitcherType)
	}
	return nil
}

// fwScreenToUSBDevMode moves the DUT from the firmware bootup screen to USB Dev mode.
// This should be called immediately after powering on.
// The actual behavior depends on the ModeSwitcherType.
func (ms *ModeSwitcher) fwScreenToUSBDevMode(ctx context.Context, opts ...ModeSwitchOption) error {
	h := ms.Helper
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "requiring servo")
	}

	preSleepTime := h.Config.FirmwareScreen
	if msOptsContain(opts, WaitSoftwareSync) {
		preSleepTime += h.Config.SoftwareSyncUpdate
	}

	switch h.Config.ModeSwitcherType {
	case MenuSwitcher:
		// Same as KeyboardDevSwitcher.
		fallthrough
	case KeyboardDevSwitcher:
		// 1. Wait until the firmware screen appears.
		// 2. Press Ctrl-U to move to the confirm screen.
		// 3. Wait until the confirm screen appears.
		// 4. Push some button depending on the DUT's config: toggle the rec button, press power, or press enter.
		testing.ContextLog(ctx, "Set DFP mode")
		if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
			testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %s", err)
		}
		// Keep pressing CTRL-U until connected, but wait a little longer for the connect each time.
		connectTimeout := 2 * time.Second
		waitConnectOpt := []WaitConnectOption{ResetEthernetDongle}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			testing.ContextLog(ctx, "Pressing CTRL-U")
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()
			connectTimeout += time.Second
			if err := h.WaitConnect(ctx, waitConnectOpt...); err != nil {
				waitConnectOpt = nil
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: h.Config.USBImageBootTimeout + preSleepTime}); err != nil {
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
	default:
		return errors.Errorf("booting to dev mode: unsupported ModeSwitcherType: %s", h.Config.ModeSwitcherType)
	}

	return nil
}

// EnableRecMode powers the DUT into the "rec" state, but does not wait to reconnect to the DUT.
// If booting into rec mode, usbMux should point to the DUT, so that the DUT can finish booting into recovery mode.
// Otherwise, usbMux should be off. This will prevent the DUT from transitioning to rec mode, so other operations can be performed (such as bypassing to dev mode).
func (ms *ModeSwitcher) EnableRecMode(ctx context.Context, usbMux servo.USBMuxState) error {
	h := ms.Helper
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "requiring servo")
	}

	if h.DUT.Connected(ctx) {
		// Stainless reported thermal shutdown on some dedede duts while they
		// were booting into recovery mode. Check for the temperature information
		// before power-off for debugging purposes.
		out, err := h.DUT.Conn().CommandContext(ctx, "ectool", "temps", "all").Output()
		if err != nil {
			testing.ContextLog(ctx, "Failed to run ectool: ", err)
		}
		temps := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(temps) < 2 {
			testing.ContextLog(ctx, "Did not find temperature data")
		} else {
			testing.ContextLog(ctx, "Found temperature data")
			for _, val := range temps {
				testing.ContextLog(ctx, val)
			}
		}
	}

	if err := ms.PowerOff(ctx); err != nil {
		return errors.Wrap(err, "powering off DUT")
	}
	testing.ContextLog(ctx, "Sleeping for 20 seconds")
	// GoBigSleepLint: b/268492022 Ensure DUT fully off before setting it on.
	if err := testing.Sleep(ctx, 20*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for 20 seconds")
	}
	// Booting into recovery mode seems to work better if you don't enable the USB key until after the recovery power state.
	if usbMux == servo.USBMuxDUT {
		testing.ContextLog(ctx, "Powering off USB")
		if err := h.Servo.SetUSBMuxState(ctx, usbMux); err != nil {
			return errors.Wrapf(err, "setting usb mux state to %s while DUT is off", usbMux)
		}
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
			return errors.Wrapf(err, "setting usb mux state to %s while DUT is off", usbMux)
		}
	}
	// GoBigSleepLint: Powering off the USB mux has some side effects that take some time. Specifically, you can't turn
	// it back on again too quickly or the USB stick fails.
	if err := testing.Sleep(ctx, UsbDisableTime); err != nil {
		return errors.Wrapf(err, "sleeping before setting usb mux state to %s", usbMux)
	}
	if usbMux != servo.USBMuxDUT {
		if ok, err := h.Servo.HasControl(ctx, string(servo.ImageUSBKeyPwr)); err != nil {
			return errors.Wrap(err, "failed checking control ImageUSBKeyPwr")
		} else if ok {
			if err := h.Servo.SetUSBMuxState(ctx, usbMux); err != nil {
				return errors.Wrapf(err, "setting usb mux state to %s while DUT is off", usbMux)
			}
		}
	}
	// According to Stainless, some DUTs were stuck at G3 while
	// booting to recovery mode. Their ec logs reported that they
	// were experiencing thermal shutdown. Match for the relevant
	// texts and report in the returned error. If thermal shutdown
	// is caught, attempt a few more retries to boot dut to recovery.
	var ecStream string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		testing.ContextLog(ctx, "Capturing EC log")
		if err := h.Servo.SetOnOff(ctx, servo.ECUARTCapture, servo.On); err != nil {
			return errors.Wrap(err, "failed to set ec_uart_capture on")
		}
		defer func() {
			if err := h.Servo.SetOnOff(ctx, servo.ECUARTCapture, servo.Off); err != nil {
				testing.ContextLog(ctx, "Failed to disable ec_uart_capture: ", err)
			}
			// Sometimes capturing thermal shutdown fails because of
			// noise data, for example, "thermal SHU[TDOWN". Setting
			// the 'chan' command before to hear from a particular channel
			// would not work because sending servo.PowerStateRec later
			// reverses this setting, and enables all channels back.
			// Save and upload the uart log to Stainless for debugging purposes.
			outDir, ok := testing.ContextOutDir(ctx)
			if ok {
				destPath := filepath.Join(outDir, "ecUart.log")
				if err := ioutil.WriteFile(destPath, []byte(ecStream), 0666); err != nil {
					testing.ContextLog(ctx, "Failed to write ecUart.log: ", err)
				}
			} else {
				testing.ContextLog(ctx, "Failed to find test output directory")
			}

		}()

		if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
			return errors.Wrapf(err, "setting power state to %s", servo.PowerStateRec)
		}
		out, err := h.Servo.GetQuotedString(ctx, servo.ECUARTStream)
		if err != nil {
			return errors.Wrap(err, "failed to read ec stream")
		}
		ecStream = out

		// Based on chipset_shutdown_reason defined in ec_command.h found under the
		// ec repo, 32776 would refer to thermal shutdown.
		var (
			regexpThermalShutdown      = `(?i)thermal shutdown`
			regexpShutdownReason       = `chipset_force_shutdown\(\)\s+32776`
			regexpMatchThermalShutdown = `(` + regexpThermalShutdown + `|` + regexpShutdownReason + `)`
		)
		thermalShutdown := regexp.MustCompile(regexpMatchThermalShutdown).FindStringSubmatch(ecStream)
		if len(thermalShutdown) != 0 {
			testing.ContextLog(ctx, "Warning!!! Captured thermal shutdown")
			currPowerState, err := h.Servo.GetECSystemPowerState(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to check for powerstate after thermal shutdown detected")
			}
			if currPowerState == "G3" {
				return errors.Errorf("captured %s at G3 after rebooting dut to recovery", thermalShutdown)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Minute, Interval: 3 * time.Second}); err != nil {
		return err
	}

	if usbMux == servo.USBMuxDUT {
		testing.ContextLog(ctx, "Enabling USB")
		if err := h.Servo.SetUSBMuxState(ctx, usbMux); err != nil {
			return errors.Wrapf(err, "setting usb mux state to %s at rec screen", usbMux)
		}
	}
	return nil
}

// PowerOff safely powers off the DUT with the "poweroff" command, then waits for the DUT to be unreachable.
func (ms *ModeSwitcher) PowerOff(ctx context.Context) error {
	h := ms.Helper
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "requiring servo")
	}
	testing.ContextLog(ctx, "Powering off DUT")
	powerOffCtx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	if err := h.CloseRPCConnection(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close rpc connection: ", err)
	}
	if h.DUT.Connected(ctx) {
		// Since the DUT will power off, deadline exceeded is expected here.
		if err := h.DUT.Conn().CommandContext(powerOffCtx, "poweroff").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return errors.Wrap(err, "DUT poweroff")
		}

		// Try reading the power state from the EC.
		err := h.WaitForPowerStates(ctx, PowerStateInterval, PowerStateTimeout, "G3", "S5")
		if err == nil {
			return nil
		}
		testing.ContextLogf(ctx, "Failed to get G3 or S5 power state: %s", err)
	}

	// We didn't reach G3/S5 so try having servo power off instead.
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
		return errors.Wrap(err, "set power_state:off")
	}

	// If the EC didn't return a power state, try wait unreachable instead.
	if err := ms.waitUnreachable(ctx); err != nil {
		return errors.Wrap(err, "waiting for DUT to be unreachable after sending poweroff command")
	}
	return nil
}

func (ms *ModeSwitcher) waitUnreachable(ctx context.Context) error {
	offCtx, cancel := context.WithTimeout(ctx, offTimeout)
	defer cancel()
	if err := ms.Helper.DUT.WaitUnreachable(offCtx); err != nil {
		return errors.Wrap(err, "waiting for DUT to be unreachable after powering off")
	}
	return nil
}
