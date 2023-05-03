// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mtbf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/galleryapp"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/filesapp"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/mtbf"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	mediaPlaybackSmokeH264_720p      = "h264_720p.mp4"
	mediaPlaybackSmokeH264_1080p     = "h264_1080p.mp4"
	mediaPlaybackSmokePcm            = "GLASS.wav"
	mediaPlaybackSmokePcmS16be       = "intelinsideA_pcm_s16be.mov"
	mediaPlaybackSmokePcmS24be       = "intelinsideA_pcm_s24be.mov"
	mediaPlaybackSmokeM4a            = "audio.m4a"
	mediaPlaybackSmokeMp3            = "audio.mp3"
	mediaPlaybackSmokeHEAac          = "HE-AAC_Stereo_32000Hz.m4a"
	mediaPlaybackSmokeLCAac          = "AAC-LC_v4_Stereo_48000Hz.m4a"
	mediaPlaybackSmokeCorruptedVideo = "corruptedVideo.mp4"
	mediaPlaybackSmokeCorruptedAudio = "corruptedAudio.mp3"
)

type mediaPlaybackSmokeParam struct {
	media []string
	// corrupted is a field to indicate if the media is corrupted.
	// Corrupted media doesn't get to play.
	corrupted bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MediaPlaybackSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify media is supported by checking the series of media fundamental playback functionalities",
		Contacts: []string{
			"chromeos-perf-reliability-eng@google.com",
			"abergman@google.com",
			"xibin@google.com",
			"cienet-development@googlegroups.com",
			"sun.tsai@cienet.com", // Test author.
		},
		// ChromeOS > EngProd > Platform > SPERA > Automation
		BugComponent: "b:1025042",
		// MTBF tests are not included in mainline or crosbolt for now.
		Attr:         []string{"group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "video_mp4",
				Fixture: mtbf.LoginReuseFixture,
				Val: mediaPlaybackSmokeParam{
					media:     []string{mediaPlaybackSmokeH264_720p, mediaPlaybackSmokeH264_1080p},
					corrupted: false,
				},
				ExtraData: []string{mediaPlaybackSmokeH264_720p, mediaPlaybackSmokeH264_1080p},
			}, {
				Name:    "video_mov",
				Fixture: mtbf.LoginReuseFixture,
				Val: mediaPlaybackSmokeParam{
					media:     []string{mediaPlaybackSmokePcmS16be, mediaPlaybackSmokePcmS24be},
					corrupted: false,
				},
				ExtraData: []string{mediaPlaybackSmokePcmS16be, mediaPlaybackSmokePcmS24be},
			}, {
				Name:    "video_corrupted",
				Fixture: mtbf.LoginReuseFixture,
				Val: mediaPlaybackSmokeParam{
					media:     []string{mediaPlaybackSmokeCorruptedVideo},
					corrupted: true,
				},
				ExtraData: []string{mediaPlaybackSmokeCorruptedVideo},
			}, {
				Name:    "audio_aac",
				Fixture: mtbf.LoginReuseFixture,
				Val: mediaPlaybackSmokeParam{
					media:     []string{mediaPlaybackSmokeHEAac, mediaPlaybackSmokeLCAac},
					corrupted: false,
				},
				ExtraData: []string{mediaPlaybackSmokeHEAac, mediaPlaybackSmokeLCAac},
			}, {
				Name:    "audio_m4a",
				Fixture: mtbf.LoginReuseFixture,
				Val: mediaPlaybackSmokeParam{
					media:     []string{mediaPlaybackSmokeM4a},
					corrupted: false,
				},
				ExtraData: []string{mediaPlaybackSmokeM4a},
			}, {
				Name:    "audio_mp3",
				Fixture: mtbf.LoginReuseFixture,
				Val: mediaPlaybackSmokeParam{
					media:     []string{mediaPlaybackSmokeMp3},
					corrupted: false,
				},
				ExtraData: []string{mediaPlaybackSmokeMp3},
			}, {
				Name:    "audio_wav",
				Fixture: mtbf.LoginReuseFixture,
				Val: mediaPlaybackSmokeParam{
					media:     []string{mediaPlaybackSmokePcm},
					corrupted: false,
				},
				ExtraData: []string{mediaPlaybackSmokePcm},
			}, {
				Name:    "audio_corrupted",
				Fixture: mtbf.LoginReuseFixture,
				Val: mediaPlaybackSmokeParam{
					media:     []string{mediaPlaybackSmokeCorruptedAudio},
					corrupted: true,
				},
				ExtraData: []string{mediaPlaybackSmokeCorruptedAudio},
			},
		},
	})
}

// MediaPlaybackSmoke verifies video is supported by checking the series of video fundamental playback functionalities.
func MediaPlaybackSmoke(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	recorder, err := mtbf.NewRecorder(ctx)
	if err != nil {
		s.Fatal("Failed to start record performance: ", err)
	}
	defer recorder.Record(cleanupCtx, s.OutDir())

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard controller: ", err)
	}
	defer kb.Close(ctx)

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open Files App: ", err)
	}
	defer filesApp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "filesapp_ui_dump")

	param := s.Param().(mediaPlaybackSmokeParam)
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve users Downloads path: ", err)
	}

	for _, fileName := range param.media {
		s.Run(ctx, "playback "+fileName, func(ctx context.Context, s *testing.State) {
			fileLocation := filepath.Join(downloadsPath, fileName)
			if err := fsutil.CopyFile(s.DataPath(fileName), fileLocation); err != nil {
				s.Fatalf("Failed to copy the test media %q to Downloads: %v", fileName, err)
			}
			defer os.Remove(fileLocation)

			if err := uiauto.Combine(fmt.Sprintf("open the file %q", fileName),
				filesApp.OpenDownloads(),
				filesApp.WaitForFile(fileName),
				filesApp.OpenFile(fileName),
			)(ctx); err != nil {
				s.Fatal("Failed to complete all actions: ", err)
			}

			gallery, err := galleryapp.ConnectToApp(ctx, cr, tconn)
			if err != nil {
				s.Fatal("Failed to connect to gallery app: ", err)
			}
			defer gallery.Close(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "gallery_ui_dump_"+fileName)

			if err := mediaPlayback(ctx, gallery, param.corrupted); err != nil {
				s.Fatalf("Failed to verify the playback functionalities of the media %q: %v", fileName, err)
			}
		})
	}
}

func mediaPlayback(ctx context.Context, gallery *galleryapp.Gallery, corrupted bool) error {
	if corrupted {
		testing.ContextLog(ctx, "Verify the decode failure message shows properly when playing a corrupted media")
		return gallery.DecodeFailed(ctx)
	}

	if err := uiauto.Combine("play, pause and resume the media",
		gallery.Play,
		gallery.Pause,
		gallery.Play,
	)(ctx); err != nil {
		return err
	}

	duration, err := gallery.MediaDuration(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to fetch the media duration")
	}
	testing.ContextLog(ctx, "Media duration in seconds: ", duration)

	// The short media would not have enough duration to verify forward, backward and random seeking.
	// Therefore, only do the verification on the long enough media.
	const threshold = 15.0
	if duration < threshold {
		testing.ContextLog(ctx, "Skip forward, backward and random seeking")
		return nil
	}

	if err := uiauto.Combine("forward and backward",
		gallery.Forward,
		gallery.Backward,
	)(ctx); err != nil {
		return err
	}

	testing.ContextLog(ctx, "Random seeking")
	for i := 0; i < 5; i++ {
		if err := gallery.RandomSeek(ctx); err != nil {
			return errors.Wrap(err, "failed to finish random seeking")
		}
	}
	return nil
}
