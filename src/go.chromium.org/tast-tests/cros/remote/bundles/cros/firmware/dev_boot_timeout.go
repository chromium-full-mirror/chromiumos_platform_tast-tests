// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type cbmemData struct {
	logs       string
	timeStamps string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DevBootTimeout,
		Desc: "Validate developer firmware screen timeout period",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.DevMode,
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func DevBootTimeout(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	const (
		timeoutBootLogs           = "timeoutBootLogs.log"
		timeoutBootTimestamps     = "timeoutBootTimestamps.log"
		quickBypassBootLogs       = "quickBypassBootLogs.log"
		quickBypassBootTimestamps = "quickBypassBootTimestamps.log"
	)

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	var timeoutBootTime, quickBypassBootTime time.Duration
	s.Log("Running timeout boot in developer mode")
	if err := rebootInDevMode(ctx, h, false); err != nil {
		s.Fatal("Failed to reboot in dev mode: ", err)
	}
	timeoutBootTime, err := recordFwBootTime(ctx, h)
	if err != nil {
		s.Fatal("Failed to find boot time for timeout boot: ", err)
	}
	timeoutBootCbmem, err := getCBMEMData(ctx, h)
	if err != nil {
		s.Fatal("Failed to get cbmem data for timeout boot: ", err)
	}

	s.Log("Running quick bypass boot in developer mode")
	if err := rebootInDevMode(ctx, h, true); err != nil {
		s.Fatal("Failed to reboot in dev mode: ", err)
	}
	quickBypassBootTime, err = recordFwBootTime(ctx, h)
	if err != nil {
		s.Fatal("Failed to find boot time for quick bypass boot: ", err)
	}
	quickBypassBootCbmem, err := getCBMEMData(ctx, h)
	if err != nil {
		s.Fatal("Failed to get cbmem data for quick bypass boot: ", err)
	}

	gotTimeout := timeoutBootTime - quickBypassBootTime
	s.Log("Found difference between measured and expected timeout: ", gotTimeout-firmware.DevScreenTimeout)
	// Set 2 seconds as the acceptable deviation when comparing the boot times
	// between timeout boot and quick bypass boot.
	timeoutMargin := 2 * time.Second
	if (gotTimeout - firmware.DevScreenTimeout).Abs() > timeoutMargin {
		// Store cbmem logs and timestamps for debugging purposes.
		saveCbmemData := [][]string{
			{timeoutBootCbmem.logs, timeoutBootLogs},
			{timeoutBootCbmem.timeStamps, timeoutBootTimestamps},
			{quickBypassBootCbmem.logs, quickBypassBootLogs},
			{quickBypassBootCbmem.timeStamps, quickBypassBootTimestamps},
		}
		for _, data := range saveCbmemData {
			saveLogPath := filepath.Join(s.OutDir(), data[1])
			if err := os.WriteFile(saveLogPath, []byte(data[0]), 0666); err != nil {
				s.Logf("Failed to save %s: %v", saveLogPath, err)
			}
		}
		s.Fatalf("The developer firmware timeout is expected to be 30 +/- %v, but got %v", timeoutMargin, gotTimeout)
	}
}

// recordFwBootTime checks the firmware-boot-time file and returns the firmware
// boot time.
func recordFwBootTime(ctx context.Context, h *firmware.Helper) (time.Duration, error) {
	// Time for the concerned files to be added to the filesystem.
	runShellReadyTimeMargin := 10 * time.Second
	var fwbootTime float64
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := h.Reporter.CatFile(ctx, "/tmp/firmware-boot-time")
		if err != nil {
			return errors.Wrap(err, "failed to get firmware boot time")
		}
		fwbootTime, err = strconv.ParseFloat(strings.Split(string(out), "\n")[0], 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse firmware boot time")
		}
		return nil
	}, &testing.PollOptions{Timeout: runShellReadyTimeMargin, Interval: 1 * time.Second}); err != nil {
		return -1, err
	}
	testing.ContextLog(ctx, "Found firmware boot time: ", time.Duration(fwbootTime*float64(time.Second)))
	return time.Duration(fwbootTime * float64(time.Second)), nil
}

// rebootInDevMode reboots the DUT in dev mode either by waiting for developer
// screen timeout, or by pressing Ctrl-D to bypass the developer screen.
func rebootInDevMode(ctx context.Context, h *firmware.Helper, bypassDevScreen bool) error {
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		return errors.Wrap(err, "failed to warm reset the DUT")
	}
	if bypassDevScreen {
		// Repeat Ctrl-D presses from power-on until the DUT is reconnected.
		testing.ContextLog(ctx, "Starting Ctrl-D presses until the DUT is reconnected")
		if err := ctrlDUntilDUTConnected(ctx, h); err != nil {
			return err
		}
	} else {
		testing.ContextLog(ctx, "Waiting for the DUT to reconnect")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing+firmware.DevScreenTimeout)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
	}
	return nil
}

func getCBMEMData(ctx context.Context, h *firmware.Helper) (cbmemData, error) {
	var data cbmemData
	cbmem, err := h.Reporter.GetCBMEMLogs(ctx)
	if err != nil {
		return data, err
	}
	cbmemTs, err := h.Reporter.GetCBMEMTimestamps(ctx)
	if err != nil {
		return data, err
	}
	data.logs = cbmem
	data.timeStamps = cbmemTs
	return data, nil
}

func ctrlDUntilDUTConnected(ctx context.Context, h *firmware.Helper) error {
	status := make(chan error, 1)
	go func() {
		defer func() {
			close(status)
		}()
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing+firmware.DevScreenTimeout)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			status <- errors.Wrap(err, "failed to reconnect to DUT")
		}
		status <- nil

	}()
	for {
		select {
		case err := <-status:
			return err
		default:
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
				return errors.Wrap(err, "failed to press ctrl-d")
			}
		}
	}
}
