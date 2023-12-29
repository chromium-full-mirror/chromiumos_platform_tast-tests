// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/arcvideoplayback"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type arcVideoTestParam struct {
	VideoName  string
	TimeParams power.TimeParams
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCVideoPlayback,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect power metrics of playing video of different video formats in video app in full screen",
		Contacts:     []string{"chromeos-platform-power@google.com", "cienet-development@googlegroups.com", "vivian.chen@cienet.com"},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "powerAshARC",
		Params: []testing.Param{
			{
				Name:      "h264_720_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "h264_720_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/h264_720_30fps.mp4"},
			}, {
				Name:      "h264_720_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "h264_720_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/h264_720_60fps.mp4"},
			}, {
				Name:      "h264_1080_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "h264_1080_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/h264_1080_30fps.mp4"},
			}, {
				Name:      "h264_1080_30fps_1hr_ash",
				Val:       arcVideoTestParam{VideoName: "h264_1080_30fps", TimeParams: power.TimeParams{Total: time.Hour, Interval: 5 * time.Second}},
				Timeout:   time.Hour + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/h264_1080_30fps.mp4"},
				ExtraAttr: []string{"group:power", "power_regression"},
			}, {
				Name:      "h264_1080_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "h264_1080_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/h264_1080_60fps.mp4"},
			}, {
				Name:      "h264_4k_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "h264_4k_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/h264_4k_30fps.mp4"},
			}, {
				Name:      "h264_4k_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "h264_4k_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/h264_4k_60fps.mp4"},
			}, {
				Name:      "vp8_720_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp8_720_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp8_720_30fps.webm"},
			}, {
				Name:      "vp8_720_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp8_720_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp8_720_60fps.webm"},
			}, {
				Name:      "vp8_1080_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp8_1080_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp8_1080_30fps.webm"},
			}, {
				Name:      "vp8_1080_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp8_1080_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp8_1080_60fps.webm"},
			}, {
				Name:      "vp8_4k_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp8_4k_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp8_4k_30fps.webm"},
			}, {
				Name:      "vp8_4k_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp8_4k_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp8_4k_60fps.webm"},
			}, {
				Name:      "vp9_720_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp9_720_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp9_720_30fps.webm"},
			}, {
				Name:      "vp9_720_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp9_720_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp9_720_60fps.webm"},
			}, {
				Name:      "vp9_1080_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp9_1080_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp9_1080_30fps.webm"},
			}, {
				Name:      "vp9_1080_30fps_1hr_ash",
				Val:       arcVideoTestParam{VideoName: "vp9_1080_30fps", TimeParams: power.TimeParams{Total: time.Hour, Interval: 5 * time.Second}},
				Timeout:   time.Hour + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp9_1080_30fps.webm"},
				ExtraAttr: []string{"group:power", "power_regression"},
			}, {
				Name:      "vp9_1080_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp9_1080_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp9_1080_60fps.webm"},
			}, {
				Name:      "vp9_4k_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp9_4k_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp9_4k_30fps.webm"},
			}, {
				Name:      "vp9_4k_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "vp9_4k_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/vp9_4k_60fps.webm"},
			}, {
				Name:      "av1_720_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "av1_720_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/av1_720_30fps.mp4"},
			}, {
				Name:      "av1_720_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "av1_720_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/av1_720_60fps.mp4"},
			}, {
				Name:      "av1_1080_30fps_ash",
				Val:       arcVideoTestParam{VideoName: "av1_1080_30fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/av1_1080_30fps.mp4"},
			}, {
				Name:      "av1_1080_60fps_ash",
				Val:       arcVideoTestParam{VideoName: "av1_1080_60fps"},
				Timeout:   5*time.Minute + power.RecorderTimeout,
				ExtraData: []string{"arc_video_playback/av1_1080_60fps.mp4"},
			},
		},
	})
}

// ARCVideoPlayback performs the video test in video app and collects the power-related data.
func ARCVideoPlayback(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	a := s.FixtValue().(setup.PowerUIFixtureData).ARC

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to create new ARC device: ", err)
	}
	defer func(ctx context.Context) {
		// If an error happened, the device will be closed before dumping ARC UI.
		// Only close if the device is alive.
		if d.Alive(ctx) {
			d.Close(ctx)
		}
	}(cleanupCtx)

	mxApp := arcvideoplayback.NewMxPlayerApp(tconn, kb, a, d)
	if err := mxApp.Install(ctx); err != nil {
		s.Fatal("Failed to install MX Player app: ", err)
	}
	defer mxApp.Uninstall(cleanupCtx)

	interval := s.Param().(arcVideoTestParam).TimeParams.Interval
	total := s.Param().(arcVideoTestParam).TimeParams.Total
	videoName := s.Param().(arcVideoTestParam).VideoName

	// Use default value for timeParam if not set.
	defaultTimeParams := power.TimeParams{Interval: 5 * time.Second, Total: 5 * time.Minute}
	if interval == time.Duration(0) {
		interval = defaultTimeParams.Interval
	}
	if total == time.Duration(0) {
		total = defaultTimeParams.Total
	}

	// VP8 and VP9 use webm, h264, av1 use mp4.
	fileName := videoName
	if strings.HasPrefix(videoName, "vp") {
		fileName += ".webm"
	} else {
		fileName += ".mp4"
	}
	videoPath := s.DataPath("arc_video_playback/" + fileName)
	cleanupFile, err := mxApp.CopyFileToDownloadsFolder(ctx, videoPath)
	if err != nil {
		s.Fatal("Failed to copy video file to Downloads folder: ", err)
	}
	defer cleanupFile()

	recorder := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer recorder.Close(cleanupCtx)

	if err := recorder.Cooldown(ctx); err != nil {
		s.Fatal("Failed to cool down: ", err)
	}

	if err := mxApp.Launch(ctx); err != nil {
		s.Fatal("Failed to launch MX Player app: ", err)
	}
	defer func(ctx context.Context) {
		// Make sure to close the arc UI device before calling the function.
		// Otherwise uiautomator might have errors.
		if err := d.Close(ctx); err != nil {
			s.Log("Failed to close ARC UI device: ", err)
		}
		if err := a.DumpUIHierarchyOnError(ctx, filepath.Join(s.OutDir(), "arc"), s.HasError); err != nil {
			s.Log("Failed to dump arc: ", err)
		}
		faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")
		mxApp.Close(ctx)
	}(cleanupCtx)

	if err := uiauto.NamedCombine("enter full screen and play video",
		mxApp.DismissPrompts,
		mxApp.EnterFullScreen,
		mxApp.OpenAndPlayVideo(videoName),
		mxApp.SetLoopOn,
	)(ctx); err != nil {
		s.Fatal("Failed to enter full screen and play video: ", err)
	}

	s.Logf("Run test testName: %s, video: %q", s.TestName(), fileName)
	if err := recorder.Start(ctx); err != nil {
		s.Fatal("Failed to start collecting power metrics: ", err)
	}

	s.Logf("Play video for %v to measure power consumption: ", total)
	// GoBigSleepLint: sleep to let device play the video.
	if err := testing.Sleep(ctx, total); err != nil {
		s.Fatal("Failed to sleep while video is playing: ", err)
	}

	if err := recorder.Finish(ctx); err != nil {
		s.Fatal("Failed to finish collecting power metrics: ", err)
	}
}
