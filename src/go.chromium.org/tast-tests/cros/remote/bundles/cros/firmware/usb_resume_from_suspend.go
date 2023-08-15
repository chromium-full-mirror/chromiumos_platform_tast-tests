// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         USBResumeFromSuspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify if all usb ports come back from suspend",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService"},
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

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	s.Log("Starting a new Chrome")
	chromeService := pb.NewChromeServiceClient(h.RPCClient.Conn)
	if _, err := chromeService.New(ctx, &pb.NewRequest{
		LoginMode: pb.LoginMode_LOGIN_MODE_GUEST_LOGIN,
	}); err != nil {
		s.Fatal("Failed to create new Chrome at login: ", err)
	}
	defer chromeService.Close(ctx, &empty.Empty{})

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
		var usbEvents []string
		for _, event := range []string{"usb_dev_suspend", "usb_dev_resume"} {
			reCallAction := `(` + fmt.Sprintf(`usb%d:.*calling\s*%s`, idx, event) +
				`|` + fmt.Sprintf(`calling\s*usb%d.*%s`, idx, event) + `)`
			reActionSuccess := `(` + fmt.Sprintf(`usb%d:.*%s.*returned\s*0`, idx, event) +
				`|` + fmt.Sprintf(`call\s*usb%d.*returned\s*0`, idx) + `)`
			usbEvents = append(usbEvents, reCallAction, reActionSuccess)
		}
		if err := h.ScanWithExpectedSequenceInSource(ctx, out, usbEvents); err != nil {
			s.Fatalf("While checking for usb bus %d: %v", idx, err)
		}
	}
}
