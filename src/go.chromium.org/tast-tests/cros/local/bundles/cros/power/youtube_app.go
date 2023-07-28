// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	videoFPS30Title = "Bora Bora 4K - Scenic Relaxation Film with Calming Music"
	videoFPS60Title = "Stadia GDC 2019 Gaming Announcement"
	// The video duration of |videoFPS30URL| and |videoFPS60URL| should be longer
	// than |youtubeAppTestDuration| to properly test the power consumption.
	videoFPS30URL = "https://youtu.be/FyzlhpMFxok"
	videoFPS60URL = "https://youtu.be/nUih5C5rOrA"

	youtubeAppTestDuration = 5 * time.Minute
	// Add 5 minutes buffer time for launching app and cleanup actions.
	youtubeAppTimeout = youtubeAppTestDuration + 5*time.Minute + power.RecorderTimeout
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         YoutubeApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect power metrics of playing YouTube video of different video formats in full screen",
		Contacts:     []string{"chromeos-platform-power@google.com", "cienet-development@googlegroups.com", "vivian.chen@cienet.com"},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "powerAshARC",
		Vars: []string{
			"power.yt_apk_url", // Optional. The url of apk file.
		},
		Timeout: youtubeAppTimeout,
		Params: []testing.Param{
			{
				Name: "720p_30fps_ash",
				Val: youtube.VideoSrc{
					Title:   videoFPS30Title,
					URL:     videoFPS30URL,
					Quality: youtube.Quality720P,
				},
			},
			{
				Name: "720p_60fps_ash",
				Val: youtube.VideoSrc{
					Title:   videoFPS60Title,
					URL:     videoFPS60URL,
					Quality: youtube.Quality720P60,
				},
			},
			{
				Name: "1080p_30fps_ash",
				Val: youtube.VideoSrc{
					Title:   videoFPS30Title,
					URL:     videoFPS30URL,
					Quality: youtube.Quality1080P,
				},
			},
			{
				Name: "1080p_60fps_ash",
				Val: youtube.VideoSrc{
					Title:   videoFPS60Title,
					URL:     videoFPS60URL,
					Quality: youtube.Quality1080P60,
				},
			},
			{
				Name: "4k_30fps_ash",
				Val: youtube.VideoSrc{
					Title:   videoFPS30Title,
					URL:     videoFPS30URL,
					Quality: youtube.Quality2160P,
				},
			},
			{
				Name: "4k_60fps_ash",
				Val: youtube.VideoSrc{
					Title:   videoFPS60Title,
					URL:     videoFPS60URL,
					Quality: youtube.Quality2160P60,
				},
			},
		},
	})
}

// YoutubeApp performs the video test on youtube app.
func YoutubeApp(ctx context.Context, s *testing.State) {
	videoSrc := s.Param().(youtube.VideoSrc)
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	a := s.FixtValue().(setup.PowerUIFixtureData).ARC

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	youtubeApkURL := ""
	if v, ok := s.Var("power.yt_apk_url"); ok {
		youtubeApkURL = v
	} else {
		s.Log("Apk url is not provided, will install latest version")
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to create new ARC device: ", err)
	}

	outDir := s.OutDir()
	ytApp := youtube.NewYtApp(cr, tconn, kb, a, d, outDir, youtubeApkURL)
	if err := ytApp.Install(ctx); err != nil {
		s.Fatal("Failed to install Youtube app: ", err)
	}
	defer ytApp.Uninstall(cleanupCtx)

	checkInterval := time.Second
	recorder := power.NewRecorder(ctx, checkInterval, outDir, s.TestName())
	defer recorder.Close(cleanupCtx)

	if err := recorder.Cooldown(ctx); err != nil {
		s.Fatal("Failed to cool down: ", err)
	}

	if err := ytApp.Launch(ctx); err != nil {
		s.Fatal("Failed to launch Youtube app: ", err)
	}
	defer func(ctx context.Context) {
		// Make sure to close the arc UI device before calling the function.
		// Otherwise uiautomator might have errors.
		if err := d.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close ARC UI device: ", err)
		}
		if err := a.DumpUIHierarchyOnError(ctx, filepath.Join(outDir, "arc"), s.HasError); err != nil {
			testing.ContextLog(ctx, "Failed to dump arc: ", err)
		}
		faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, s.HasError, cr, "ui_dump")
		ytApp.Close(ctx)
	}(cleanupCtx)

	if err := searchAndPlayVideoInFullScreen(ctx, ytApp, videoSrc); err != nil {
		s.Fatal("Failed to search and play video in full screen: ", err)
	}

	s.Logf("Run test testName: %s, url: %q, quality: %s", s.TestName(), videoSrc.URL, videoSrc.Quality)
	if err := recorder.Start(ctx); err != nil {
		s.Fatal("Failed to start collecting power metrics: ", err)
	}

	// GoBigSleepLint: Play video for 5 minutes to measure power consumption.
	if err := testing.Sleep(ctx, youtubeAppTestDuration); err != nil {
		s.Fatal("Failed to sleep while video is playing: ", err)
	}

	if err := recorder.Finish(ctx); err != nil {
		s.Fatal("Failed to finish collecting power metrics: ", err)
	}
}

// searchAndPlayVideoInFullScreen searches and plays the given video in specific quality
// from beginning in full screen.
func searchAndPlayVideoInFullScreen(ctx context.Context, videoApp *youtube.YtApp, videoSrc youtube.VideoSrc) error {
	if err := videoApp.SearchAndPlayVideo(ctx, videoSrc); err != nil {
		return errors.Wrapf(err, "failed to search and play video: %s(%s)", videoSrc.Title, videoSrc.Quality)
	}
	if err := videoApp.EnterFullScreen(ctx); err != nil {
		return errors.Wrap(err, "failed to enter full screen")
	}
	if err := videoApp.SwitchQuality(ctx, videoSrc.Quality); err != nil {
		return errors.Wrap(err, "failed to switch video quality")
	}
	if err := videoApp.StartFromBeginning(ctx); err != nil {
		return errors.Wrap(err, "failed to start the video from beginning")
	}
	if err := videoApp.IsPlaying()(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure video is playing")
	}
	return nil
}
