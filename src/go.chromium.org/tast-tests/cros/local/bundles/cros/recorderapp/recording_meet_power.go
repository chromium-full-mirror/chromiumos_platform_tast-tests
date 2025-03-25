// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/wav"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/recorderapp/data"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/input/voice"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/recorderapp"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	addBotTimeout = 1 * time.Minute
	meetTimeout   = 10 * time.Minute
	testDuration  = recorderapp.CUJRecordTime + power.RecorderTimeout + meetTimeout + 2*time.Minute
	cameraService = "cros-camera"
)

type recordingMeetPowerParam struct {
	launchConfig        recorderapp.LaunchConfig
	recordAudio         bool
	speechAudioDataPath string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         RecordingMeetPower,
		Desc:         "Collect power metrics when using Recorder App and Meet at the same time",
		Contacts:     []string{"chromeos-recorder-app@google.com", "hsuanling@google.com"},
		BugComponent: "b:1522466", // ChromeOS > Platform > Technologies > Audio > Recorder App
		VarDeps:      []string{"ui.bond_credentials"},
		Timeout:      testDuration,
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{data.ShortNewsAudio, data.ShortJapaneseNewsAudio},
		Params: []testing.Param{
			{
				Name:    "idle",
				Fixture: "powerAshGAIAWithRecorderAppAndAloopLoaded",
				Val: recordingMeetPowerParam{
					launchConfig:        recorderapp.LaunchConfig{},
					recordAudio:         false,
					speechAudioDataPath: data.ShortNewsAudio,
				},
			},
			{
				Name:    "record",
				Fixture: "powerAshGAIAWithRecorderAppAndAloopLoaded",
				Val: recordingMeetPowerParam{
					launchConfig:        recorderapp.LaunchConfig{},
					recordAudio:         true,
					speechAudioDataPath: data.ShortNewsAudio,
				},
			},
			{
				Name:    "record_transcription",
				Fixture: "powerAshGAIAWithRecorderAppAndAloopLoaded",
				Val: recordingMeetPowerParam{
					launchConfig:        recorderapp.LaunchConfig{TranscriptionForceEnabled: true},
					recordAudio:         true,
					speechAudioDataPath: data.ShortNewsAudio,
				},
				ExtraSoftwareDeps: []string{"ondevice_speech"},
			},
			{
				Name:    "record_japanese_transcription",
				Fixture: "powerAshGAIAWithRecorderAppAndAloopLoaded.japanese_transcription",
				Val: recordingMeetPowerParam{
					launchConfig: recorderapp.LaunchConfig{
						TranscriptionForceEnabled: true,
						TranscriptionLanguage:     recorderapp.JaJp,
					},
					recordAudio:         true,
					speechAudioDataPath: data.ShortJapaneseNewsAudio,
				},
				ExtraSoftwareDeps: []string{"ondevice_speech"},
			},
			{
				Name:    "record_speaker_label",
				Fixture: "powerAshGAIAWithRecorderAppAndAloopLoaded",
				Val: recordingMeetPowerParam{
					launchConfig: recorderapp.LaunchConfig{
						TranscriptionForceEnabled: true,
						SpeakerLabelForceEnabled:  true,
					},
					recordAudio:         true,
					speechAudioDataPath: data.ShortNewsAudio,
				},
				ExtraSoftwareDeps: []string{"ondevice_speech"},
			},
		},
	})
}

