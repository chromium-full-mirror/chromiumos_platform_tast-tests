// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/getdisplaymedia"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type getDisplayMediaTestParams struct {
	surfaceType string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: GetDisplayMediaPerf,
		Desc: "Verifies that WebRTC getDisplayMedia() (screen, window, tab capture) works and collects performance data",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"hiroh@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("kodama")), // b/324981613
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
			Val:               getDisplayMediaTestParams{surfaceType: "monitor"},
			Fixture:           "chromeScreenCapture",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.NoExternalDisplay()),
		}, {
			Name:    "window",
			Val:     getDisplayMediaTestParams{surfaceType: "window"},
			Fixture: "chromeWindowCapture",
		}, {
			Name:    "tab",
			Val:     getDisplayMediaTestParams{surfaceType: "browser"},
			Fixture: "chromeTabCapture",
		}},
	})
}

// GetDisplayMediaPerf verifies that the homonymous API works as expected and collects performance data.
func GetDisplayMediaPerf(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	params := s.Param().(getDisplayMediaTestParams)

	tconn, err := cr.TestAPIConn(ctx)
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
		s.Fatal("Failed in power setup: ", err)
	}
	defer cleanup(cleanupCtx)

	if err := getdisplaymedia.RunGetDisplayMediaPerf(ctx, s.DataFileSystem(), cr, tconn, params.surfaceType); err != nil {
		s.Fatal("TestPlay failed: ", err)
	}
}
