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

	// powerOffTimeout is the timeout to wait for the DUT reach G3 or ssh to disconnect before trying harder to power off.
	powerOffTimeout = 30 * time.Second

	// PowerStateInterval is the interval to wait before polling DUT powerstate.
	PowerStateInterval = 1 * time.Second

	// UsbVisibleTime is the time to wait after making the USB stick visible to DUT
	UsbVisibleTime = 5 * time.Second

	// UsbDisableTime is the time to wait for USB to be disabled.
	UsbDisableTime = 5 * time.Second

	// DevScreenShortDelay is the reduced timeout for the dev screen
	// if GBBFlag_DEV_SCREEN_SHORT_DELAY was set.
	DevScreenShortDelay = 2 * time.Second

	// DevScreenTimeout is the default timeout for the dev screen.
	DevScreenTimeout = 30 * time.Second
)

// ModeSwitcher enables booting the DUT into different firmware boot modes (normal, dev, rec).
type ModeSwitcher struct {
	Helper *Helper
	bypasser
}

// NewModeSwitcher creates a new ModeSwitcher. It relies on a firmware Helper to track dependent objects, such as servo and RPC client.
func NewModeSwitcher(ctx context.Context, h *Helper) (*ModeSwitcher, error) {
	if err := h.RequireConfig(ctx); err != nil {
		return nil, errors.Wrap(err, "requiring firmware config")
	}
	newbp, err := NewBypasser(ctx, h)
	if err != nil {
		return nil, errors.Wrap(err, "creating a new bypasser")
	}
	return &ModeSwitcher{
		Helper:   h,
		bypasser: newbp,
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

	// RebootForGBBFlagsChanged indicates that a reboot is required
	// if gbb flags were changed. This would be helpful in mode transition by
	// canceling gbb flag restriction first.
	RebootForGBBFlagsChanged ModeSwitchOption = iota

	// RecoveryForceMRCBoot specifies rec_force_mrc instead of rec for booting to recovery mode.
	RecoveryForceMRCBoot ModeSwitchOption = iota
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
func (ms *ModeSwitcher) RebootToMode(ctx context.Context, toMode fwCommon.BootMode, opts ...ModeSwitchOption) (errReturn error) {
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

	devModeBypasserParams := RunBypasser{BypasserMethod: ms.bypasser.BypassDevMode, RepeatBypasser: true, WaitUntilDUTConnected: h.Config.DelayRebootToPing}
	if msOptsContain(opts, WaitSoftwareSync) {
		devModeBypasserParams.WaitUntilDUTConnected += h.Config.SoftwareSyncUpdate
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
		// Reconnect to the DUT.
		testing.ContextLog(ctx, "Reestablishing connection to DUT")
		connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		if err := h.WaitConnect(connectCtx, waitConnectOpt...); err != nil {
			return errors.Wrapf(err, "failed to reconnect to DUT after booting to %s", toMode)
		}
	case fwCommon.BootModeRecovery:
		// Recovery mode requires the DUT to boot the image on the USB.
		// Thus, the servo must show the USB to the DUT.
		recType := servo.PowerStateRec
		if msOptsContain(opts, RecoveryForceMRCBoot) {
			recType = servo.PowerStateRecForceMRC
		}
		if err := ms.EnableRecMode(ctx, recType, servo.USBMuxDUT); err != nil {
			return err
		}
		if msOptsContain(opts, SkipWaitConnect) {
			return nil
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
			if err := ms.RunBypasserUntilDUTConnected(ctx, devModeBypasserParams); err == nil {
				newMode, err := h.Reporter.CurrentBootMode(ctx)
				if err != nil {
					return errors.Wrap(err, "determining boot mode after simple reboot")
				}
				testing.ContextLogf(ctx, "Warm reset finished, DUT in %s", newMode)
				transitionToDev = newMode != fwCommon.BootModeDev
			}
		}
		if transitionToDev {
			closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
			if err != nil {
				return errors.Wrap(err, "failed to enable capture EC UART")
			}
			defer func() { errReturn = errors.Join(errReturn, closeUART(ctx)) }()
			// 1. Set power_state to 'rec', but don't show the DUT a USB image to boot from.
			// 2. From the firmware screen that appears, press keys to transition to dev mode.
			//    The specific keypresses will depend on the DUT's ModeSwitcherType.
			if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
				return err
			}
			if err := ms.RecScreenToDevMode(ctx, opts...); err != nil {
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
			if err := ms.RunBypasserUntilDUTConnected(ctx, devModeBypasserParams); err == nil {
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
			closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
			if err != nil {
				return errors.Wrap(err, "failed to enable capture EC UART")
			}
			defer func() { errReturn = errors.Join(errReturn, closeUART(ctx)) }()
			if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
				return err
			}
			if err := ms.RecScreenToDevMode(ctx, opts...); err != nil {
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
// Pass the option AllowGBBForce if you know that the DUT is using GBB flags and won't wait for Ctrl-D or Ctrl-U to be pressed.
func (ms *ModeSwitcher) ModeAwareReboot(ctx context.Context, resetType ResetType, opts ...ModeSwitchOption) (retErr error) {
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
		if msOptsContain(opts, VerifyGSCNoBoot) {
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err := h.Servo.CheckGSCBootMode(ctx, []string{"NO_BOOT", "NoBoot"}); err != nil {
					return errors.Wrap(err, "CheckGSCBootMode")
				}
				return nil
			}, &testing.PollOptions{
				Timeout: 1 * time.Minute,
			}); err != nil {
				return errors.Wrap(err, "gsc boot mode")
			}
		}
	} else {
		// Capture EC UART if booting into dev mode.
		if fromMode == fwCommon.BootModeDev && !msOptsContain(opts, AllowGBBForce) {
			closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
			if err != nil {
				return errors.Wrap(err, "failed to enable capture EC UART")
			}
			defer func() { retErr = errors.Join(retErr, closeUART(ctx)) }()
		}
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
		testing.ContextLog(ctx, "Wait 2 seconds for AC and battery initialization to complete")
		// GoBigSleepLint: Because storo sets the minimum power requirement at starup.So add 2S sleep before pressing power button to make battery and adapter initialization done.
		if err := testing.Sleep(ctx, 2*time.Second); err != nil {
			return errors.Wrap(err, "Fail to sleep ")
		}
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
	if fromMode == fwCommon.BootModeDev && !msOptsContain(opts, AllowGBBForce) {
		testing.ContextLog(ctx, "Waiting for DUT to reach the firmware screen before bypassing dev")
		if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreen); err != nil {
			return errors.Wrap(err, "failed to get to firmware screen")
		}
		params := RunBypasser{BypasserMethod: ms.bypasser.BypassDevMode, RepeatBypasser: true, WaitUntilDUTConnected: h.Config.DelayRebootToPing}
		if msOptsContain(opts, WaitSoftwareSync) {
			params.WaitUntilDUTConnected += h.Config.SoftwareSyncUpdate
		}
		if err := ms.RunBypasserUntilDUTConnected(ctx, params); err != nil {
			return errors.Wrap(err, "bypass dev mode")
		}
	} else if fromMode == fwCommon.BootModeUSBDev && !msOptsContain(opts, AllowGBBForce) {
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
			return errors.Wrapf(err, "failed to connect to DUT within %s", connectTime)
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

// RecScreenToDevMode moves the DUT from the firmware bootup screen to Dev mode.
// This should be called immediately after powering on.
// The actual behavior depends on the ModeSwitcherType.
func (ms *ModeSwitcher) RecScreenToDevMode(ctx context.Context, opts ...ModeSwitchOption) error {
	h := ms.Helper
	testing.ContextLog(ctx, "Waiting for DUT to reach the firmware screen before TriggerRecToDev")
	if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreenRecMode); err != nil {
		return errors.Wrap(err, "failed to get to firmware screen")
	}

	if err := ms.bypasser.TriggerRecToDev(ctx); err != nil {
		return errors.Wrap(err, "failed to bypass to dev")
	}

	// It takes some time to powerwash, etc. when going from normal -> dev, and at the end, we're at the dev mode screen and need to press Ctrl-D (or equivalent).
	// Reconnect to the DUT.
	params := RunBypasser{BypasserMethod: ms.bypasser.BypassDevMode, RepeatBypasser: true, WaitUntilDUTConnected: h.Config.DelayRebootToPing + h.Config.FirmwareScreen}
	if msOptsContain(opts, WaitSoftwareSync) {
		params.WaitUntilDUTConnected += h.Config.SoftwareSyncUpdate
	}

	return ms.RunBypasserUntilDUTConnected(ctx, params)
}

// fwScreenToUSBDevMode moves the DUT from the firmware bootup screen to USB Dev mode.
// This should be called immediately after powering on.
// The actual behavior depends on the ModeSwitcherType.
func (ms *ModeSwitcher) fwScreenToUSBDevMode(ctx context.Context, opts ...ModeSwitchOption) error {
	h := ms.Helper
	testing.ContextLog(ctx, "Set DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
	}
	totalTimeout := h.Config.USBImageBootTimeout + h.Config.FirmwareScreen
	if msOptsContain(opts, WaitSoftwareSync) {
		totalTimeout += h.Config.SoftwareSyncUpdate
	}
	params := RunBypasser{BypasserMethod: ms.bypasser.BypassDevBootUSB, RepeatBypasser: true, WaitUntilDUTConnected: totalTimeout}
	return ms.RunBypasserUntilDUTConnected(ctx, params)
}

// EnableRecMode powers the DUT into the "rec" state, but does not wait to reconnect to the DUT.
// If booting into rec mode, usbMux should point to the DUT, so that the DUT can finish booting into recovery mode.
// Otherwise, usbMux should be off. This will prevent the DUT from transitioning to rec mode, so other operations can be performed (such as bypassing to dev mode).
func (ms *ModeSwitcher) EnableRecMode(ctx context.Context, recType servo.PowerStateValue, usbMux servo.USBMuxState) (retErr error) {
	h := ms.Helper
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "requiring servo")
	}

	// There are two usb type-a ports on servo_v4p1. By default, the bottom
	// one is set to 'dut_sees_usbkey'. If there's a usb device connected to it
	// with a valid ChromeOS image, the DUT can attempt booting into recovery
	// mode. As a safety measure, always disable the second port.
	if ok, err := h.Servo.HasControl(ctx, string(servo.SecondUSBKeyDirection)); err != nil {
		return errors.Wrapf(err, "failed to check control %s", servo.SecondUSBKeyDirection)
	} else if ok {
		// Set second_usbkey_direction to servo_sees_usbkey to disable
		// the port because it doesn't accept USBMuxOff.
		if err := h.Servo.SetString(ctx, servo.SecondUSBKeyDirection, string(servo.USBMuxHost)); err != nil {
			return errors.Wrapf(err, "failed to set servo control %s to %s", servo.SecondUSBKeyDirection, servo.USBMuxHost)
		}
	}

	// TODO(b/265193946): Remove this when dedede RO is fixed.
	if h.DUT.Connected(ctx) && h.Board == "dedede" {
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
	testing.ContextLogf(ctx, "Sleeping for %s (UsbDisableTime)", UsbDisableTime)
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
	// TODO(b/265193946): Remove this when dedede RO is fixed.
	if h.Board == "dedede" {
		// According to Stainless, some DUTs were stuck at G3 while
		// booting to recovery mode. Their ec logs reported that they
		// were experiencing thermal shutdown. Match for the relevant
		// texts and report in the returned error. If thermal shutdown
		// is caught, attempt a few more retries to boot dut to recovery.
		var ecStream string
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
			if err != nil {
				return errors.Wrap(err, "failed to enable capture EC UART")
			}
			defer func() {
				retErr = errors.Join(retErr, closeUART(ctx))
				// Sometimes capturing thermal shutdown fails because of
				// noise data, for example, "thermal SHUTDOWN". Setting
				// the 'chan' command before to hear from a particular channel
				// would not work because sending servo rec command later
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

			if err := h.Servo.SetPowerState(ctx, recType); err != nil {
				return errors.Wrapf(err, "setting power state to %s", recType)
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
	} else {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			return errors.Wrap(err, "failed to enable capture EC UART")
		}
		defer func() { retErr = errors.Join(retErr, closeUART(ctx)) }()
		if err := h.Servo.SetPowerState(ctx, recType); err != nil {
			return errors.Wrapf(err, "setting power state to %s", recType)
		}
	}

	if usbMux == servo.USBMuxDUT {
		testing.ContextLog(ctx, "Waiting for DUT to reach the firmware screen before USB enable")
		if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreenRecMode); err != nil {
			return errors.Wrap(err, "failed to get to firmware screen")
		}
		testing.ContextLog(ctx, "Set DFP mode")
		if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
			testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
			// If we have a battery, and the servo supports pd role, turn the power off to force DUT to DFP
			if h.Config.HasECCapability(ECBattery) {
				ok, err := h.Servo.HasControl(ctx, string(servo.PDRole))
				if err != nil {
					testing.ContextLogf(ctx, "Failed to check for %q control: %s", servo.PDRole, err)
				} else if ok {
					err = h.Servo.SetPDRole(ctx, servo.PDRoleSnk)
					if err != nil {
						testing.ContextLogf(ctx, "Failed to set pd role to %q: %s", servo.PDRoleSnk, err)
					}
				}
			}
		}
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
	waitForPowerOff := func(ctx context.Context) error {
		// If Chrome EC exists, check power state reaches G3,
		// otherwise wait for DUT unreachable.
		if h.Config.ChromeEC {
			return h.WaitForPowerStates(ctx, PowerStateInterval, powerOffTimeout, "G3")
		}
		offCtx, cancel := context.WithTimeout(ctx, powerOffTimeout)
		defer cancel()
		return ms.waitUnreachable(offCtx)
	}
	if h.DUT.Connected(ctx) {
		// Since the DUT will power off, deadline exceeded is expected here.
		if err := h.DUT.Conn().CommandContext(powerOffCtx, "poweroff").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return errors.Wrap(err, "DUT poweroff")
		}
		err := waitForPowerOff(ctx)
		if err == nil {
			return nil
		}
		if !errors.As(err, &context.DeadlineExceeded) {
			return errors.Wrap(err, "unexpected error in waiting for the dut to reach power-off")
		}
		testing.ContextLog(ctx, "Command poweroff failed to power off DUT: ", err)
	}
	// We didn't reach G3, or find DUT unreachable, so try having servo power off instead.
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
		return errors.Wrap(err, "set power_state:off")
	}
	return waitForPowerOff(ctx)
}

func (ms *ModeSwitcher) waitUnreachable(ctx context.Context) error {
	offCtx, cancel := context.WithTimeout(ctx, offTimeout)
	defer cancel()
	if err := ms.Helper.DUT.WaitUnreachable(offCtx); err != nil {
		return errors.Wrap(err, "waiting for DUT to be unreachable after powering off")
	}
	return nil
}

// RunBypasser contains information about how to run a bypasser
// for RunBypasserUntilDUTConnected.
type RunBypasser struct {
	BypasserMethod        func(ctx context.Context) error
	RepeatBypasser        bool
	WaitUntilDUTConnected time.Duration
}

// RunBypasserUntilDUTConnected either repeats a bypasser, or calls it only
// once to boot up the DUT, based on the specified controls in runBypasser.
func (ms *ModeSwitcher) RunBypasserUntilDUTConnected(ctx context.Context, params RunBypasser) error {
	h := ms.Helper
	waitConnectOpt := []WaitConnectOption{ResetEthernetDongle}
	switch params.RepeatBypasser {
	case true:
		connectTimeout := 3 * time.Second
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := params.BypasserMethod(ctx); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()
			connectTimeout += time.Second
			if err := h.WaitConnect(ctx, waitConnectOpt...); err != nil {
				// Only reset the Ethernet dongle if waiting for DUT to reconnect
				// fails during the first try.
				waitConnectOpt = nil
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: params.WaitUntilDUTConnected}); err != nil {
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
	default:
		testing.ContextLogf(ctx, "Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
		// GoBigSleepLint: Wait for firmware screen.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			return errors.Wrapf(err, "failed to sleep for %s (FirmwareScreen)", h.Config.FirmwareScreen)
		}
		if err := params.BypasserMethod(ctx); err != nil {
			return err
		}
		// Reconnect to the DUT.
		waitConnectOpt := []WaitConnectOption{ResetEthernetDongle}
		connectCtx, cancel := context.WithTimeout(ctx, params.WaitUntilDUTConnected)
		defer cancel()
		if err := h.WaitConnect(connectCtx, waitConnectOpt...); err != nil {
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
	}
	return nil
}

// RebootToFirmwareScreen requires that the DUT be connected initially, and runs
// the respective logic to reboot the DUT to the given firmware screen.
func (ms *ModeSwitcher) RebootToFirmwareScreen(ctx context.Context, fwScreen fwCommon.FwScreenType) error {
	h := ms.Helper
	switch fwScreen {
	case fwCommon.FwBrokenScreen:
		testing.ContextLog(ctx, "Setting crossystem recovery_request to 193")
		if !h.DUT.Connected(ctx) {
			return errors.New("requiring DUT to be connected initially")
		}
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "recovery_request=193").Run(); err != nil {
			return errors.Wrap(err, "failed to set crossystem recovery_request to 193")
		}
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
			return errors.Wrap(err, "failed to warm reset the DUT")
		}
	case fwCommon.FwDeveloperScreen:
		if !h.DUT.Connected(ctx) {
			return errors.New("requiring DUT to be connected initially")
		}
		curr, err := h.Reporter.CurrentBootMode(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get current boot mode")
		}
		if curr != fwCommon.BootModeDev {
			if err := ms.RebootToMode(ctx, fwCommon.BootModeDev, ExpectDevModeAfterReboot); err != nil {
				return errors.Wrap(err, "failed to reboot to developer mode")
			}
		}
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
			return errors.Wrap(err, "failed to warm reset the DUT")
		}
	case fwCommon.FwRecoveryScreen:
		if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
			return errors.Wrap(err, "failed to reboot to recovery screen")
		}
	}
	return nil
}

// PowerOn powers on the DUT by calling servo.PowerStateOn and BypassDevMode
// to boot up the device, regardless of its boot mode.
func (ms *ModeSwitcher) PowerOn(ctx context.Context) error {
	h := ms.Helper
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
		return errors.Wrap(err, "failed to set DUT's power")
	}
	// Repeat BypassDevMode for the machine to boot up
	// as fast as possible.
	params := RunBypasser{
		BypasserMethod:        ms.bypasser.BypassDevMode,
		RepeatBypasser:        true,
		WaitUntilDUTConnected: h.Config.DelayRebootToPing,
	}
	if err := ms.RunBypasserUntilDUTConnected(ctx, params); err != nil {
		return errors.Wrap(err, "failed to bypass dev mode")
	}
	return nil
}
