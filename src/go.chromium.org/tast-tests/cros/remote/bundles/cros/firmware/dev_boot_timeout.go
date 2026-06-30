// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DevBootTimeout,
		Desc: "Validate developer firmware screen timeout period",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT

		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		Fixture:      fixture.DevMode,
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
	})
}

func DevBootTimeout(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			s.Error("Failed to capture EC UART: ", err)
		}
		defer func() {
			if err := closeUART(ctx); err != nil {
				s.Error("Failed to cancel capture EC UART: ", err)
			}
		}()
	}

	s.Log("Running timeout boot in developer mode")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset the DUT: ", err)
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable after sending a warm reset: ", err)
	}

	if h.HasAPFwState {
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.DeveloperMode); err != nil {
			s.Fatal("Failed to detect firmware screen: ", err)
		}
	} else {
		s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
		// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
		}
	}

	testing.ContextLog(ctx, "Waiting for the DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing+firmware.DevScreenTimeout)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Getting CBMEM timestamp")
	cbmemTs, err := h.Reporter.GetCBMEMTimestamps(ctx)
	if err != nil {
		s.Fatal("Failed to get CBMEM timestamps: ", err)
	}
	// finished storage device initialization is the first timestamp begin from booting.
	re := regexp.MustCompile(`finished storage device initialization.*\(([\d,]*)\)`)
	regResult := re.FindStringSubmatch(cbmemTs)
	if regResult == nil {
		s.Fatalf("Failed to match %q from output: %s", re, cbmemTs)
	}
	timeoutTs, err := strconv.Atoi(strings.Replace(regResult[1], ",", "", -1))
	if err != nil {
		s.Fatal("Failed to parse result to int: ", err)
	}
	gotTimeout := time.Duration(timeoutTs) * time.Microsecond
	testing.ContextLog(ctx, "Found timeout timestamps: ", gotTimeout)

	timeoutMargin := 2 * time.Second
	if (gotTimeout - firmware.DevScreenTimeout).Abs() > timeoutMargin {
		// Store cbmem logs and timestamps for debugging purposes.
		cbmem, err := h.Reporter.GetCBMEMLogs(ctx)
		if err != nil {
			s.Log("Failed to get CBMEM logs: ", err)
		} else {
			saveCbmemData := [][]string{
				{"bootCbmem.logs", cbmem},
				{"bootTimestamps.log", cbmemTs},
			}
			for _, data := range saveCbmemData {
				saveLogPath := filepath.Join(s.OutDir(), data[0])
				if err := os.WriteFile(saveLogPath, []byte(data[1]), 0666); err != nil {
					s.Logf("Failed to save %s: %v", saveLogPath, err)
				}
			}
		}
		s.Fatalf("The developer firmware timeout is expected to be 30 +/- %v, but got %v", timeoutMargin, gotTimeout)
	}
}
