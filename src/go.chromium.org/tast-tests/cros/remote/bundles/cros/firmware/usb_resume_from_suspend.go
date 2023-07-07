// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: USBResumeFromSuspend,
		Desc: "Verify if all usb ports come back from suspend",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.NormalMode,
		Timeout:      8 * time.Minute,
	})
}

func USBResumeFromSuspend(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	logPath := "/var/log/messages"
	s.Logf("Cleaning %s", logPath)
	if err := h.DUT.Conn().CommandContext(ctx, "truncate", "--size=0", logPath).Run(); err != nil {
		s.Fatal("Failed to remove kernel message file: ", err)
	}

	// Get the number of usb buses.
	output, err := h.DUT.Conn().CommandContext(ctx, "lsusb", "-t").Output()
	if err != nil {
		s.Fatal("Failed to run lsusb command: ", err)
	}
	r := regexp.MustCompile("Class=root_hub")
	match := r.FindAllStringSubmatch(string(output), -1)
	usbBusNum := len(match)

	s.Log("Suspending DUT")
	if err := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend").Start(); err != nil {
		s.Fatal("Failed to suspend DUT: ", err)
	}
	waitUnreachableCtx, cancelUnreachable := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelUnreachable()
	if err := h.DUT.WaitUnreachable(waitUnreachableCtx); err != nil {
		s.Fatal("Failed to wait DUT unreachable: ", err)
	}
	s.Log("Checking for S0ix or S3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0ix", "S3"); err != nil {
		s.Fatal("Failed to get power state at S0ix or S3, but found dut disconnected: ", err)
	}

	wakeupKey := servo.Enter
	if h.Config.ModeSwitcherType == firmware.TabletDetachableSwitcher {
		wakeupKey = servo.PowerKey
	}
	s.Logf("Waking DUT from suspend by %s", wakeupKey)
	if err := h.Servo.KeypressWithDuration(ctx, wakeupKey, servo.DurPress); err != nil {
		s.Fatalf("Failed to wake from suspend by %s: %v", wakeupKey, err)
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	out, err := h.Reporter.CatFile(ctx, logPath)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", logPath, err)
	}
	for idx := 1; idx <= usbBusNum; idx++ {
		s.Logf("Verifying resume from suspend for usb bus %d", idx)
		if err := checkUSBSuspendResume(ctx, h, idx, out); err != nil {
			s.Fatalf("While checking for usb bus %d: %v", idx, err)
		}
	}
}

// checkUSBSuspendResume checks the kernel message file, and scans for the associated usb
// events for a specified port in the following order: usb_dev_suspend, and usb_dev_resume.
func checkUSBSuspendResume(ctx context.Context, h *firmware.Helper, usbBusNum int, log string) error {
	var usbEvents []string
	for _, event := range []string{"usb_dev_suspend", "usb_dev_resume"} {
		reCallAction := `(` + fmt.Sprintf(`usb%d:.*calling\s*%s`, usbBusNum, event) +
			`|` + fmt.Sprintf(`calling\s*usb%d.*%s`, usbBusNum, event) + `)`
		reActionSuccess := `(` + fmt.Sprintf(`usb%d:.*%s.*returned\s*0`, usbBusNum, event) +
			`|` + fmt.Sprintf(`call\s*usb%d.*returned\s*0`, usbBusNum) + `)`
		usbEvents = append(usbEvents, reCallAction, reActionSuccess)
	}
	// Scan for the kernel message file, and expect to find usb_dev_suspend
	// first, before reaching usb_dev_resume. Pop out the event found from usbEvents.
	scanner := bufio.NewScanner(strings.NewReader(log))
	for scanner.Scan() {
		if len(usbEvents) > 0 {
			if match := regexp.MustCompile(usbEvents[0]).FindStringSubmatch(scanner.Text()); match != nil {
				testing.ContextLogf(ctx, "Found usb event: %s", match[0])
				usbEvents = usbEvents[1:]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to scan kernal message file")
	}
	// Verify all usb events were found. Namely, calling usb_dev_suspend and usb_dev_resume
	// were both successful.
	if len(usbEvents) != 0 {
		return errors.Errorf("got %d usb events not found, check test log for details", len(usbEvents))
	}
	return nil
}
