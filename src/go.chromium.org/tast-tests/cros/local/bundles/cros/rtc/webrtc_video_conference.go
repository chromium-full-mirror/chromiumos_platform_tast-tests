// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rtc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/rtc/webrtc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebRTCVideoConference,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure audio, camera, graphics and video stack performance in a video conference using WebRTC",
		Contacts: []string{
			"chromeos-rtc@google.com",
		},
		BugComponent: "b:1401297", // ChromeOS > Platform > Technologies > RTC
		// TODO(hiroh): Look for a suite
		SoftwareDeps: []string{"chrome", caps.BuiltinCamera},
		Data:         webrtc.TestFiles(),
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Val:     webrtc.VCTestParams{},
				Fixture: "chromeRTCPerf",
			},
		},
	})
}

func WebRTCVideoConference(ctx context.Context, s *testing.State) {
	params := s.Param().(webrtc.VCTestParams)

	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Setup the device for power testing. This includes setting the battery to discharge mode in
	// order to be able to collect system power usage numbers.
	//
	// Note that we do this before possibly launching LaCrOS so that setup.PowerTest() doesn't
	// get confused with multiple windows.
	cleanup, err := setup.PowerTest(ctx, tconn, setup.PowerTestOptions{
		Wifi:               setup.DisableWifiInterfaces,
		NightLight:         setup.DisableNightLight,
		DarkTheme:          setup.EnableLightTheme,
		KeyboardBrightness: setup.SetKbBrightnessToZero,
	}, setup.NewBatteryDischarge(true /*discharge*/, false /*ignoreErr*/, setup.DefaultDischargeThreshold))
	if err != nil {
		// This is not really an error: sometimes powerd is down or lost and setting
		// up the power test fails. Just don't provide any metric.
		s.Log("Skipping test, something went wrong during test set up: ", err)
		return
	}
	defer cleanup(ctx)

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), browser.TypeAsh)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	if err := webrtc.RunVideoConference(ctx, cs, cr, s, params); err != nil {
		s.Fatal("RunVideoConference failed: ", err)
	}
}
