// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// Note: Pendrive connected to the servo should be having the recovery OS flashed.

func init() {
	testing.AddTest(&testing.Test{
		Func:         RecModeTime,
		Desc:         "Test to ensure boot to recovery mode time is consistently less than 5 seconds",
		SoftwareDeps: []string{"crossystem", "flashrom"},
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		Fixture:      fixture.DevModeGBB,
		Timeout:      10 * time.Minute,
	})
}

func RecModeTime(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	h := s.FixtValue().(*fixture.Value).Helper
	dut := s.DUT()
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	s.Log("Stopping power supply")
	if err := h.SetDUTPower(ctx, false); err != nil {
		s.Fatal("Failed to remove charger: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if attached, err := h.Servo.GetChargerAttached(ctx); err != nil {
			return errors.Wrap(err, "error checking whether charger is attached")
		} else if attached {
			return errors.New("charger is still attached - use Servo V4 Type-C or supply RPM vars")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check if charger is disconnected via Servo: ", err)
	}

	defer func(ctx context.Context) {
		s.Log("Performing cleanup")
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to attach charger: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if attached, err := h.Servo.GetChargerAttached(ctx); err != nil {
				return errors.Wrap(err, "error checking whether charger is attached")
			} else if !attached {
				return errors.New("charger is not attached at cleanup - use Servo V4 Type-C or supply RPM vars")
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Fatal("Failed to check if charger is connected via Servo: ", err)
		}
	}(cleanupCtx)

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create new boot mode switcher: ", err)
	}
	s.Log("Rebooting into recovery mode")
	if err := ms.RebootToMode(ctx, fwCommon.BootModeRecovery); err != nil {
		s.Fatal("Failed to reboot into recovery mode: ", err)
	}

	s.Log("Reconnecting to DUT")
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking that DUT has booted from removable device")
	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Fatal("Failed to determine boot device type: ", err)
	}
	if !bootedFromRemovableDevice {
		s.Fatalf("DUT did not boot from the bootable device: got %v, want true", bootedFromRemovableDevice)
	}

	fwBootTime, err := firmwareTimestampBootTime(ctx, dut)
	if err != nil {
		s.Fatal("Failed to read firmware boot time: ", err)
	}

	const expectedBootTimeSec float64 = 5
	if fwBootTime > expectedBootTimeSec {
		s.Fatalf("Failed to verify firmware boot time, got: %.2f, want: <=%.2f sec", fwBootTime, expectedBootTimeSec)
	}

}

// firmwareTimestampBootTime reads firmware startup time from /tmp/firmware-boot-time
// and returns the value in seconds.
func firmwareTimestampBootTime(ctx context.Context, dut *dut.DUT) (float64, error) {
	var b []byte
	var err error
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		b, err = linuxssh.ReadFile(ctx, dut.Conn(), "/tmp/firmware-boot-time")
		if err != nil {
			return errors.Wrap(err, "failed to read firmware-boot-time file")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
		return 0, errors.Wrap(err, "failed to get firmware times stamp boot time")
	}
	l := strings.Split(string(b), "\n")[0]
	return strconv.ParseFloat(l, 64)
}
