// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	videoEncodeURL      = "https://storage.googleapis.com/chromiumos-test-assets-public/power_VideoEncode/power_VideoEncode.html"
	permBubbleName      = "storage.googleapis.com wants to"
	cameraDataNameRegex = "codec: video/webm.*"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoEncode,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collects power metrics while encoding video files",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "jingmuli@google.com"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name:    "vp9_hd_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "hd", Framerate: 24},
		}, {
			Name:    "vp9_vga_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "vga", Framerate: 24},
		}, {
			Name:    "vp9_qvga_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "qvga", Framerate: 24},
		}, {
			Name:    "vp8_hd_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "hd", Framerate: 24},
		}, {
			Name:    "vp8_vga_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "vga", Framerate: 24},
		}, {
			Name:    "vp8_qvga_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "qvga", Framerate: 24},
		}, {
			Name:    "h264_hd_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "h264", Resolution: "hd", Framerate: 24},
		}, {
			Name:    "h264_vga_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "h264", Resolution: "vga", Framerate: 24},
		}, {
			Name:    "vp9_hvga_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "hvga", Framerate: 24},
		}, {
			Name:    "vp9_qhvga_20fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "qhvga", Framerate: 20},
		}, {
			Name:    "vp8_hvga_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "hvga", Framerate: 24},
		}, {
			Name:    "vp8_qhvga_15fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "qhvga", Framerate: 15},
		}, {
			Name:    "vp9_fhd_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "fhd", Framerate: 24},
		}, {
			Name:    "vp8_fhd_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "fhd", Framerate: 24},
		}, {
			Name:    "h264_fhd_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "h264", Resolution: "fhd", Framerate: 24},
		}, {
			Name:    "av1_hvga_24fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "av1", Resolution: "hvga", Framerate: 24},
		}, {
			Name:    "av1_qhvga_15fps_ash",
			Fixture: "powerAsh",
			Val:     power.VideoEncodeFormatParams{Codec: "av1", Resolution: "qhvga", Framerate: 15},
		}, {
			Name:              "vp9_hd_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "hd", Framerate: 24},
		}, {
			Name:              "vp9_vga_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "vga", Framerate: 24},
		}, {
			Name:              "vp9_qvga_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "qvga", Framerate: 24},
		}, {
			Name:              "vp8_hd_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "hd", Framerate: 24},
		}, {
			Name:              "vp8_vga_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "vga", Framerate: 24},
		}, {
			Name:              "vp8_qvga_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "qvga", Framerate: 24},
		}, {
			Name:              "h264_hd_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "h264", Resolution: "hd", Framerate: 24},
		}, {
			Name:              "h264_vga_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "h264", Resolution: "vga", Framerate: 24},
		}, {
			Name:              "vp9_hvga_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "hvga", Framerate: 24},
		}, {
			Name:              "vp9_qhvga_20fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "qhvga", Framerate: 20},
		}, {
			Name:              "vp8_hvga_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "hvga", Framerate: 24},
		}, {
			Name:              "vp8_qhvga_15fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "qhvga", Framerate: 15},
		}, {
			Name:              "vp9_fhd_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp9", Resolution: "fhd", Framerate: 24},
		}, {
			Name:              "vp8_fhd_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "vp8", Resolution: "fhd", Framerate: 24},
		}, {
			Name:              "h264_fhd_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "h264", Resolution: "fhd", Framerate: 24},
		}, {
			Name:              "av1_hvga_24fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "av1", Resolution: "hvga", Framerate: 24},
		}, {
			Name:              "av1_qhvga_15fps_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.VideoEncodeFormatParams{Codec: "av1", Resolution: "qhvga", Framerate: 15},
		}},
	})
}

func VideoEncode(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	var videoEncodeFormatParams = s.Param().(power.VideoEncodeFormatParams)
	interval := videoEncodeFormatParams.VideoEncodeTimeParams.Interval
	total := videoEncodeFormatParams.VideoEncodeTimeParams.Total

	// Use default value for timeParam if not set.
	defaultTimeParams := power.TimeParams{Interval: 5 * time.Second, Total: 3 * time.Minute}
	if interval == time.Duration(0) {
		interval = defaultTimeParams.Interval
	}
	if total == time.Duration(0) {
		total = defaultTimeParams.Total
	}

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if err := conn.Navigate(ctx, videoEncodeURL); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}

	// Check whether we need camera permission by find one of these
	// * Allow button for camera permission
	// * cameraData node which indicate that the camera is already recorded
	bubble := nodewith.NameStartingWith(permBubbleName).First()
	allow := nodewith.Name("Allow").Role(role.Button).Ancestor(bubble)
	cameraData := nodewith.NameRegex(regexp.MustCompile(cameraDataNameRegex)).First()

	foundNode, err := ui.FindAnyExists(ctx, allow, cameraData)
	if err != nil {
		s.Fatal("Failed to find the permission bubble: ", err)
	}

	// Click the allow button if found, and wait until cameraData node exists.
	pc := pointer.NewMouse(tconn)
	if foundNode == allow {
		if err := pc.Click(allow)(ctx); err != nil {
			s.Fatal("Failed to click permission bubble: ", err)
		}
		if err := ui.WaitUntilAnyExists(cameraData)(ctx); err != nil {
			s.Fatal("Failed to find the fps data node: ", err)
		}
	}

	js := `changeFormat("` +
		videoEncodeFormatParams.Codec + `", "` +
		videoEncodeFormatParams.Resolution + `", ` +
		strconv.FormatInt(int64(videoEncodeFormatParams.Framerate), 10) +
		`)`
	if err := conn.Eval(ctx, js, nil); err != nil {
		s.Error("Failed to encode videos: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
	// GoBigSleepLint: Sleep a period of time to generate video encoding data.
	if err := testing.Sleep(ctx, total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