func RecordingMeetPower(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Setup for the test.
	// Setup CRAS Aloop for audio test.
	if err := voice.ActivateAloopNodes(ctx, tconn, voice.LoopbackCapture); err != nil {
		s.Fatal("Failed to load Aloop: ", err)
	}

	// Repeat the test audio to cover the test duration.
	tempDir, err := os.MkdirTemp("", "recorderapp-*")
	if err != nil {
		s.Fatal("Failed to create temporary directory for output: ", err)
	}
	defer os.RemoveAll(tempDir)
	extendedSpeechWav := filepath.Join(tempDir, "speech.wav")
	param := s.Param().(recordingMeetPowerParam)
	if err := wav.RepeatForDuration(ctx, s.DataPath(param.speechAudioDataPath), extendedSpeechWav, testDuration); err != nil {
		s.Fatal("Failed to prepare wav file: ", err)
	}

	// Setup fake camera HAL for Meet.
	if err := setupFakeCameraHAL(ctx); err != nil {
		s.Error("Failed to setup Fake HAL camera: ", err)
	}
	defer resetFakeCameraHAL(cleanupCtx)

	// Create a keyboard and mouse pointer for measuring input latency.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)
	pc := pointer.NewMouse(tconn)
	defer pc.Close(cleanupCtx)

	// Launch Recorder App and start recording if specified.
	s.Log("Launching Recorder App")

	setup := recorderapp.Setup{Config: param.launchConfig}
	app, err := recorderapp.StartAppWithSetup(ctx, cr, setup)
	if err != nil {
		s.Fatal("Failed to launch Recorder App: ", err)
	}
	defer app.Close(cleanupCtx)

	// Maximize the window to make sure waveform display is not hidden by transcription panel.
	if err := app.SetWindowStateAndWait(ctx, ash.WindowStateMaximized); err != nil {
		s.Error("Failed to maximize the Recorder App window: ", err)
	}

	if param.recordAudio {
		if err := app.StartRecording()(ctx); err != nil {
			s.Fatal("Failed to start recording audio: ", err)
		}
		defer func() {
			if err := app.StopRecording()(cleanupCtx); err != nil {
				s.Error("Failed to stop record: ", err)
			}
		}()
	}

	// Speak in the background.
	go func(ctx context.Context) {
		if err := audio.PlayWavToPCM(ctx, extendedSpeechWav, "hw:Loopback,0"); err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			s.Error("Failed to play wav to PCM: ", err)
		}
	}(ctx)

	// Start CUJ Recorder.
	recorder, err := cujrecorder.NewRecorder(ctx, tconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create the CUJ recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.AddCommonMetrics(); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	// Join a Meet session.
	s.Log("Joining a Meet session")

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		conn, err := cr.NewConn(ctx, chrome.NewTabURL)
		if err != nil {
			s.Fatal("Failed to open a new tab: ", err)
		}
		defer conn.Close()

		creds := s.RequiredVar("ui.bond_credentials")
		bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
		if err != nil {
			s.Fatal("Failed to create a bond client: ", err)
		}
		defer bc.Close()

		meetingCode, err := bc.CreateConference(ctx)
		if err != nil {
			s.Fatal("Failed to create a conference room: ", err)
		}

		botCtx, botCtxCancel := context.WithTimeout(ctx, addBotTimeout)
		defer botCtxCancel()
		_, _, err = bc.AddBots(botCtx, meetingCode, 1, meetTimeout+5*time.Minute)
		if err != nil {
			s.Fatal("Failed to add bot: ", err)
		}

		gm, err := googlemeet.JoinMeeting(ctx, cr, conn, meetingCode, map[string]string{}, googlemeet.WithAllPermissions)
		if err != nil {
			s.Fatal("Failed to join a meeting: ", err)
		}
		defer gm.Close(cleanupCtx)

		if err := setupGoogleMeet(ctx, cr, tconn, gm); err != nil {
			s.Error("Failed to setup Google Meet: ", err)
		}

		// Repeatedly perform keyboard action and AshWorkflows to measure input latency.
		for endTime := time.Now().Add(recorderapp.CUJRecordTime); time.Now().Before(endTime); {
			// Search in Meet page to measure keyboard latency.
			if uiauto.Combine("Search in the page",
				keyboard.AccelAction("Ctrl+f"),
				keyboard.TypeAction("The quick brown fox jumps over the lazy dog."),
				keyboard.AccelAction("Ctrl+a"),
				keyboard.AccelAction("Backspace"),
				keyboard.AccelAction("Esc"),
			)(ctx); err != nil {
				s.Fatal("Failed to search to exercise the keyboard: ", err)
			}

			if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
				s.Fatal("Failed to do Ash workflow: ", err)
			}
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	pv := perf.NewValues()
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the performance data: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save the performance data: ", err)
	}
}

func setupFakeCameraHAL(ctx context.Context) error {
	if err := testutil.SetupTestConfig(ctx, testutil.UseFakeHALCamera); err != nil {
		return errors.Wrap(err, "failed to set up camera test config")
	}
	if err := testutil.SetupFakeHALConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to setup Fake HAL config")
	}
	if err := upstart.RestartJob(ctx, cameraService); err != nil {
		return errors.Wrap(err, "failed to restart cros-camera after test config setup")
	}
	return nil
}

func resetFakeCameraHAL(ctx context.Context) {
	testutil.RemoveFakeHALConfig(ctx)
	testutil.RemoveTestConfig(ctx)
	upstart.RestartJob(ctx, cameraService)
}

func setupGoogleMeet(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, gm *googlemeet.GoogleMeet) error {
	// Setup DUT and bot cameras to specific resolution so the power results
	if err := uiauto.Combine("Configure Google Meet",
		gm.ChangeSettings(
			gm.SetSendResolution(googlemeet.ResolutionHD720P),
			gm.SetReceiveResolution(googlemeet.ResolutionHD720P),
		),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to configure Meet")
	}

	// Move the Meet window to right side of the screen.
	meetRE := regexp.MustCompile(`\bMeet\b|\bmeet\.\b`)
	meetWindow, err := ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return meetRE.MatchString(w.Title) })
	if err != nil {
		return errors.Wrap(err, "failed to find the Meet window")
	}

	// Maximize the window size to fit typical user behavior.
	if err := ash.SetWindowStateAndWait(ctx, tconn, meetWindow.ID, ash.WindowStateMaximized); err != nil {
		return errors.Wrap(err, "failed to maximize the Meet window")
	}
	return nil
}
