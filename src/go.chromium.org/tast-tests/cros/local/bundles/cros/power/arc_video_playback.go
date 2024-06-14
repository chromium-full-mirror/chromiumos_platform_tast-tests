// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/local/arc"
	arcvpb "go.chromium.org/tast-tests/cros/local/bundles/cros/power/arcvideoplayback"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type arcVideoTestParam struct {
	App        func(cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *androidui.Device, dataPath func(string) string) arcvpb.VideoApp
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
		Timeout:      3*time.Hour + power.RecorderTimeout, // 1 hour for video and 2 hour for charging
		Params: []testing.Param{
			{
				Name:      "mxplayer_h264_720_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "h264_720_30fps"},
				ExtraData: []string{"arc_video_playback/h264_720_30fps.mp4"},
			}, {
				Name:      "mxplayer_h264_720_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "h264_720_60fps"},
				ExtraData: []string{"arc_video_playback/h264_720_60fps.mp4"},
			}, {
				Name:      "mxplayer_h264_1080_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "h264_1080_30fps"},
				ExtraData: []string{"arc_video_playback/h264_1080_30fps.mp4"},
				ExtraAttr: []string{"group:power", "power_regression"},
			}, {
				Name:      "mxplayer_h264_1080_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "h264_1080_60fps"},
				ExtraData: []string{"arc_video_playback/h264_1080_60fps.mp4"},
			}, {
				Name:      "mxplayer_h264_4k_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "h264_4k_30fps"},
				ExtraData: []string{"arc_video_playback/h264_4k_30fps.mp4"},
			}, {
				Name:      "mxplayer_h264_4k_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "h264_4k_60fps"},
				ExtraData: []string{"arc_video_playback/h264_4k_60fps.mp4"},
			}, {
				Name:      "mxplayer_vp8_720_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp8_720_30fps"},
				ExtraData: []string{"arc_video_playback/vp8_720_30fps.webm"},
			}, {
				Name:      "mxplayer_vp8_720_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp8_720_60fps"},
				ExtraData: []string{"arc_video_playback/vp8_720_60fps.webm"},
			}, {
				Name:      "mxplayer_vp8_1080_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp8_1080_30fps"},
				ExtraData: []string{"arc_video_playback/vp8_1080_30fps.webm"},
			}, {
				Name:      "mxplayer_vp8_1080_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp8_1080_60fps"},
				ExtraData: []string{"arc_video_playback/vp8_1080_60fps.webm"},
			}, {
				Name:      "mxplayer_vp8_4k_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp8_4k_30fps"},
				ExtraData: []string{"arc_video_playback/vp8_4k_30fps.webm"},
			}, {
				Name:      "mxplayer_vp8_4k_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp8_4k_60fps"},
				ExtraData: []string{"arc_video_playback/vp8_4k_60fps.webm"},
			}, {
				Name:      "mxplayer_vp9_720_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp9_720_30fps"},
				ExtraData: []string{"arc_video_playback/vp9_720_30fps.webm"},
			}, {
				Name:      "mxplayer_vp9_720_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp9_720_60fps"},
				ExtraData: []string{"arc_video_playback/vp9_720_60fps.webm"},
			}, {
				Name:      "mxplayer_vp9_1080_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp9_1080_30fps"},
				ExtraData: []string{"arc_video_playback/vp9_1080_30fps.webm"},
				ExtraAttr: []string{"group:power", "power_regression"},
			}, {
				Name:      "mxplayer_vp9_1080_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp9_1080_60fps"},
				ExtraData: []string{"arc_video_playback/vp9_1080_60fps.webm"},
			}, {
				Name:      "mxplayer_vp9_4k_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp9_4k_30fps"},
				ExtraData: []string{"arc_video_playback/vp9_4k_30fps.webm"},
			}, {
				Name:      "mxplayer_vp9_4k_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "vp9_4k_60fps"},
				ExtraData: []string{"arc_video_playback/vp9_4k_60fps.webm"},
			}, {
				Name:      "mxplayer_av1_720_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "av1_720_30fps"},
				ExtraData: []string{"arc_video_playback/av1_720_30fps.mp4"},
			}, {
				Name:      "mxplayer_av1_720_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "av1_720_60fps"},
				ExtraData: []string{"arc_video_playback/av1_720_60fps.mp4"},
			}, {
				Name:      "mxplayer_av1_1080_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "av1_1080_30fps"},
				ExtraData: []string{"arc_video_playback/av1_1080_30fps.mp4"},
			}, {
				Name:      "mxplayer_av1_1080_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewMxPlayerApp, VideoName: "av1_1080_60fps"},
				ExtraData: []string{"arc_video_playback/av1_1080_60fps.mp4"},
			}, {
				Name:      "exoplayer_h264_720_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "h264_720_30fps"},
				ExtraData: []string{"arc_video_playback/h264_720_30fps.mp4", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_h264_720_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "h264_720_60fps"},
				ExtraData: []string{"arc_video_playback/h264_720_60fps.mp4", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_h264_1080_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "h264_1080_30fps"},
				ExtraData: []string{"arc_video_playback/h264_1080_30fps.mp4", arcvpb.ExoPlayerAPKFileName},
				ExtraAttr: []string{"group:power", "power_regression"},
			}, {
				Name:      "exoplayer_h264_1080_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "h264_1080_60fps"},
				ExtraData: []string{"arc_video_playback/h264_1080_60fps.mp4", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_h264_4k_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "h264_4k_30fps"},
				ExtraData: []string{"arc_video_playback/h264_4k_30fps.mp4", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_h264_4k_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "h264_4k_60fps"},
				ExtraData: []string{"arc_video_playback/h264_4k_60fps.mp4", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp8_720_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp8_720_30fps"},
				ExtraData: []string{"arc_video_playback/vp8_720_30fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp8_720_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp8_720_60fps"},
				ExtraData: []string{"arc_video_playback/vp8_720_60fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp8_1080_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp8_1080_30fps"},
				ExtraData: []string{"arc_video_playback/vp8_1080_30fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp8_1080_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp8_1080_60fps"},
				ExtraData: []string{"arc_video_playback/vp8_1080_60fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp8_4k_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp8_4k_30fps"},
				ExtraData: []string{"arc_video_playback/vp8_4k_30fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp8_4k_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp8_4k_60fps"},
				ExtraData: []string{"arc_video_playback/vp8_4k_60fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp9_720_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp9_720_30fps"},
				ExtraData: []string{"arc_video_playback/vp9_720_30fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp9_720_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp9_720_60fps"},
				ExtraData: []string{"arc_video_playback/vp9_720_60fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp9_1080_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp9_1080_30fps"},
				ExtraData: []string{"arc_video_playback/vp9_1080_30fps.webm", arcvpb.ExoPlayerAPKFileName},
				ExtraAttr: []string{"group:power", "power_regression"},
			}, {
				Name:      "exoplayer_vp9_1080_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp9_1080_60fps"},
				ExtraData: []string{"arc_video_playback/vp9_1080_60fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp9_4k_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp9_4k_30fps"},
				ExtraData: []string{"arc_video_playback/vp9_4k_30fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_vp9_4k_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "vp9_4k_60fps"},
				ExtraData: []string{"arc_video_playback/vp9_4k_60fps.webm", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_av1_720_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "av1_720_30fps"},
				ExtraData: []string{"arc_video_playback/av1_720_30fps.mp4", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_av1_720_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "av1_720_60fps"},
				ExtraData: []string{"arc_video_playback/av1_720_60fps.mp4", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_av1_1080_30fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "av1_1080_30fps"},
				ExtraData: []string{"arc_video_playback/av1_1080_30fps.mp4", arcvpb.ExoPlayerAPKFileName},
			}, {
				Name:      "exoplayer_av1_1080_60fps_ash",
				Val:       arcVideoTestParam{App: arcvpb.NewExoPlayerApp, VideoName: "av1_1080_60fps"},
				ExtraData: []string{"arc_video_playback/av1_1080_60fps.mp4", arcvpb.ExoPlayerAPKFileName},
			},
		},
	})
}

