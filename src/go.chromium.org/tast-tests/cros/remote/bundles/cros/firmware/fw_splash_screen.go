// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FwSplashScreen,
		Desc:         "Test to validate the firmware splash screen feature",
		Contacts:     []string{"digehlot@google.com", "chromeos-firmware@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		HardwareDeps: hwdep.D(hwdep.FirmwareSplashScreen()),
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		Timeout:      5 * time.Minute,
	})
}

func FwSplashScreen(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	r := h.Reporter
	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	s.Log("Reboot and wait for DUT to reconnect")
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	events, err := r.EventlogList(ctx)
	if err != nil {
		s.Fatal(err, "failed to read event log")
	}
	for _, event := range events {
		if strings.Contains(event.Message, "Firmware Splash Screen") {
			s.Log(event.Message)
			split := strings.SplitN(event.Message, " | ", 2)
			if split[1] == "Disabled" {
				s.Error("Splash screen feature enabled, but elog shows disabled")
			} else if split[1] == "Enabled" {
				s.Log("Splash screen feature verified")
				return
			}
			s.Error("Unexpected splash screen feature log: ", event.Message)
		}
	}
	s.Error("Splash Screen Feature is not enabled in coreboot hence failed")
}
