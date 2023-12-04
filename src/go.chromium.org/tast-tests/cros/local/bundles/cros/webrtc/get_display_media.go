// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/getdisplaymedia"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type getDisplayMediaTestParams struct {
	surfaceType string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetDisplayMedia,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that WebRTC getDisplayMedia() (screen, window, tab capture) works",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org", // Test author.
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         getdisplaymedia.DataFiles(),
		Attr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
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
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:              "tab_lacros",
			Val:               getDisplayMediaTestParams{surfaceType: "browser", browserType: browser.TypeLacros},
			Fixture:           "chromeTabCaptureLacros",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// GetDisplayMedia verifies that the homonymous API works as expected.
func GetDisplayMedia(ctx context.Context, s *testing.State) {
	params := s.Param().(getDisplayMediaTestParams)

	_, l, cs, err := lacros.Setup(ctx, s.FixtValue(), params.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	if err := getdisplaymedia.RunGetDisplayMedia(ctx, s, cs, params.surfaceType); err != nil {
		s.Fatal("TestPlay failed: ", err)
	}
}
