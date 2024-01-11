// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/playback"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlaybackStress,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Playback in Chrome browser with system goes to suspend/resume cycle",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"pwang@chromium.org",
			"mcasas@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			"video.html",
			"playback.js",
		},
		Params: []testing.Param{
			{
				Name: "h264_720p_30fps",
				Val: playback.Config{
					FileName:      "perf/h264/720p_30fps_300frames.h264.mp4",
					BrowserType:   browser.TypeAsh,
					SuspendResume: true,
				},
				ExtraSoftwareDeps: []string{"proprietary_codecs", "autotest-capability:hw_dec_h264_1080_30"},
				ExtraData:         []string{"perf/h264/720p_30fps_300frames.h264.mp4"},
				Fixture:           "chromeVideo",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "h264_720p_30fps_lacros",
				Val: playback.Config{
					FileName:      "perf/h264/720p_30fps_300frames.h264.mp4",
					BrowserType:   browser.TypeLacros,
					SuspendResume: true,
				},
				ExtraSoftwareDeps: []string{"proprietary_codecs", "autotest-capability:hw_dec_h264_1080_30"},
				ExtraData:         []string{"perf/h264/720p_30fps_300frames.h264.mp4"},
				Fixture:           "chromeVideoLacros",
				Timeout:           5 * time.Minute,
			},
		},
	})
}

// PlaybackStress plays a video in the Chrome browser.
func PlaybackStress(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(playback.Config)
	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	playback.RunTest(ctx, s, tconn, testOpt)
}
