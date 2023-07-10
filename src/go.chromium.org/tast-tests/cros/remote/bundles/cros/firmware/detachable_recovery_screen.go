// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"regexp"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DetachableRecoveryScreen,
		Desc: "Confirm recovery screen appears after rec mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_detachable"},
		Fixture:      fixture.NormalMode,
		// To-do: Find a way to preserve firmware logs on Soraka and Nocturne.
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Detachable), hwdep.SkipOnModel("soraka", "nocturne")),
		Timeout:      25 * time.Minute,
	})
}

type testKeyPress int

const (
	powerBtnPress testKeyPress = iota
	volumeUpDownPress
	volumeUpPress
)

func DetachableRecoveryScreen(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	// Ensure no external devices connected for DUTs to boot from.
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to disable usbkey: ", err)
	}

	type readFwLog struct {
		enableFwLog func(ctx context.Context, h *firmware.Helper) error
		dataToScan  []string
		cleanups    func(ctx context.Context, h *firmware.Helper) error
	}
	var args readFwLog
	switch h.Board {
	case "kukui":
		args = readFwLog{
			enableFwLog: recToDevMode,
			dataToScan: []string{
				`display init`,
				`Logged recovery mode boot, reason: 0x02`,
				`Starting depthcharge on`,
				`cbgfx initialized: screen:width=\d*, height=\d*, offset=\d* canvas:width=\d*, height=\d*, offset=\d*`,
				`recovery_ui: waiting for a recovery image`,
				`vboot_draw_ui: screen=0x202`,
				`load_archive: loading vbgfx.bin`,
			},
			cleanups: rebootToNormal,
		}
	case "strongbad":
		args = readFwLog{
			enableFwLog: rebootAp,
			dataToScan: []string{
				`display init`,
				`Logged recovery mode boot, reason: 0x02`,
				`Starting depthcharge on`,
				`Recovery using external disk`,
				`display_ui: screen=0x200`,
				`cbgfx initialized: screen:width=\d*, height=\d*, offset=\d* canvas:width=\d*, height=\d*, offset=\d*`,
				`start display`,
			},
		}
	default:
		s.Fatal("Found unknown board name: ", h.Board)
	}

	for _, tc := range []struct {
		steps []func(ctx context.Context, h *firmware.Helper) error
	}{
		{
			[]func(ctx context.Context, h *firmware.Helper) error{
				func(ctx context.Context, h *firmware.Helper) error {
					return h.SetDUTPower(ctx, true)
				},
				powerOffDUT,
				func(ctx context.Context, h *firmware.Helper) error {
					return h.Servo.SetPowerState(ctx, servo.PowerStateRec)
				},
				func(ctx context.Context, h *firmware.Helper) error {
					// GoBigSleepLint: Sleeping for firmware screen.
					return testing.Sleep(ctx, h.Config.FirmwareScreen)
				},
				args.enableFwLog,
				func(ctx context.Context, h *firmware.Helper) error {
					return scanFwLog(ctx, h, args.dataToScan)
				},
				args.cleanups,
			},
		},
		{
			[]func(ctx context.Context, h *firmware.Helper) error{
				func(ctx context.Context, h *firmware.Helper) error {
					return h.SetDUTPower(ctx, false)
				},
				powerOffDUT,
				func(ctx context.Context, h *firmware.Helper) error {
					return h.Servo.SetPowerState(ctx, servo.PowerStateRec)
				},
				func(ctx context.Context, h *firmware.Helper) error {
					// GoBigSleepLint: Sleeping for firmware screen.
					return testing.Sleep(ctx, h.Config.FirmwareScreen)
				},
				args.enableFwLog,
				func(ctx context.Context, h *firmware.Helper) error {
					return scanFwLog(ctx, h, args.dataToScan)
				},
			},
		},
	} {
		for _, fnc := range tc.steps {
			if fnc != nil {
				if err := fnc(ctx, h); err != nil {
					s.Fatal("Unexpected error: ", err)
				}
			}
		}
	}

}

func powerOffDUT(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
		return err
	}
	return h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3")
}

// pressBtn presses a variety of buttons when requested.
func pressBtn(ctx context.Context, h *firmware.Helper, key testKeyPress) error {
	var err error
	switch key {
	case volumeUpDownPress:
		err = h.Servo.SetInt(ctx, servo.VolumeUpDownHold, 100)
	case volumeUpPress:
		err = h.Servo.SetInt(ctx, servo.VolumeUpHold, 100)
	case powerBtnPress:
		err = h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab)
	default:
		err = errors.New("no key detected")
	}
	if err != nil {
		return errors.Wrapf(err, "failed to press %v", key)
	}
	// GoBigSleepLint: Sleeping for keyboard delay.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for 1s")
	}
	return nil
}

// rebootAp reboots ap.
func rebootAp(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.RunECCommand(ctx, "apreset"); err != nil {
		return errors.Wrap(err, "failed to run apreset")
	}

	testing.ContextLog(ctx, "Waiting for DUT to reconnect")
	waitConnectOps := []firmware.WaitConnectOption{firmware.ResetEthernetDongle}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, waitConnectOps...); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	return nil
}

// scanFwLog verifies the messages in firmware log appear in sequence.
func scanFwLog(ctx context.Context, h *firmware.Helper, fwMessage []string) error {
	output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
	if err != nil {
		return errors.Wrap(err, "failed to read firmware log")
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		if len(fwMessage) > 0 {
			if match := regexp.MustCompile(fwMessage[0]).FindStringSubmatch(scanner.Text()); match != nil {
				testing.ContextLogf(ctx, "Found %q", match[0])
				fwMessage = fwMessage[1:]
			}
		}
	}
	if len(fwMessage) != 0 {
		return errors.Errorf("unable to find %q in firmware log", fwMessage[0])
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to scan firmware log")
	}
	return nil
}

func recToDevMode(ctx context.Context, h *firmware.Helper) error {
	for _, key := range []testKeyPress{volumeUpDownPress, volumeUpPress, powerBtnPress} {
		if err := pressBtn(ctx, h, key); err != nil {
			return errors.Wrap(err, "failed to enable developer mode")
		}
	}
	waitConnectOps := []firmware.WaitConnectOption{firmware.ResetEthernetDongle}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 4*time.Minute)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, waitConnectOps...); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	return nil
}

func rebootToNormal(ctx context.Context, h *firmware.Helper) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	return ms.RebootToMode(ctx, fwCommon.BootModeNormal)
}
