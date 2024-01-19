// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/getdisplaymedia"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type getDisplayMediaTestParams struct {
	surfaceType string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetDisplayMediaPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that WebRTC getDisplayMedia() (screen, window, tab capture) works and collects performance data",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org", // Test author.
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         getdisplaymedia.DataFiles(),
		Attr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
		// Set the larger timeout than the default one (i.e. 2 minutes)
		// because waiting for cpu cool down may take a while.
		Timeout: 5 * time.Minute,
		// See https://w3c.github.io/mediacapture-screen-share/#displaycapturesurfacetype
		// for where the case names come from.
		// TODO(crbug.com/1063449): add other cases when the adequate precondition is ready.
		Params: []testing.Param{{
			Name:              "monitor",
			Val:               getDisplayMediaTestParams{surfaceType: "monitor", browserType: browser.TypeAsh},
			Fixture:           "chromeScreenCapture",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}, {
			Name:    "window",
			Val:     getDisplayMediaTestParams{surfaceType: "window", browserType: browser.TypeAsh},
			Fixture: "chromeWindowCapture",
		}, {
			Name:    "tab",
			Val:     getDisplayMediaTestParams{surfaceType: "browser", browserType: browser.TypeAsh},
			Fixture: "chromeTabCapture",
		}, {
			Name:              "monitor_zero_copy",
			Val:               getDisplayMediaTestParams{surfaceType: "monitor", browserType: browser.TypeAsh},
			Fixture:           "chromeZeroCopyScreenCapture",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}, {
			Name:    "window_zero_copy",
			Val:     getDisplayMediaTestParams{surfaceType: "window", browserType: browser.TypeAsh},
			Fixture: "chromeZeroCopyWindowCapture",
		}, {
			Name:    "tab_zero_copy",
			Val:     getDisplayMediaTestParams{surfaceType: "browser", browserType: browser.TypeAsh},
			Fixture: "chromeZeroCopyTabCapture",
		}, {
			Name:              "monitor_lacros",
			Val:               getDisplayMediaTestParams{surfaceType: "monitor", browserType: browser.TypeLacros},
			Fixture:           "chromeScreenCaptureLacros",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:              "window_lacros",
			Val:               getDisplayMediaTestParams{surfaceType: "window", browserType: browser.TypeLacros},
			Fixture:           "chromeWindowCaptureLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:              "tab_lacros",
			Val:               getDisplayMediaTestParams{surfaceType: "browser", browserType: browser.TypeLacros},
			Fixture:           "chromeTabCaptureLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// GetDisplayMediaPerf verifies that the homonymous API works as expected and collects performance data.
func GetDisplayMediaPerf(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := setup.PowerTest(ctx, tconn, setup.PowerTestOptions{
		Wifi:               setup.DisableWifiInterfaces,
		NightLight:         setup.DisableNightLight,
		DarkTheme:          setup.EnableLightTheme,
		KeyboardBrightness: setup.SetKbBrightnessToZero,
	}, setup.NewBatteryDischarge(true /*discharge*/, true /*ignoreErr*/, setup.DefaultDischargeThreshold))
	if err != nil {
		// This is not really an error: sometimes powerd is down or lost and setting
		// up the power test fails. Just don't provide any metric.
		s.Log("Skipping test, something went wrong during test set up: ", err)
		return
	}
	defer cleanup(cleanupCtx)

	params := s.Param().(getDisplayMediaTestParams)
	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), params.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	var br *browser.Browser
	switch params.browserType {
	case browser.TypeAsh:
		br = cr.Browser()
	case browser.TypeLacros:
		br = l.Browser()
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API: ", err)
	}

	if err := getdisplaymedia.RunGetDisplayMediaPerf(ctx, s.DataFileSystem(), cs, tconn, bTconn, params.surfaceType); err != nil {
		s.Fatal("TestPlay failed: ", err)
	}
}
