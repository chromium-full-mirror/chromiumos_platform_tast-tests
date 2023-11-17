// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type videoPlaybackTestParam struct {
	VideoName  string
	TimeParams power.TimeParams
}

var videoPlaybackDefaultTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 6 * time.Minute}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoPlayback,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics playing offline video",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      time.Hour + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:      "h264_720_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "h264_720_30fps"},
			ExtraData: []string{"video_playback/h264_720_30fps.mp4"},
		}, {
			Name:      "h264_720_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "h264_720_60fps"},
			ExtraData: []string{"video_playback/h264_720_60fps.mp4"},
		}, {
			Name:      "h264_1080_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "h264_1080_30fps"},
			ExtraData: []string{"video_playback/h264_1080_30fps.mp4"},
			ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
		}, {
			Name:      "h264_1080_30fps_1hr_ash",
			Fixture:   "powerAshRamfs",
			ExtraData: []string{"video_playback/h264_1080_30fps.mp4"},
			ExtraAttr: []string{"group:power", "power_regression"},
			Val: videoPlaybackTestParam{
				VideoName:  "h264_1080_30fps",
				TimeParams: power.TimeParams{Total: time.Hour, Interval: 5 * time.Second}},
		}, {
			Name:      "h264_1080_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "h264_1080_60fps"},
			ExtraData: []string{"video_playback/h264_1080_60fps.mp4"},
		}, {
			Name:      "h264_4k_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "h264_4k_30fps"},
			ExtraData: []string{"video_playback/h264_4k_30fps.mp4"},
		}, {
			Name:      "h264_4k_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "h264_4k_60fps"},
			ExtraData: []string{"video_playback/h264_4k_60fps.mp4"},
		}, {
			Name:      "vp8_720_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp8_720_30fps"},
			ExtraData: []string{"video_playback/vp8_720_30fps.webm"},
		}, {
			Name:      "vp8_720_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp8_720_60fps"},
			ExtraData: []string{"video_playback/vp8_720_60fps.webm"},
		}, {
			Name:      "vp8_1080_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp8_1080_30fps"},
			ExtraData: []string{"video_playback/vp8_1080_30fps.webm"},
		}, {
			Name:      "vp8_1080_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp8_1080_60fps"},
			ExtraData: []string{"video_playback/vp8_1080_60fps.webm"},
		}, {
			Name:      "vp8_4k_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp8_4k_30fps"},
			ExtraData: []string{"video_playback/vp8_4k_30fps.webm"},
		}, {
			Name:      "vp8_4k_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp8_4k_60fps"},
			ExtraData: []string{"video_playback/vp8_4k_60fps.webm"},
		}, {
			Name:      "vp9_720_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp9_720_30fps"},
			ExtraData: []string{"video_playback/vp9_720_30fps.webm"},
		}, {
			Name:      "vp9_720_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp9_720_60fps"},
			ExtraData: []string{"video_playback/vp9_720_60fps.webm"},
		}, {
			Name:      "vp9_1080_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp9_1080_30fps"},
			ExtraData: []string{"video_playback/vp9_1080_30fps.webm"},
			ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
		}, {
			Name:      "vp9_1080_30fps_1hr_ash",
			Fixture:   "powerAshRamfs",
			ExtraData: []string{"video_playback/vp9_1080_30fps.webm"},
			ExtraAttr: []string{"group:power", "power_regression"},
			Val: videoPlaybackTestParam{
				VideoName:  "vp9_1080_30fps",
				TimeParams: power.TimeParams{Total: time.Hour, Interval: 5 * time.Second}},
		}, {
			Name:      "vp9_1080_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp9_1080_60fps"},
			ExtraData: []string{"video_playback/vp9_1080_60fps.webm"},
		}, {
			Name:      "vp9_4k_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp9_4k_30fps"},
			ExtraData: []string{"video_playback/vp9_4k_30fps.webm"},
		}, {
			Name:      "vp9_4k_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "vp9_4k_60fps"},
			ExtraData: []string{"video_playback/vp9_4k_60fps.webm"},
		}, {
			Name:      "av1_720_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "av1_720_30fps"},
			ExtraData: []string{"video_playback/av1_720_30fps.mp4"},
		}, {
			Name:      "av1_720_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "av1_720_60fps"},
			ExtraData: []string{"video_playback/av1_720_60fps.mp4"},
		}, {
			Name:      "av1_1080_30fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "av1_1080_30fps"},
			ExtraData: []string{"video_playback/av1_1080_30fps.mp4"},
		}, {
			Name:      "av1_1080_60fps_ash",
			Fixture:   "powerAshRamfs",
			Val:       videoPlaybackTestParam{VideoName: "av1_1080_60fps"},
			ExtraData: []string{"video_playback/av1_1080_60fps.mp4"},
		}, {
			Name:              "h264_720_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "h264_720_30fps"},
			ExtraData:         []string{"video_playback/h264_720_30fps.mp4"},
		}, {
			Name:              "h264_720_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "h264_720_60fps"},
			ExtraData:         []string{"video_playback/h264_720_60fps.mp4"},
		}, {
			Name:              "h264_1080_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "h264_1080_30fps"},
			ExtraData:         []string{"video_playback/h264_1080_30fps.mp4"},
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		}, {
			Name:              "h264_1080_30fps_1hr_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraData:         []string{"video_playback/h264_1080_30fps.mp4"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val: videoPlaybackTestParam{
				VideoName:  "h264_1080_30fps",
				TimeParams: power.TimeParams{Total: time.Hour, Interval: 5 * time.Second}},
		}, {
			Name:              "h264_1080_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "h264_1080_60fps"},
			ExtraData:         []string{"video_playback/h264_1080_60fps.mp4"},
		}, {
			Name:              "h264_4k_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "h264_4k_30fps"},
			ExtraData:         []string{"video_playback/h264_4k_30fps.mp4"},
		}, {
			Name:              "h264_4k_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "h264_4k_60fps"},
			ExtraData:         []string{"video_playback/h264_4k_60fps.mp4"},
		}, {
			Name:              "vp8_720_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp8_720_30fps"},
			ExtraData:         []string{"video_playback/vp8_720_30fps.webm"},
		}, {
			Name:              "vp8_720_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp8_720_60fps"},
			ExtraData:         []string{"video_playback/vp8_720_60fps.webm"},
		}, {
			Name:              "vp8_1080_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp8_1080_30fps"},
			ExtraData:         []string{"video_playback/vp8_1080_30fps.webm"},
		}, {
			Name:              "vp8_1080_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp8_1080_60fps"},
			ExtraData:         []string{"video_playback/vp8_1080_60fps.webm"},
		}, {
			Name:              "vp8_4k_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp8_4k_30fps"},
			ExtraData:         []string{"video_playback/vp8_4k_30fps.webm"},
		}, {
			Name:              "vp8_4k_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp8_4k_60fps"},
			ExtraData:         []string{"video_playback/vp8_4k_60fps.webm"},
		}, {
			Name:              "vp9_720_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp9_720_30fps"},
			ExtraData:         []string{"video_playback/vp9_720_30fps.webm"},
		}, {
			Name:              "vp9_720_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp9_720_60fps"},
			ExtraData:         []string{"video_playback/vp9_720_60fps.webm"},
		}, {
			Name:              "vp9_1080_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp9_1080_30fps"},
			ExtraData:         []string{"video_playback/vp9_1080_30fps.webm"},
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		}, {
			Name:              "vp9_1080_30fps_1hr_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraData:         []string{"video_playback/vp9_1080_30fps.webm"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val: videoPlaybackTestParam{
				VideoName:  "vp9_1080_30fps",
				TimeParams: power.TimeParams{Total: time.Hour, Interval: 5 * time.Second}},
		}, {
			Name:              "vp9_1080_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp9_1080_60fps"},
			ExtraData:         []string{"video_playback/vp9_1080_60fps.webm"},
		}, {
			Name:              "vp9_4k_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp9_4k_30fps"},
			ExtraData:         []string{"video_playback/vp9_4k_30fps.webm"},
		}, {
			Name:              "vp9_4k_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "vp9_4k_60fps"},
			ExtraData:         []string{"video_playback/vp9_4k_60fps.webm"},
		}, {
			Name:              "av1_720_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "av1_720_30fps"},
			ExtraData:         []string{"video_playback/av1_720_30fps.mp4"},
		}, {
			Name:              "av1_720_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "av1_720_60fps"},
			ExtraData:         []string{"video_playback/av1_720_60fps.mp4"},
		}, {
			Name:              "av1_1080_30fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "av1_1080_30fps"},
			ExtraData:         []string{"video_playback/av1_1080_30fps.mp4"},
		}, {
			Name:              "av1_1080_60fps_lacros",
			Fixture:           "powerLacrosRamfs",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               videoPlaybackTestParam{VideoName: "av1_1080_60fps"},
			ExtraData:         []string{"video_playback/av1_1080_60fps.mp4"},
		}},
	})
}

