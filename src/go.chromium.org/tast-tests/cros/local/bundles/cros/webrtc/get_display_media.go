// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/getdisplaymedia"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetDisplayMedia,
		LacrosStatus: testing.LacrosVariantUnknown,
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
			Val:               "monitor",
			Fixture:           "chromeScreenCapture",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}, {
			Name:    "window",
			Val:     "window",
			Fixture: "chromeWindowCapture",
		}, {
			Name:    "tab",
			Val:     "browser",
			Fixture: "chromeTabCapture",
		}, {
			Name:              "monitor_zero_copy",
			Val:               "monitor",
			Fixture:           "chromeZeroCopyScreenCapture",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}, {
			Name:    "window_zero_copy",
			Val:     "window",
			Fixture: "chromeZeroCopyWindowCapture",
		}, {
			Name:    "tab_zero_copy",
			Val:     "browser",
			Fixture: "chromeZeroCopyTabCapture",
		}},
	})
}

// GetDisplayMedia verifies that the homonymous API works as expected.
func GetDisplayMedia(ctx context.Context, s *testing.State) {
	if err := getdisplaymedia.RunGetDisplayMedia(ctx, s, s.FixtValue().(chrome.HasChrome).Chrome(), s.Param().(string)); err != nil {
		s.Fatal("TestPlay failed: ", err)
	}
}