// ARCVideoPlayback performs the video test in video app and collects the power-related data.
func ARCVideoPlayback(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
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

	interval := s.Param().(arcVideoTestParam).TimeParams.Interval
	total := s.Param().(arcVideoTestParam).TimeParams.Total
	videoName := s.Param().(arcVideoTestParam).VideoName
	app := s.Param().(arcVideoTestParam).App

	videoApp := app(cr, tconn, kb, a, d, s.DataPath)
	if err := videoApp.Install(ctx); err != nil {
		s.Fatal("Failed to install video app: ", err)
	}
	defer videoApp.Uninstall(cleanupCtx)

	configValues := perf.NewValues()
	appVersion, err := videoApp.GetAppVersion(ctx)
	if err != nil {
		s.Error("Failed to get video app version: ", err)
	}
	appVersionNum, err := convertAppVersionToFloat(appVersion)
	if err != nil {
		s.Error("Failed to convert app version from string to float: ", err)
	}
	configValues.Set(perf.Metric{Name: cp.GeneralPerfMetricType + "arc_video_app_version", Unit: "unit", Direction: perf.BiggerIsBetter}, appVersionNum)

	// Use default value for timeParam if not set.
	defaultTimeParams := power.TimeParams{Interval: 5 * time.Second, Total: time.Hour}
	if interval == time.Duration(0) {
		interval = defaultTimeParams.Interval
	}
	if total == time.Duration(0) {
		total = defaultTimeParams.Total
	}
	// For tests that take more than 1 hour, make sure the device has at least
	// 50% of battery.
	if total >= time.Hour {
		s.Logf("Prepare the device to have at least %.2f%% battery", power.RegressionTestChargeParam.MinChargePercentage)
		setup.PrepareBattery(ctx, power.RegressionTestChargeParam)
		discharge = power.RegressionTestChargeParam.DischargeOnCompletion
	}

	// VP8 and VP9 use webm, h264, av1 use mp4.
	fileName := videoName
	if strings.HasPrefix(videoName, "vp") {
		fileName += ".webm"
	} else {
		fileName += ".mp4"
	}
	cleanupFile, err := videoApp.CopyFileToFolder(ctx, "arc_video_playback/"+fileName)
	if err != nil {
		s.Fatal("Failed to copy video file to folder: ", err)
	}
	defer cleanupFile()

	recorder := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer recorder.Close(cleanupCtx)

	if err := recorder.Cooldown(ctx); err != nil {
		s.Fatal("Failed to cool down: ", err)
	}

	if err := videoApp.Launch(ctx); err != nil {
		s.Fatal("Failed to launch video app: ", err)
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
		videoApp.Close(ctx)
	}(cleanupCtx)

	if err := videoApp.PlayVideoInFullScreen(fileName)(ctx); err != nil {
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

	// Save custom perf values to power logs and results-chart.json.
	recorder.AddOptionalRecorderArg(cp.OptionalRecorderArgPowerLogCustomPerfKey, configValues)
	if err := recorder.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}

// convertAppVersionToFloat converts the app version from string to float
// for easier comparisons within the power dashboard.
func convertAppVersionToFloat(appVersion string) (float64, error) {
	// Only keep major.minor.patch of the app version and disgard pre-release label
	// if there is one. For example, for "1.4.0-alpha01", we only keep "1.4.0".
	transformedAppVersion := strings.Split(appVersion, "-")[0]
	// Pad the app version if minor or patch is missing. For example, "1.4" will
	// be padded as "1.4.0".
	for count := strings.Count(transformedAppVersion, "."); count < 2; count++ {
		transformedAppVersion = transformedAppVersion + ".0"
	}

	var appVersionNum float64
	appVersionSlice := strings.Split(transformedAppVersion, ".")
	for _, n := range appVersionSlice {
		num, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to convert %q to float", n)
		}
		appVersionNum = appVersionNum*1000 + num
	}
	return appVersionNum, nil
}