func VideoPlayback(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateFullscreen); err != nil {
		s.Fatal("Failed to make the browser window fullscreen: ", err)
	}

	// Copy file to ramfs. We don't want the difference between storage type to affect this test.
	// The reason is that the most common use case, video streaming, the data are loaded from RAM.
	format := s.Param().(videoPlaybackTestParam).VideoName
	interval := s.Param().(videoPlaybackTestParam).TimeParams.Interval
	total := s.Param().(videoPlaybackTestParam).TimeParams.Total

	// Use default value for timeParam if not set
	if interval == time.Duration(0) {
		interval = videoPlaybackDefaultTimeParams.Interval
	}
	if total == time.Duration(0) {
		total = videoPlaybackDefaultTimeParams.Total
	}

	// VP8 and VP9 use webm, h264, av1 use mp4.
	var fileName = format
	if strings.HasPrefix(format, "vp") {
		fileName += ".webm"
	} else {
		fileName += ".mp4"
	}

	filePathOnDisk := s.DataPath("video_playback/" + fileName)
	filePathOnRAM := "/tmp/ramdisk/" + fileName
	if err := testexec.CommandContext(ctx, "cp", filePathOnDisk, filePathOnRAM).Run(); err != nil {
		s.Fatalf("Can't copy video from %s to %s : %v", filePathOnDisk, filePathOnRAM, err)
	}

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	// Register test specific metrics.
	r.RegisterMetrics(power.NewVideoFpsMetrics(conn))

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	// Start of main test body.
	url := "file://" + filePathOnRAM
	if err := conn.Navigate(ctx, url); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}
	if err := conn.Eval(ctx, "document.getElementsByTagName('video')[0].loop=true", nil); err != nil {
		s.Fatal("Failed to make video loop: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// GoBigSleepLint: sleep to let device play the video.
	if err := testing.Sleep(ctx, total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// End of main test body.

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}

	// Clean up: open blank page and delete file from ramdisk.
	if err := conn.Navigate(ctx, "about:blank"); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}
	if err := testexec.CommandContext(ctx, "rm", "-f", filePathOnRAM).Run(); err != nil {
		s.Fatalf("Can't delete %s : %v", filePathOnRAM, err)
	}
}
