// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	audiofixture "go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioRecord,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests basic audio recording on lacros",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "yuhsuan@chromium.org"},
		BugComponent: "b:167272",
		Attr:         []string{"group:mainline", "group:audio"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Timeout:      7 * time.Minute, // A lenient limit for launching Lacros Chrome.
		Data:         []string{"audio_capture_test.html", "audio_capture_test.js"},
		Params: []testing.Param{{
			Name:              "stable_fieldtrial_config_disable",
			Fixture:           audiofixture.AloopLoaded{Channels: 2, Parent: "lacrosAudioFieldTrialConfigDisable"}.Instance(),
			ExtraSoftwareDeps: []string{"lacros_stable"},
			ExtraAttr:         []string{"group:cq-medium"},
		}, {
			Name:              "unstable_fieldtrial_config_disable",
			Fixture:           audiofixture.AloopLoaded{Channels: 2, Parent: "lacrosAudioFieldTrialConfigDisable"}.Instance(),
			ExtraSoftwareDeps: []string{"lacros_unstable"},
			ExtraAttr:         []string{"informational"},
		}, {
			Name:              "stable_fieldtrial_config_enable",
			Fixture:           audiofixture.AloopLoaded{Channels: 2, Parent: "lacrosAudioFieldTrialConfigEnable"}.Instance(),
			ExtraSoftwareDeps: []string{"lacros_stable"},
			ExtraAttr:         []string{"informational", "group:cq-medium"},
		}, {
			Name:              "unstable_fieldtrial_config_enable",
			Fixture:           audiofixture.AloopLoaded{Channels: 2, Parent: "lacrosAudioFieldTrialConfigEnable"}.Instance(),
			ExtraSoftwareDeps: []string{"lacros_unstable"},
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func AudioRecord(ctx context.Context, s *testing.State) {
	const (
		cleanupTime      = 45 * time.Second
		playbackDuration = 10 * time.Second
		goldenFrequency  = 440 // Hz
		incorrectLimit   = 3
		rate             = 48000
		channels         = 2
		downloadsURL     = "chrome://downloads"
		recordedFileName = "test.webm"
		playbackFileName = "noise.wav"
	)
	chrome := s.FixtValue().(chrome.HasChrome).Chrome()

	// Reserve time to remove input file and unload ALSA loopback at the end of the test.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	defer func(ctx context.Context) {
		if err := crastestclient.WaitForNoStream(ctx, 15*time.Second); err != nil {
			// There are still active stream, mark as error and dump audio diagnostic to see the stream info.
			s.Error("Wait for no stream error: ", err)
			if err := crastestclient.DumpAudioDiagnostics(ctx, s.OutDir()); err != nil {
				s.Error("Failed to dump audio diagnostics: ", err)
			}
		}
	}(cleanupCtx)

	downloadsPath, err := cryptohome.DownloadsPath(ctx, chrome.User())
	if err != nil {
		s.Fatal("Cannot get Downloads path: ", err)
	}

	// Select the loopback devices
	if err = audio.SetupLoopback(ctx, chrome); err != nil {
		s.Fatal("Failed to setup loopback device: ", err)
	}

	tconn, err := chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch lacros-chrome: ", err)
	}
	defer l.Close(ctx)

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	conn, err := l.NewConn(ctx, server.URL+"/audio_capture_test.html")
	if err != nil {
		s.Fatal("Failed to open new tab: ", err)
	}
	defer conn.Close()

	var found bool
	s.Log("Check audio input")
	if err := conn.Call(ctx, &found, `async () => {
		let devices = await navigator.mediaDevices.enumerateDevices();
		return devices.some((dev) => dev.kind == 'audioinput');
	}`); err != nil {
		s.Fatal("Failed to check audio input: ", err)
	}

	if !found {
		s.Fatal("Failed to find audio input devices")
	}

	// Prepare output for recording.
	noiseWave := filepath.Join(s.OutDir(), "noise.wav")
	if err := testexec.CommandContext(
		ctx,
		"sox",
		"-n", "-L",
		"-e", "signed-integer",
		"-b", "16",
		"-r", strconv.Itoa(rate),
		"-c", "2",
		noiseWave,
		"synth", strconv.FormatFloat(playbackDuration.Seconds(), 'f', -1, 64),
		"sine", strconv.Itoa(goldenFrequency),
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Cannot generate ", playbackFileName, ": ", err)
	}

	playbackCaptureCtx, cancel := context.WithTimeout(ctx, 2*playbackDuration)
	defer cancel()

	playbackDone := make(chan struct{})
	go func() {
		defer close(playbackDone)
		// Run playback.
		if err := audio.PlayWavToPCM(playbackCaptureCtx, noiseWave, "hw:Loopback,0"); err != nil {
			s.Error("Cannot run playback: ", err)
		}
	}()

	s.Log("Start recording")
	if err := conn.Eval(ctx, "recordAudio()", nil); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	if err := conn.WaitForExpr(ctx, "recordFinished"); err != nil {
		s.Fatal("Failed to wait for recording finish: ", err)
	}

	// Download recorded file from webpage.
	if err := conn.Eval(ctx, "download()", nil); err != nil {
		s.Fatal("Failed to download recorded file: ", err)
	}

	// download() only triggers download, and will finish before the file is
	// completely downloaded.
	// Verify the file is downloaded before continuing.

	downloadConn, err := l.NewConn(ctx, downloadsURL)
	if err != nil {
		s.Fatal("Failed to open downloads page: ", err)
	}
	defer downloadConn.Close()

	ui := uiauto.New(tconn)
	downloadedFile := nodewith.NameStartingWith(recordedFileName).Role(role.Link)
	if err := ui.WaitUntilExists(downloadedFile)(ctx); err != nil {
		s.Fatal("Failed to find downloaded recording file: ", err)
	}
	if err := downloadConn.CloseTarget(ctx); err != nil {
		s.Fatal("Failed to close downloads page: ", err)
	}

	if err := fsutil.MoveFile(filepath.Join(downloadsPath, recordedFileName), filepath.Join(s.OutDir(), recordedFileName)); err != nil {
		s.Fatal("Cannot copy recorded files to results folder: ", err)
	}

	s.Log("Waiting for playback to complete")
	<-playbackDone

	// Verify recording correctness.
	if _, err := exec.Command("ffmpeg", "-i", filepath.Join(s.OutDir(), recordedFileName), filepath.Join(s.OutDir(), "test.wav")).Output(); err != nil {
		s.Fatal("Cannot convert format: ", err)
	}

	tone, err := audio.ReadS16LEPCM(filepath.Join(s.OutDir(), "test.wav"), 2)

	if err != nil {
		s.Fatal("Failed to read recording from file: ", err)
	}

	for channel := 0; channel < 2; channel++ {
		if err := audio.CheckFrequency(ctx, tone[channel], float64(rate), float64(goldenFrequency), 10, incorrectLimit); err != nil {
			s.Errorf("Channel %d frequency check failed: %v", channel+1, err)
		}
	}
}
