// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type videoPlaybackDrmTestParam struct {
	VideoName  string
	TimeParams power.TimeParams
}

var videoPlaybackDrmDefaultTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 6 * time.Minute}

var hwdrmDataFiles = []string{
	"drm_video_playback/shaka_hwdrm.html",
	"drm_video_playback/third_party/shaka-player/shaka-player.compiled.debug.js",
	"drm_video_playback/third_party/shaka-player/shaka-player.compiled.debug.map",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoPlaybackDrm,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics playing protected offline video",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"jkardatzke@google.com"},
		SoftwareDeps: []string{"chrome", "protected_content"},
		Params: []testing.Param{{
			Name:              "cencv1_h264_ctr_ash",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_h264_cencv1_ctr.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_h264_cencv1_ctr.mp4", "drm_video_playback/tulip_audio_aac_cencv1_ctr.mp4", "drm_video_playback/tulip_480p_h264_cencv1_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV1H264, "proprietary_codecs"},
			Fixture:           "powerAshProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv1_h264_ctr_lacros",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_h264_cencv1_ctr.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_h264_cencv1_ctr.mp4", "drm_video_playback/tulip_audio_aac_cencv1_ctr.mp4", "drm_video_playback/tulip_480p_h264_cencv1_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV1H264, "proprietary_codecs"},
			Fixture:           "powerLacrosProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv3_h264_ctr_ash",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_h264_cencv3_ctr.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_h264_cencv3_ctr.mp4", "drm_video_playback/tulip_audio_aac_cencv3_ctr.mp4", "drm_video_playback/tulip_480p_h264_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3H264, "proprietary_codecs"},
			Fixture:           "powerAshProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv3_h264_ctr_lacros",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_h264_cencv3_ctr.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_h264_cencv3_ctr.mp4", "drm_video_playback/tulip_audio_aac_cencv3_ctr.mp4", "drm_video_playback/tulip_480p_h264_cencv3_ctr.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3H264, "proprietary_codecs"},
			Fixture:           "powerLacrosProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv3_hevc_ctr_ash",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_hevc_cencv3_ctr.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_hevc_cencv3_ctr.mp4", "drm_video_playback/tulip_audio_aac_cencv3_ctr.mp4", "drm_video_playback/tulip_480p_hevc_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           "powerAshProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv3_hevc_ctr_lacros",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_hevc_cencv3_ctr.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_hevc_cencv3_ctr.mp4", "drm_video_playback/tulip_audio_aac_cencv3_ctr.mp4", "drm_video_playback/tulip_480p_hevc_cencv3_ctr.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeCTRV3HEVC, "proprietary_codecs"},
			Fixture:           "powerLacrosProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv3_vp9_cbc_ash",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_vp9_cencv3_cbc.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_vp9_cencv3_cbc.mp4", "drm_video_playback/tulip_audio_aac_cencv3_cbc.mp4", "drm_video_playback/tulip_480p_vp9_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3VP9, "proprietary_codecs"},
			Fixture:           "powerAshProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv3_vp9_cbc_lacros",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_vp9_cencv3_cbc.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_vp9_cencv3_cbc.mp4", "drm_video_playback/tulip_audio_aac_cencv3_cbc.mp4", "drm_video_playback/tulip_480p_vp9_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3VP9, "proprietary_codecs"},
			Fixture:           "powerLacrosProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv3_av1_cbc_ash",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_av1_cencv3_cbc.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_av1_cencv3_cbc.webm", "drm_video_playback/tulip_audio_aac_cencv3_cbc.mp4", "drm_video_playback/tulip_480p_av1_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3AV1, "proprietary_codecs"},
			Fixture:           "powerAshProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}, {
			Name:              "cencv3_av1_cbc_lacros",
			Val:               videoPlaybackDrmTestParam{VideoName: "tulip_480p_av1_cencv3_cbc.mpd"},
			ExtraData:         append(hwdrmDataFiles, "drm_video_playback/tulip_480p_av1_cencv3_cbc.webm", "drm_video_playback/tulip_audio_aac_cencv3_cbc.mp4", "drm_video_playback/tulip_480p_av1_cencv3_cbc.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeCBCV3AV1, "proprietary_codecs"},
			Fixture:           "powerLacrosProtectedVideo",
			Timeout:           6*time.Minute + power.RecorderTimeout,
		}},
	})
}

func VideoPlaybackDrm(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

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

	interval := s.Param().(videoPlaybackDrmTestParam).TimeParams.Interval
	total := s.Param().(videoPlaybackDrmTestParam).TimeParams.Total

	// Use default value for timeParam if not set
	if interval == time.Duration(0) {
		interval = videoPlaybackDrmDefaultTimeParams.Interval
	}
	if total == time.Duration(0) {
		total = videoPlaybackDrmDefaultTimeParams.Total
	}
	// For tests that take more than 1 hour, make sure the device has at least
	// 50% of battery.
	if total >= time.Hour {
		s.Logf("Prepare the device to have at least %.2f%% battery", power.RegressionTestChargeParam.MinChargePercentage)
		setup.PrepareBattery(ctx, power.RegressionTestChargeParam)
	}

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)
	// Register test specific metrics.
	r.RegisterMetrics(pm.NewVideoFpsMetrics(conn))

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	// Start of main test body.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	url := server.URL + "/drm_video_playback/shaka_hwdrm.html"

	if err := conn.Navigate(ctx, url); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}

	mpdFile := s.Param().(videoPlaybackDrmTestParam).VideoName
	if err := conn.Call(ctx, nil, "play_shaka_drm", mpdFile); err != nil {
		s.Fatal("Failed to start playback: ", err)
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

	// Clean up: open blank page.
	if err := conn.Navigate(ctx, "about:blank"); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}
}
