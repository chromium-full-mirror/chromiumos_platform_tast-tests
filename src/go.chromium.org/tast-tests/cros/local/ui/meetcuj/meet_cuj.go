// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package meetcuj contains the test code for Meet/MeetMultitasking CUJ.
package meetcuj

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/async"
	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/effects"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// DefaultTestTimeout is the default timeout for the whole test.
	DefaultTestTimeout = 25 * time.Minute
	// DefaultMeetTimeout is the default timeout for the Meet session.
	DefaultMeetTimeout = 10 * time.Minute
	// longUITimeout is the timeout for UI interactions.
	longUITimeout = time.Minute
	// FakeCameraVideoFile720p is the fake camera file.
	FakeCameraVideoFile720p = "camera_video_720p.y4m"

	presentTabTitle = "Untitled document"

	// ExpectedMeetWindowWidth is the minimum pixel width to render
	// 16 participant grids in Google Meet without hiding feeds.
	// This value was determined by testing various zoom ratios across multiple devices to find
	// the minimum effective width = (window width) / (zoom ratio) that passes on all devices.
	ExpectedMeetWindowWidth float64 = 1250
)

// FakeCameraHALCfg defines parameters that are used to generate the fake
// camera HAL config.
type FakeCameraHALCfg struct {
	// VideoFileName is the video used by the fake HAL to simulate the
	// real camera input.
	VideoFileName string
	// Formats defines the supported resolution / frame rates of the fake
	// camera HAL. Fake HAL will use whatever resolution / frame rates passed
	// down from upper layer, and this config only affects what's reported in
	// the camera static metadata.
	Formats []*testutil.FakeCameraFormatsConfig
}

// MeetTest specifies the setting of a Google Meet journey. More info at go/cros-meet-tests.
type MeetTest struct {
	// bots is an increasing list of bot counts that should be in the call
	// during the test. There should be at least one value in this list, and
	// the first bot count value has to be greater than 0, to account for the
	// spotlight bot that is in every test.
	Bots []int

	Layout              googlemeet.LayoutOption // Type of the layout in the meeting.
	Enterprise          bool                    // Whether to use enterprise accounts.
	Present             bool                    // Whether it is presenting the Google Docs window.
	Docs                bool                    // Whether it is running with a Google Docs window.
	Slides              bool                    // Whether it is running with a Google Slides window.
	Sheets              bool                    // Whether it is running with a Google Sheets window.
	Split               bool                    // Whether it is in split screen mode. It can not be true if docs is false.
	Cam                 bool                    // Whether the camera is on or not.
	Effects             bool                    // Whether to turn on visual effects within Meet.
	BackgroundBlur      bool                    // Whether to turn on platform-level background blur.
	AdjustLighting      bool                    // Whether to turn on the platform-level adjust lighting feature.
	Retouch             bool                    // Whether to turn on the platform-level face retouch feature.
	NpuInference        bool                    // Whether to use NPU as inference backend for platform-level effects.
	LiveCaptions        bool                    // Whether to turn on live captioning.
	StudioStyleMic      bool                    // Whether to turn on studio style mic (formerly noise cancellation).
	StudioMic           bool                    // Whether to turn on studio mic.
	ZoomOut             bool                    // Whether to zoom out on both the browser and display.
	TabSwitchDocs       bool                    // Whether to switch between Docs and Meet. It cannot be true if docs is false.
	Duration            time.Duration           // Duration of the meet call. Must be less than test timeout.
	TypingDuration      time.Duration           // Duration of typing on Google Docs. Must be less than the duration of the meet call. If |typingDuration| is not given, it defaults to |meetTimeout|.
	BotsOptions         []bond.AddBotsOption    // Customizes the meeting participant bots.
	FakeCamHALCfg       *FakeCameraHALCfg       // Enable Fake Camera HAL if the config is present.
	MeasureEcho         bool                    // Whether to measure the echo RMS. The number of meeting participant bot must be one and should be enabled with human speech as the only audio source (no other noise) to accurately evaluate the echo RMS.
	DisabledExperiments []string                // List of experiments to disable with the e= parameter in the Meet URL.
	TabsForOverview     []string                // List of tabs we want to open and trigger overview. If empty, overview won't be triggered.
}

// FakeCamHALCfg720p is the fake camera HAL used in MeetCUJ.
var FakeCamHALCfg720p = &FakeCameraHALCfg{
	// The video file used to simulate camera input.
	VideoFileName: FakeCameraVideoFile720p,
	// Resolutions / frame rates that the fake HAL supports.
	Formats: []*testutil.FakeCameraFormatsConfig{{
		Width:      320,
		Height:     180,
		FrameRates: []int{30}}, {
		Width:      640,
		Height:     360,
		FrameRates: []int{30}}, {
		Width:      1280,
		Height:     720,
		FrameRates: []int{30}},
	},
}

// Run runs MeetCUJ which measures the performance of critical user journeys for Google Meet.
// Journeys for Google Meet are specified by testing parameters.
//
// Pre-preparation:
//   - Open a Meet window.
//   - Create and enter the meeting code.
//   - Open a Google Docs window (if necessary).
//   - Enter split mode (if necessary).
//   - Turn off camera (if necessary).
//
// During recording:
//   - Join the meeting.
//   - Add participants(bots) to the meeting.
//   - Set up the layout.
//   - Max out the number of the maximum tiles (if necessary).
//   - Start to present (if necessary).
//   - Input notes to Google Docs file (if necessary).
//   - Navigate to Google Slides and input notes to file (if necessary).
//   - Navigate to Google Sheets and input notes to file (if necessary).
//   - Wait for 30 seconds before ending the meeting.
//
// After recording:
//   - Record and save metrics.
func Run(ctx context.Context, meet MeetTest, cr *chrome.Chrome, testCaseVar func(string) (string, bool), dataPath func(string) string, outDir, creds string) (pv *perf.Values, retErr error) {
	const (
		// The addBotTimeout allows 3 2-minute BondAPI request retries by the
		// Bond lib.
		addBotTimeout  = 6*time.Minute + 10*time.Second
		defaultDocsURL = "https://docs.new/"
		newTabTitle    = "New Tab"
	)

	notes := strings.Split("Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.", "")

	if meet.TabSwitchDocs && !meet.Docs {
		return pv, errors.New("cannot tab switch docs without opening a Google Doc")
	}
	if len(meet.Bots) == 0 {
		return pv, errors.New("must have at least 1 bot count")
	}
	if meet.Bots[0] == 0 {
		return pv, errors.New("first bot count must have at least 1 bot, to add the spotlight bot")
	}

	// Determines the Meet call duration. Use the Meet duration specified in
	// test param if there is one. Otherwise, default to 10 minutes.
	meetTimeout := DefaultMeetTimeout
	if meet.Duration != 0 {
		meetTimeout = meet.Duration
	}

	if testDuration, ok := testCaseVar("ui.MeetCUJ.duration"); ok {
		var err error
		meetTimeout, err = time.ParseDuration(testDuration)
		if err != nil {
			return pv, errors.Wrapf(err, "failed to parse command-line arg ui.MeetCUJ.duration=%q", testDuration)
		}
	}

	testing.ContextLog(ctx, "Run meeting for ", meetTimeout)

	// Shorten context to allow for cleanup. Reserve one minute in case of power
	// test.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	if meet.FakeCamHALCfg != nil {
		resetFakeCameraHAL, err := SetupFakeCameraHAL(ctx, dataPath(meet.FakeCamHALCfg.VideoFileName), meet.FakeCamHALCfg.Formats)
		if err != nil {
			return pv, errors.Wrap(err, "failed to setup Fake HAL camera")
		}
		defer resetFakeCameraHAL(closeCtx)
	}

	if meet.MeasureEcho {
		testing.ContextLog(ctx, "setup POST_DSP_DELAYED_LOOPBACK")
		cras, err := audio.NewCras(ctx)
		if err != nil {
			return pv, errors.Wrap(err, "failed to connect to CRAS")
		}

		if err := cras.SetActiveNodeByType(ctx, "POST_DSP_DELAYED_LOOPBACK"); err != nil {
			return pv, errors.Wrap(err, "failed to set active node POST_DSP_DELAYED_LOOPBACK")
		}
	}

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return pv, errors.Wrap(err, "failed to capture device snapshot")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return pv, errors.Wrap(err, "failed to connect to the test API connection")
	}

	if meet.ZoomOut {
		// Sets the display zoom factor to minimum, to ensure that all
		// meeting participants' video can be shown simultaneously.
		// The display zoom should be done before the Meet window is
		// opened, to prevent visual oddities on some devices. We also
		// zoom out on the browser after the Meet window is opened,
		// because on some boards the display zoom is not enough to
		// show all of the participants.
		revertZoom, err := display.MinimizePrimaryDisplayZoomFactor(ctx, tconn)
		if err != nil {
			return pv, errors.Wrap(err, "failed to set the zoom factor of the primary display to minimum")
		}
		defer revertZoom(closeCtx, tconn)
	}

	bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
	if err != nil {
		return pv, errors.Wrap(err, "failed to create a bond client")
	}
	defer bc.Close()

	startTime := time.Now()
	meetingCode, err := bc.CreateConference(ctx)
	if err != nil {
		return pv, errors.Wrap(err, "failed to create a conference room")
	}
	createDuration := time.Since(startTime)
	pv.Set(perf.Metric{
		Name:      "TPS.Meet.CreateDuration",
		Unit:      "s",
		Direction: perf.SmallerIsBetter,
	}, createDuration.Seconds())

	testing.ContextLogf(ctx, "Created a room with the code %s in %f seconds", meetingCode, createDuration.Seconds())

	sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
	defer cancel()
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Removing all bots from the call")
		if _, _, err := bc.RemoveAllBots(ctx, meetingCode); err != nil {
			testing.ContextLog(ctx, "Failed to remove all bots: ", err)
		}
	}(closeCtx)

	// Create a bot with spotlight layout to request HD video.
	spotlightBotList, _, err := bc.AddBots(sctx, meetingCode, 1, meetTimeout+30*time.Minute, append(meet.BotsOptions, bond.WithLayout("SPOTLIGHT"))...)
	if err != nil {
		return pv, errors.Wrap(err, "failed to create bot with spotlight layout")
	}
	if len(spotlightBotList) != 1 {
		return pv, errors.Wrapf(err, "unexpected number of bots with spotlight layout successfully started; got %d, expected 1", len(spotlightBotList))
	}

	// Keep track of how many bots are already in the call, so we can add the
	// right number of bots later in the test.
	botsInCall := 1

	numBotsToAdd := meet.Bots[0] - botsInCall
	if err := AddBots(ctx, bc, numBotsToAdd, meetTimeout, meetingCode, meet.BotsOptions); err != nil {
		return pv, errors.Wrapf(err, "failed to initially add %d bots", numBotsToAdd)
	}
	botsInCall += numBotsToAdd

	tabChecker, err := cuj.NewTabCrashChecker(ctx, tconn)
	if err != nil {
		return pv, errors.Wrap(err, "failed to create TabCrashChecker")
	}

	// Ensure that even if the Meet call crashes outside of our direct
	// crash checks, we still log that the crash happened.
	caughtTabCrash := false
	defer func(ctx context.Context) {
		if caughtTabCrash {
			return
		}
		if err := tabChecker.Check(ctx); err != nil {
			testing.ContextLog(ctx, "Tab renderer crashed")
		}
	}(ctx)

	assertTabActive := func(ctx context.Context) error {
		if err := tabChecker.Check(ctx); err != nil {
			caughtTabCrash = true
			return errors.Wrap(err, "tab renderer crashed")
		}
		return nil
	}

	meetHelper := googlemeet.NewHRTelemetryHelper(cr, tconn)
	recorder, err := cujrecorder.NewRecorder(ctx, tconn, cr, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		return pv, errors.Wrap(err, "failed to create the recorder")
	}

	if err := recorder.AddCollectedMetrics(
		cujrecorder.NewCustomMetricConfig("Cras.MissedCallbackFrequencyInput", "millisecond", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("Cras.MissedCallbackFrequencyOutput", "millisecond", perf.SmallerIsBetter)); err != nil {
		return pv, errors.Wrap(err, "failed to add metrics to recorder")
	}

	if err := recorder.AddCommonMetrics(); err != nil {
		return pv, errors.Wrap(err, "failed to add common metrics to recorder")
	}

	// Take a screenshot every 2 minutes up to a maximum of 5
	// screenshots, to ensure we capture any bots that drop during
	// the call and any issues that come up with the collab window.
	if err := recorder.AddScreenshotRecorder(ctx, 2*time.Minute, 5); err != nil {
		testing.ContextLog(ctx, "Failed to add screenshot recorder")
	}

	defer func() {
		if err := recorder.Close(closeCtx); err != nil {
			testing.ContextLog(ctx, "Failed to stop recorder: ", err)
		}
	}()

	// Open chrome://webrtc-internals now so it will collect data on the meeting's streams.
	webrtcInternals, err := recorder.NewConn(ctx, cr, "WebRTC_Internals", "chrome://webrtc-internals", browser.WithNewWindow())
	if err != nil {
		return pv, errors.Wrap(err, "failed to open chrome://webrtc-internals")
	}
	defer webrtcInternals.Close()

	webRTCInternalsWindow, err := ash.FindOnlyWindow(ctx, tconn, ash.BrowserTitleMatch("WebRTC Internals"))
	if err != nil {
		return pv, errors.Wrap(err, "failed to find the WebRTC Internals window")
	}

	// Maximize the WebRTC Internals window now, in preparation to take
	// a screenshot of it if DumpWebRTCInternals fails at the end of
	// the test. That screenshot is for investigation of b/255343902.
	// TODO(b/255343902): Remove this when the bug is fixed.
	if err := ash.SetWindowStateAndWait(ctx, tconn, webRTCInternalsWindow.ID, ash.WindowStateMaximized); err != nil {
		testing.ContextLog(ctx, "Failed to ensure that the WebRTC Internals window is maximized: ", err)
	}

	if meet.Docs {
		// Ensure docs offline support is enabled to avoid docs page hitting
		// fatal network error. See http://b/254914987
		if err := cuj.EnsureDocsOfflineEnabled(ctx, cr); err != nil {
			return pv, errors.Wrap(err, "failed to enable docs offline support")
		}
	}

	// Ensure that we close the Meet window at the end of the test in case
	// the test fails. Closing the Meet window should occur after dumping
	// the UI tree, so the UI dump actually contains the proper failure.
	closedMeet := false
	defer func() {
		if closedMeet {
			return
		}
		// Close the windows to finish the meeting.
		if err := ash.CloseAllWindows(closeCtx, tconn); err != nil {
			testing.ContextLog(ctx, "Failed to close all windows: ", err)
		}
	}()

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")

	// Autorelease the automation tree when it is not used so that excessive
	// automation events runs do not consume too much cpu/power.
	// (This was originally added for Lacros in the context of b/278649596
	// but removing it now might regress the benchmark.)
	automationAutoRelease, err := uiauto.NewScopedAutoRelease(ctx, tconn)
	if err != nil {
		return pv, errors.Wrap(err, "failed to create automation ScopedAutoRelease")
	}
	defer automationAutoRelease.Reset(ctx)

	if err := cuj.ExpandCreateDumpSection(ctx, tconn); err != nil {
		return pv, errors.Wrap(err, "failed to expand Create Dump section of chrome://webrtc-internals")
	}

	if meet.MeasureEcho {
		if err := cuj.DumpDiagnosticAudioRecordings(ctx, tconn); err != nil {
			return pv, errors.Wrap(err, "failed to enable audio recordings from chrome://webrtc-internals")
		}
		downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
		if err != nil {
			return pv, errors.Wrap(err, "failed to get Downloads path")
		}
		defer cuj.CleanupDiagnosticAudioRecordings(ctx, downloadsPath)
	}

	var names []string
	for name := range cujrecorder.WebRTCMetricInfo {
		names = append(names, name)
	}
	webRTCMetricsRecorder, err := metrics.StartRecorder(ctx, tconn, names...)
	if err != nil {
		return pv, errors.Wrap(err, "failed to start recording WebRTC metrics")
	}

	if len(meet.DisabledExperiments) == 0 {
		if err := meetHelper.JoinMeeting(ctx, meetingCode, browser.WithNewWindow()); err != nil {
			return pv, errors.Wrap(err, "failed to open the hangout meet website")
		}
	} else {
		if err := meetHelper.JoinMeetingWithDisabledExperiments(ctx, meetingCode, meet.DisabledExperiments, browser.WithNewWindow()); err != nil {
			return pv, errors.Wrap(err, "failed to open the hangout meet website with disabled experiments")
		}
	}
	defer meetHelper.Close(closeCtx)

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	testing.ContextLogf(ctx, "Is in tablet-mode: %t", inTabletMode)
	if err != nil {
		return pv, errors.Wrap(err, "failed to detect it is in tablet-mode or not")
	}
	var pc pointer.Context
	var mw *input.MouseEventWriter
	if inTabletMode {
		// If it is in tablet mode, ensure it it in landscape orientation.
		// TODO(crbug/1135239): test portrait orientation as well.
		orientation, err := display.GetOrientation(ctx, tconn)
		if err != nil {
			return pv, errors.Wrap(err, "failed to get display orientation")
		}
		if orientation.Type == display.OrientationPortraitPrimary {
			info, err := display.GetPrimaryInfo(ctx, tconn)
			if err != nil {
				return pv, errors.Wrap(err, "failed to get the primary display info")
			}
			testing.ContextLog(ctx, "Rotating display 90 degrees")
			if err := display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate90); err != nil {
				return pv, errors.Wrap(err, "failed to rotate display")
			}
			defer display.SetDisplayRotationSync(closeCtx, tconn, info.ID, display.Rotate0)
		}
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			return pv, errors.Wrap(err, "failed to create a touch controller")
		}
	} else {
		// Make it into a maximized window if it is in clamshell-mode.
		if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
			return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
		}); err != nil {
			return pv, errors.Wrap(err, "failed to turn all windows into maximized state")
		}
		pc = pointer.NewMouse(tconn)

		mw, err = input.Mouse(ctx)
		if err != nil {
			return pv, errors.Wrap(err, "failed to create a mouse")
		}
		defer mw.Close(ctx)
	}
	defer pc.Close(ctx)

	// Match window titles `Google Meet` and `meet.google.com`.
	meetRE := regexp.MustCompile(`\bMeet\b|\bmeet\.\b`)
	meetWindow, err := ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return meetRE.MatchString(w.Title) })
	if err != nil {
		return pv, errors.Wrap(err, "failed to find the Meet window")
	}

	kw, err := input.Keyboard(ctx)
	if err != nil {
		return pv, errors.Wrap(err, "failed to create a keyboard")
	}
	defer kw.Close(closeCtx)

	// Find the web view of Meet window.
	webview := nodewith.ClassName("ContentsWebView").Role(role.WebView)

	// Check and grant permissions.
	if err := GrantPermissionsInMeet(tconn)(ctx); err != nil {
		return pv, errors.Wrap(err, "failed to grant permissions")
	}

	// Ensure to check that we are properly in the meeting before trying
	// to apply visual effects.
	if err := meetHelper.IsInMeeting(ctx, time.Minute); err != nil {
		return pv, errors.Wrap(err, "failed to wait to enter the meeting")
	}

	ui := uiauto.New(tconn)
	if err := WaitForParticipantInfoLoaded(ui)(ctx); err != nil {
		return pv, errors.Wrap(err, "failed to wait for participant info")
	}

	expectedParticipantCount := botsInCall + 1
	if err := meetHelper.CheckParticipantCount(ctx, expectedParticipantCount); err != nil {
		return pv, errors.Wrap(err, "the number of bots is unexpected")
	}

	// Make sure the default camera is turned on.
	if err := meetHelper.SetCamera(ctx, true); err != nil {
		return pv, errors.Wrap(err, "failed to turn on camera")
	}

	// Hide notifications so that they won't overlap with other UI components.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		return pv, errors.Wrap(err, "failed to close all notifications")
	}

	if meet.Effects {
		testing.ContextLog(ctx, "Turn on visual effects")
		if err := SetVisualEffects(ui, BlurBackgroundFinder)(ctx); err != nil {
			return pv, errors.Wrap(err, "failed to turn on visual effects")
		}
	} else {
		if err := SetVisualEffects(ui, TurnOffEffectsFinder)(ctx); err != nil {
			return pv, errors.Wrap(err, "failed to turn off visual effects")
		}
	}

	if meet.LiveCaptions ||
		meet.AdjustLighting ||
		meet.BackgroundBlur ||
		meet.Retouch ||
		meet.StudioStyleMic ||
		meet.StudioMic ||
		meet.NpuInference {
		testing.ContextLog(ctx, "Toggling platform VC effects")
		vct := vctray.New(ctx, tconn)
		blur := vctray.BackgroundBlurOff
		blurLevel := effects.KBlurDisabled
		if meet.BackgroundBlur {
			blur = vctray.BackgroundBlurFull
			blurLevel = effects.KBlurMaximum
		}
		if err := vct.ChangeSettingsInPanel(
			vct.SetLiveCaption(meet.LiveCaptions),
			vct.SetBackgroundBlur(blur),
			vct.SetStudioLookEffects(meet.AdjustLighting, meet.Retouch),
		)(ctx); err != nil {
			return pv, errors.Wrap(err, "failed to configure platform VC effects")
		}
		inference := effects.KInferenceDefault
		if meet.NpuInference {
			inference = effects.KInferenceNpu
		}
		// Passes inference backend.
		effects.ApplyPlatformEffects(ctx, meet.AdjustLighting, meet.Retouch, blurLevel, effects.KAuto, inference)
		// Studio mic and studio style mic(noise cancellation) don't appear at the same time.
		if meet.StudioMic {
			if err := vct.ChangeSettingsInPanel(vct.SetStudioMic(meet.StudioMic))(ctx); err != nil {
				return pv, errors.Wrap(err, "failed to configure platform VC effects: studio mic")
			}
		} else if meet.StudioStyleMic {
			if err := vct.ChangeSettingsInPanel(vct.SetStudioStyleMic(meet.StudioStyleMic))(ctx); err != nil {
				return pv, errors.Wrap(err, "failed to configure platform VC effects: studio style mic")
			}
		}

	}

	if err := ResetZoom(ui, kw)(ctx); err != nil {
		return pv, errors.Wrap(err, "failed to press Ctrl+0 to reset the zoom")
	}

	if meet.ZoomOut {
		meetWindowWidth := float64(meetWindow.BoundsInRoot.Width)
		if meet.Split {
			meetWindowWidth /= 2
		}

		// Zoom out on the browser to maximize the number of visible video
		// feeds. This needs to be done before the final layout mode has been set,
		// so that Meet can properly recalculate how many inbound videos should
		// be visible.
		if err := SetBrowserZoomToFitWidth(ctx, kw, ui, meetWindowWidth, ExpectedMeetWindowWidth); err != nil {
			return pv, errors.Wrap(err, "failed to set browser zoom to fit the expected width")
		}
	}

	// Make sure the Meet call window hasn't crashed before starting the recorder.
	if err := assertTabActive(ctx); err != nil {
		return pv, err
	}

	var cleanUpDoc bool
	var docsHref string
	// Shorten the context to cleanup document.
	// Some low-end devices take a long time to delete docs, so extend
	// timeout to one minute.
	cleanUpDocCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if cleanUpDoc && docsHref != "" {
			faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return retErr != nil }, cr, "cleanup_doc")
			if err := googledocs.DeleteDocWithURL(tconn, cr, docsHref)(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to delete doc: ", err)
			}
		}
	}(cleanUpDocCtx)

	if err := recorder.Run(ctx, func(ctx context.Context) (retErr error) {
		// Open up the collab window inside the recorder to collect
		// PageLoad.PaintTiming.NavigationToFirstContentfulPaint.
		var collaborationRE *regexp.Regexp
		var collaborationConn *chrome.Conn
		var collaborationWindow *ash.Window

		if meet.Docs {
			recorder.Annotate(ctx, "Open_Google_Doc")
			docsURL := defaultDocsURL
			if docsURLOverride, ok := testCaseVar("ui.MeetCUJ.doc"); ok {
				docsURL = docsURLOverride
			}

			// Create another browser window and open a Google Docs file.
			collaborationConn, err = recorder.NewConn(ctx, cr, "Docs", docsURL, browser.WithNewWindow())
			if err != nil {
				return errors.Wrap(err, "failed to open the Google Docs website")
			}
			defer collaborationConn.Close()

			// Wait for tab quiescene to give any network requests time to
			// complete before forcing Google Docs offline. Only log the error,
			// because sometimes reaching quiescence can take a really long
			// time, even when the doc is interactable.
			if err := webutil.WaitForQuiescence(ctx, collaborationConn, time.Minute); err != nil {
				testing.ContextLog(ctx, "Failed to wait for Google Docs to quiesce: ", err)
			}

			if docsURL == defaultDocsURL {
				if err := collaborationConn.Eval(ctx, "window.location.href", &docsHref); err != nil {
					return errors.Wrap(err, "failed to get Docs URL")
				}
				cleanUpDoc = true
			}

			collaborationRE = regexp.MustCompile(`\bDocs\b`)

			// Enable docs blocker extension to force Docs in offline mode after docs
			// is loaded.
			docsBlockerConn, err := cuj.GetDocsBlockerConn(ctx, cr)
			if err != nil {
				return errors.Wrap(err, "failed to get docs blocker conn")
			}
			defer docsBlockerConn.Close()

			if err := docsBlockerConn.Eval(ctx, "ForceDocsOffline(true)", nil); err != nil {
				return errors.Wrap(err, "failed to call docs blocker to block Docs")
			}
			docsBlockerCleanupCtx := ctx
			ctx, cancel = ctxutil.Shorten(ctx, 15*time.Second)
			defer cancel()
			defer func(ctx context.Context) {
				if err := docsBlockerConn.Eval(ctx, "ForceDocsOffline(false)", nil); err != nil {
					testing.ContextLog(ctx, "Failed to call docs blocker to restore: ", err)
				}
			}(docsBlockerCleanupCtx)

			if err := googledocs.ShowTheDocMenus(tconn, kw)(ctx); err != nil {
				return errors.Wrap(err, "failed to show the doc menus")
			}

			collaborationWindow, err = ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return collaborationRE.MatchString(w.Title) })
			if err != nil {
				return errors.Wrap(err, "failed to find the collaboration window")
			}
		}

		if meet.Split {
			// Start an annotation section for split screening each window.
			endSplitScreenSection := recorder.AnnotateSection(ctx, "Split_screen_windows")
			if collaborationRE == nil {
				return errors.New("need a collaboration window for split view")
			}
			if err := ash.SetWindowStateAndWait(ctx, tconn, collaborationWindow.ID, ash.WindowStatePrimarySnapped); err != nil {
				return errors.Wrap(err, "failed to snap the collaboration window to the left")
			}
			if err := ash.SetWindowStateAndWait(ctx, tconn, meetWindow.ID, ash.WindowStateSecondarySnapped); err != nil {
				return errors.Wrap(err, "failed to snap the Meet window to the right")
			}
			endSplitScreenSection(ctx)
		} else {
			if err := meetWindow.ActivateWindow(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to activate the Meet window")
			}
		}

		shareMessage := "Share this info with people you want in the meeting"
		if err := ui.WaitUntilExists(nodewith.Name(shareMessage).Ancestor(webview))(ctx); err == nil {
			// "Share this code" popup appears, dismissing by close button.
			if err := uiauto.Combine(
				"click the close button and wait for the popup to disappear",
				pc.Click(nodewith.Name("Close").Role(role.Button).Ancestor(webview)),
				ui.WaitUntilGone(nodewith.Name(shareMessage).Ancestor(webview)),
			)(ctx); err != nil {
				return err
			}
		}
		clearPromptsChannel := make(chan error)
		dismissPromptIfExists(ctx, tconn, clearPromptsChannel)

		if err := meetHelper.SetMicrophone(ctx, true); err != nil {
			return errors.Wrap(err, "failed to turn on microphone")
		}

		if err := meetHelper.SetCamera(ctx, meet.Cam); err != nil {
			return errors.Wrapf(err, "failed to set camera off-status to %t", !meet.Cam)
		}

		if err := meetHelper.ChangeLayoutOption(ctx, meet.Layout); err != nil {
			return errors.Wrapf(err, "failed to set %s layout", meet.Layout)
		}

		if err := meetHelper.SetSendResolution720p(ctx); err != nil {
			return errors.Wrap(err, "failed to request sending 720p")
		}

		if err := meetHelper.SetReceiveResolution720p(ctx); err != nil {
			return errors.Wrap(err, "failed to request receiving 720p")
		}

		isPresenting := false
		presentingCleanupCtx := ctx
		ctx, cancel = ctxutil.Shorten(ctx, 15*time.Second)
		defer cancel()
		defer func(ctx context.Context) {
			if isPresenting {
				if err := meetHelper.StopPresenting(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to stop presenting: ", err)
				}
				isPresenting = false
			}
		}(presentingCleanupCtx)

		if meet.Present {
			if !meet.Docs {
				return errors.New("need a Google Docs tab to present")
			}

			// Start an annotation section for opening the screen share window
			// and screen sharing the collaboration window.
			endPresentSection := recorder.AnnotateSection(ctx, "Screenshare")

			if err := meetHelper.PresentTab(ctx, collaborationConn, kw, presentTabTitle); err != nil {
				return errors.Wrap(err, "failed to start screen sharing")
			}
			isPresenting = true
			expectedParticipantCount++
			dismissPromptIfExists(ctx, tconn, clearPromptsChannel)

			endPresentSection(ctx)
		}

		errc := make(chan error)
		testing.ContextLog(ctx, "Keeping the meet session for ", meetTimeout)
		async.Run(ctx, func(ctx context.Context) {
			// Using goroutine to measure GPU counters asynchronously because:
			// - we will add some other test scenarios (controlling windows / meet sessions).
			// - graphics.MeasureGPUCounters may quit immediately when the hardware or
			//   kernel does not support the reporting mechanism.
			errc <- graphics.MeasureGPUCounters(ctx, meetTimeout, pv)
		}, "measure GPU counters")

		var addBotsErr error
		stopAddBotsC := make(chan struct{})
		defer func(ctx context.Context) {
			close(stopAddBotsC)
			if addBotsErr != nil {
				retErr = errors.Wrapf(retErr, "failed to run bot phases during the Meet call: %v", addBotsErr)
			}
		}(ctx)

		var stopSnapshot func(ctx context.Context) error
		numPhases := len(meet.Bots)
		// If there are multiple phases, collect a shortened list of UMA
		// metrics for each phase.
		ashMetrics, browserMetrics := cujrecorder.GetShortenedPerformanceMetrics()
		async.Run(ctx, func(ctx context.Context) {
			if numPhases == 1 {
				return
			}

			// The call will be split into len(meet.Bots) phases.
			// Each phase i will have meet.Bots[i] number of bots in the call.
			// At this point in the test, the first set of bots have already
			// been added.
			currentPhase := 0
			startBotAddTime := time.Now()

			// The test is broken up into equal length phases with different
			// bot counts.
			phaseDuration := meetTimeout / time.Duration(numPhases)

			addingMoreBots := true
			for addingMoreBots {
				// Prefix the metric with the number of people in the call.
				// This is the number of bots in the call + the user themselves.
				stopSnapshot, err = recorder.StartSnapshot(ctx, fmt.Sprintf("%dp", botsInCall+1), ashMetrics, browserMetrics)
				if err != nil {
					addBotsErr = errors.Wrapf(err, "failed to start snapshot for phase %d", currentPhase)
					break
				}

				// End the goroutine early if we added the bots for the
				// last phase.
				if currentPhase == numPhases-1 {
					addingMoreBots = false
					break
				}

				select {
				// Subtract the time it took to add the bots from the phase
				// duration. For the first bot phase, the bots are added before
				// recorder.Run, so we subtract a negligible amount of time
				// from the phase duration.
				case <-time.After(phaseDuration - time.Since(startBotAddTime)):
					// Complete the snapshot of the last phase.
					if err := stopSnapshot(ctx); err != nil {
						addBotsErr = errors.Wrapf(err, "failed to stop snapshot for phase %d", currentPhase)
					}
					stopSnapshot = nil

					// Take a screenshot of the previous phase.
					recorder.CustomScreenshot(ctx)

					startBotAddTime = time.Now()

					currentPhase++
					numBotsToAdd = meet.Bots[currentPhase] - botsInCall

					if err := AddBots(ctx, bc, numBotsToAdd, meetTimeout, meetingCode, meet.BotsOptions); err != nil {
						addBotsErr = errors.Wrapf(err, "failed to add %d bots", numBotsToAdd)
						addingMoreBots = false
						break
					}
					botsInCall += numBotsToAdd
					recorder.Annotate(ctx, fmt.Sprintf("Added_%d_bots", numBotsToAdd))

					if expectedParticipantCount <= 2 {
						dismissPromptIfExists(ctx, tconn, clearPromptsChannel)
					}

					// Ensure to properly keep track of how many participants
					// are in the call, which would include the current user
					// and the optional screensharing connection.
					expectedParticipantCount += numBotsToAdd

				case <-stopAddBotsC:
					testing.ContextLog(ctx, "add_bots: Background signaled to stop")
					addBotsErr = errors.Errorf("failed to complete phase %d, background signaled to stop", currentPhase)
					addingMoreBots = false
				}
			}
		}, "increasing bot count during test")
		defer func(ctx context.Context) {
			if stopSnapshot == nil {
				return
			}
			if err := stopSnapshot(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to stop final snapshot: ", err)
			}
		}(ctx)

		// Record trace for 30 seconds.
		// See go/trace-in-cuj-tests about rules for tracing.
		var tracingErr error
		traceDuration := 30 * time.Second
		stopTracingC := make(chan struct{})
		defer func(ctx context.Context) {
			close(stopTracingC)
			if tracingErr != nil {
				retErr = errors.Wrapf(retErr, "failed to complete tracing: %v", tracingErr)
			}
		}(ctx)

		startTracingRoutine := func(ctx context.Context) {
			async.Run(ctx, func(ctx context.Context) {
				if err := recorder.StartTracing(ctx, outDir, dataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
					tracingErr = errors.Wrap(err, "failed to start tracing")
					return
				}

				for {
					select {
					case <-time.After(traceDuration):
						if err := recorder.StopTracing(ctx); err != nil {
							tracingErr = errors.Wrap(err, "failed to stop tracing")
						}
						return
					case <-stopTracingC:
						testing.ContextLog(ctx, "tracing: Background signaled to stop")
						tracingErr = errors.New("failed to complete tracing, background signaled to stop")
						return
					}
				}
			}, /*prefix=*/ "Tracing")
		}

		meetEndTime := time.Now().Add(meetTimeout)
		if meet.Docs {
			// Start an annotation section to interact with Google Docs.
			endDocsInteractions := recorder.AnnotateSection(ctx, "Docs_interactions")

			// Since adding bots also takes snapshots, to avoid collision, only start
			// snapshot for Google Docs if the number of bots won't vary during
			// meetTimeout.
			if numPhases == 1 {
				stopSnapshot, err = recorder.StartSnapshot(ctx, "Docs", ashMetrics, browserMetrics)
				if err != nil {
					return errors.Wrap(err, "failed to start snapshot for Google Docs")
				}
			}

			if err := collaborationWindow.ActivateWindow(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to activate the collaboration window")
			}

			// The UI elements might not immediately appear after activating the window.
			// Wait for the web area of the Google Docs website to appear
			// to ensure the security alert can be correctly dismissed.
			if err := ui.WaitUntilExists(googledocs.DocsWebArea)(ctx); err != nil {
				return errors.Wrap(err, "failed to wait for docs root web area to appear")
			}
			if err := cuj.DismissCriticalSecurityAlert(ctx, tconn, collaborationConn); err != nil {
				return errors.Wrap(err, "failed to dismiss critical security alert")
			}
			// There may be multiple canvases in the docs root area, so add First() here.
			docsCanvas := nodewith.Role(role.Canvas).Ancestor(googledocs.DocsWebArea).First()
			if err := action.Combine("select and zoom document",
				pc.Click(docsCanvas),
				kw.AccelAction("Ctrl+Alt+["),
				kw.AccelAction("Ctrl+A"),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to select Google Docs")
			}

			typingDuration := meetTimeout
			if meet.TypingDuration != 0 {
				typingDuration = meet.TypingDuration
			}
			end := time.Now().Add(typingDuration)

			// By default, type a bolded header and paragraph, then sleep
			// for 5 seconds.
			cycleDescription := "type and sleep"
			cycleActions := []action.Action{
				// Ctrl+Alt+1 is the shortcut for activating Heading 1.
				kw.AccelAction("Ctrl+Alt+1"),

				inputsimulations.TypeSequenceWPMAction(ctx, kw, 120, strings.Split("my bolded header", "")),

				// Press enter to go to the next line and undo the bold lettering.
				kw.AccelAction("Enter"),

				// Type a paragraph in normal text.
				inputsimulations.TypeSequenceWPMAction(ctx, kw, 120, notes),
				kw.AccelAction("Enter"),

				// Add a small delay before typing again.
				action.Sleep(5 * time.Second),
			}

			// If tabSwitchDocs is true, Alt+Tab twice to switch to another window
			// and come back to the current window.
			if meet.TabSwitchDocs {
				cycleDescription = "sleep, type, and task switch"
				taskSwitch := kw.AccelAction("Alt+Tab")
				cycleActions = append(cycleActions,
					taskSwitch,
					action.Sleep(10*time.Second),
					taskSwitch,
				)
			}

			startTracingRoutine(ctx)

			// Start an annotation section for typing on the Google Doc.
			endTypingSection := recorder.AnnotateSection(ctx, "Type_on_docs")
			for time.Until(end) > 0 {
				if err := action.Combine(cycleDescription, cycleActions...)(ctx); err != nil {
					return err
				}
			}
			endTypingSection(ctx)

			// Toggle the Google Docs File menu button for press and
			// release metrics.
			if err := toggleFileMenuButton(ctx, collaborationConn, tconn, kw, ui, pc); err != nil {
				return errors.Wrap(err, "failed to toggle file menu button")
			}

			// Get the Google Docs window again to properly retrieve
			// the split-screen bounds.
			docsWindow, err := ash.GetWindow(ctx, tconn, collaborationWindow.ID)
			if err != nil {
				return errors.Wrap(err, "failed to get the Google Docs window")
			}

			// Highlight text on Google Docs to get mouse drag metrics.
			docsBounds := docsWindow.TargetBounds
			if !inTabletMode {
				if err := mouse.Move(tconn, docsBounds.CenterPoint(), 500*time.Millisecond)(ctx); err != nil {
					return errors.Wrap(err, "failed to move mouse to center of Google Docs window")
				}
			}
			if err := pc.Drag(
				docsBounds.CenterPoint(),
				pc.DragTo(docsBounds.TopLeft(), 500*time.Millisecond),
				pc.DragTo(docsBounds.BottomRight(), time.Second),
				pc.DragTo(docsBounds.CenterPoint(), 500*time.Millisecond),
			)(ctx); err != nil {
				return err
			}

			if inTabletMode {
				x := docsBounds.CenterX()
				topSwipePoint := coords.NewPoint(x, docsBounds.Height/4)
				bottomSwipePoint := coords.NewPoint(x, docsBounds.Height*3/4)

				// Scroll with 1-finger swipe.
				if err := pc.Drag(bottomSwipePoint,
					pc.DragTo(topSwipePoint, time.Second),
					pc.DragTo(bottomSwipePoint, time.Second),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to scroll with 1-finger swipe")
				}
			} else {
				// Scroll with mouse wheel.
				for _, scrollDown := range []bool{true, false} {
					if err := inputsimulations.RepeatMouseScroll(ctx, mw, scrollDown, 50*time.Millisecond, 30); err != nil {
						return errors.Wrap(err, "failed to repeat mouse scroll")
					}
				}
			}

			// Enable docsBlocker extension again to restore Google Docs.
			docsBlockerConn, err := cuj.GetDocsBlockerConn(ctx, cr)
			if err != nil {
				return errors.Wrap(err, "failed to get docs blocker conn")
			}
			defer docsBlockerConn.Close()

			if err := docsBlockerConn.Eval(ctx, "ForceDocsOffline(false)", nil); err != nil {
				testing.ContextLog(ctx, "Failed to call docs blocker to restore: ", err)
			}

			if err := kw.Accel(ctx, "Alt+Tab"); err != nil {
				return errors.Wrap(err, "failed to hit alt-tab and focus back to Meet tab")
			}
			endDocsInteractions(ctx)

			if numPhases == 1 {
				if err := stopSnapshot(ctx); err != nil {
					return errors.Wrap(err, "failed to stop snapshot for Google Docs")
				}
			}
			if meet.Present && isPresenting && ui.Gone(googlemeet.StopPresentingButton)(ctx) == nil {
				testing.ContextLog(ctx, "The connection lost, restart screen sharing and turn on camera")
				recorder.CustomScreenshot(ctx)

				if err := meetHelper.PresentTab(ctx, collaborationConn, kw, presentTabTitle); err != nil {
					return errors.Wrap(err, "failed to start screen sharing")
				}
				// Sometimes the connection may be lost, causing the camera to turn off.
				// Turn on the camera to make sure the number of bots is expected.
				if err := meetHelper.SetCamera(ctx, true); err != nil {
					return errors.Wrap(err, "failed to turn on camera")
				}

				// After reconnecting, it may take more time for videos to load.
				// Waiting for the video to reach the expected number means the loading
				// is complete.
				if err := testing.Poll(ctx, func(ctx context.Context) error {
					videoFinder := nodewith.Role(role.Video).Ancestor(meetRootWebArea)
					videos, err := ui.NodesInfo(ctx, videoFinder)
					if err != nil {
						return errors.Wrap(err, "failed to get info for the videos")
					}
					if len(videos) != expectedParticipantCount {
						return errors.Wrapf(err, "the number of videos is not expected, want %v; got %v", expectedParticipantCount, len(videos))
					}
					return nil
				}, &testing.PollOptions{Timeout: time.Minute, Interval: 10 * time.Second}); err != nil {
					testing.ContextLog(ctx, "Failed to wait for videos to load: ", err)
				}

				recorder.CustomScreenshot(ctx)
			}
		} else {
			startTracingRoutine(ctx)
		}

		// "Stop presenting" if the test wants to interact with
		// Google Slides or Google Sheets later.
		if meet.Slides || meet.Sheets {
			if err := meetHelper.StopPresenting(ctx); err != nil {
				return errors.Wrap(err, "failed to stop presenting")
			}
			// When a participant share the screen, one more participant
			// is added to the meeting. Therefore, when screen sharing stops,
			// the number of participants should decrease by one.
			expectedParticipantCount--
			isPresenting = false
		}

		// If we have a collaboration window open, navigate away from the page
		// to collect LCP metrics.
		if collaborationConn != nil {
			// If leaving the edit document page, the "Leave site?" window may pop up
			// which causing navigation to fail.
			// Set shortCtx to quickly check if it fails to navigate.
			shortCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if err := collaborationConn.Navigate(shortCtx, chrome.VersionURL); err != nil {
				testing.ContextLogf(ctx, "Failed to navigate to %s: %v", chrome.VersionURL, err)
				// If the "Leave site?" prompt pops up, click the leave button.
				if err := uiauto.Retry(3, prompts.ClearPotentialPrompts(tconn, 5*time.Second, prompts.LeaveSitePrompt))(ctx); err != nil {
					return errors.Wrap(err, "failed to clear leave site prompt")
				}
				if err := webutil.WaitForQuiescence(ctx, collaborationConn, 15*time.Second); err != nil {
					testing.ContextLogf(ctx, "Failed to wait for %s to achieve quiescence: %v", chrome.VersionURL, err)
				}
				targets, err := cr.FindTargets(ctx, chrome.MatchTargetURL(chrome.VersionURL))
				if err != nil || len(targets) == 0 {
					return errors.Wrapf(err, "failed to find URL %s", chrome.VersionURL)
				}
			}
		}

		if meet.Slides {
			// Start an annotation section to interact with Google Slides.
			endSlidesInteractions := recorder.AnnotateSection(ctx, "Slides_interactions")

			// Since adding bots also takes snapshots, to avoid collision, only start
			// snapshot for Google Slides if the number of bots won't vary during
			// meetTimeout.
			if numPhases == 1 {
				stopSnapshot, err = recorder.StartSnapshot(ctx, "Slides", ashMetrics, browserMetrics)
				if err != nil {
					return errors.Wrap(err, "failed to start snapshot for Google Slides")
				}
			}

			slidesURL, err := cuj.GetTestSlidesURL(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get Google Slides URL")
			}
			if err := navigate(ctx, collaborationConn, cr, slidesURL); err != nil {
				return errors.Wrap(err, "failed to navigate to Google Slides website")
			}
			// We stopped presenting before navigating to the Google Slides page.
			// Start screen sharing again for the page.
			if err := meetHelper.PresentTab(ctx, collaborationConn, kw, "Google Slides"); err != nil {
				return errors.Wrap(err, "failed to start screen sharing")
			}
			isPresenting = true

			// Ensure the slides deck gets scrolled.
			clickOnPage := googledocs.ClickOnSlidesWebArea(tconn)
			if err := scrollDownPage(ctx, collaborationConn, kw, ui, clickOnPage, "punch-filmstrip-scroll"); err != nil {
				return err
			}
			// Ensure MouseClick, LCP2 and ADF metrics are generated.
			if err := generateMetrics(ctx, collaborationConn, tconn, kw, ui, pc, inTabletMode); err != nil {
				return err
			}

			// "Stop presenting" if the test wants to interact with
			// Google Sheets later.
			if err := meetHelper.StopPresenting(ctx); err != nil {
				return errors.Wrap(err, "failed to stop presenting")
			}
			isPresenting = false
			endSlidesInteractions(ctx)

			if numPhases == 1 {
				if err := stopSnapshot(ctx); err != nil {
					return errors.Wrap(err, "failed to stop snapshot for Google Slides")
				}
			}
		}

		if meet.Sheets {
			// Start an annotation section to interact with Google Sheets.
			endSheetsInteractions := recorder.AnnotateSection(ctx, "Sheets_interactions")

			// Since adding bots also takes snapshots, to avoid collision, only start
			// snapshot for Google Sheets if the number of bots won't vary during
			// meetTimeout.
			if numPhases == 1 {
				stopSnapshot, err = recorder.StartSnapshot(ctx, "Sheets", ashMetrics, browserMetrics)
				if err != nil {
					return errors.Wrap(err, "failed to start snapshot for Google Sheets")
				}
			}

			sheetsURL, err := cuj.GetTestSheetsViewerURL(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get Google Sheets URL")
			}
			if err := navigate(ctx, collaborationConn, cr, sheetsURL); err != nil {
				return errors.Wrap(err, "failed to navigate to Google Sheets website")
			}
			// We stopped presenting before navigating to the Google Sheets page.
			// Start screen sharing again for the page.
			if err := meetHelper.PresentTab(ctx, collaborationConn, kw, "Google Sheets"); err != nil {
				return errors.Wrap(err, "failed to start screen sharing")
			}
			isPresenting = true
			expectedParticipantCount++

			// Ensure the sheets deck gets scrolled.
			clickOnPage := googledocs.ClickOnSheetsWebArea(tconn)
			if err := scrollDownPage(ctx, collaborationConn, kw, ui, clickOnPage, "native-scrollbar-y"); err != nil {
				return err
			}
			// Ensure MouseClick, LCP2 and ADF metrics are generated.
			if err := generateMetrics(ctx, collaborationConn, tconn, kw, ui, pc, inTabletMode); err != nil {
				return err
			}
			endSheetsInteractions(ctx)

			if numPhases == 1 {
				if err := stopSnapshot(ctx); err != nil {
					return errors.Wrap(err, "failed to stop snapshot for Google Sheets")
				}
			}
		}

		// Open some tabs and trigger overview.
		if len(meet.TabsForOverview) > 0 {
			for _, url := range meet.TabsForOverview {
				conn, err := cr.NewConn(ctx, url, browser.WithNewWindow())
				if err != nil {
					return errors.Wrapf(err, "failed to open url %s", url)
				}
				defer conn.Close()
			}
			if err := inputsimulations.DoOverviewWorkflow(ctx, tconn, pc); err != nil {
				return errors.Wrap(err, "failed to do overview workflow")
			}
			if err := meetWindow.ActivateWindow(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to activate the Meet window")
			}
		}

		moveMouseTimeout := time.Until(meetEndTime)
		// Ensures that meet session is long enough. graphics.MeasureGPUCounters
		// exits early without errors on ARM where there is no i915 counters.
		if err := inputsimulations.MoveMouseFor(ctx, tconn, moveMouseTimeout); err != nil {
			return errors.Wrap(err, "failed to simulate mouse movement")
		}
		if err := <-errc; err != nil {
			return errors.Wrap(err, "failed to collect GPU counters")
		}
		select {
		case err := <-clearPromptsChannel:
			if err != nil {
				return errors.Wrap(err, "failed to dismiss the prompt")
			}
		case <-time.After(2 * time.Second):
			testing.ContextLog(ctx, "Dismiss prompt did not start")
		}
		if err := meetHelper.CheckParticipantCount(ctx, expectedParticipantCount); err != nil {
			if isPresenting && ui.Gone(googlemeet.StopPresentingButton)(ctx) == nil {
				return errors.Wrap(err, "the number of bots is unexpected, screen sharing is interrupted")
			}
			return errors.Wrap(err, "the number of bots is unexpected, the bond server may have lost bots")
		}

		return nil
	}); err != nil {
		return pv, errors.Wrap(err, "failed to conduct the recorder task")
	}

	// Before recording the metrics, check if there is any tab crashed.
	assertTabActive(ctx)

	// Sometimes the nodes on the background window cannot be found.
	// Activate the window to download the dump from the WebRTC-internals window.
	if err := webRTCInternalsWindow.ActivateWindow(ctx, tconn); err != nil {
		return pv, errors.Wrap(err, "failed to activate the WebRTC-internals window")
	}
	// Some DUTs need more time to wait for quiescence. Add log for debugging
	// loading duration.
	startTime = time.Now()
	if err := webutil.WaitForQuiescence(ctx, webrtcInternals, 2*time.Minute); err != nil {
		return pv, errors.Wrap(err, "failed to wait for quiescence")
	}
	testing.ContextLog(ctx, "Loading page took: ", time.Since(startTime))

	// Report info from chrome://webrtc-internals.
	path, err := cuj.DumpWebRTCInternals(ctx, tconn, ui, webrtcInternals, cr.NormalizedUser())
	if err != nil {
		// Take a screenshot with the chrome://webrtc-internals tab in
		// the foreground, to facilitate investigation of b/255343902.
		// TODO(b/255343902): Remove this when the bug is fixed.
		recorder.CustomScreenshot(ctx)
		return pv, errors.Wrap(err, "failed to download dump from chrome://webrtc-internals")
	}
	dump, readErr := cuj.ReadWebRTCFile(path)
	if readErr != nil {
		return pv, errors.Wrap(readErr, "failed to read WebRTC internals dump from Downloads folder")
	}
	if err := os.Remove(path); err != nil {
		return pv, errors.Wrap(err, "failed to remove WebRTC internals dump from Downloads folder")
	}
	if readErr == nil {
		if err := os.WriteFile(filepath.Join(outDir, "webrtc-internals.json"), dump, 0644); err != nil {
			return pv, errors.Wrap(err, "failed to write WebRTC internals dump to test results folder")
		}
		enterpriseEffects := meet.Enterprise && meet.Effects
		webRTCInternalsPV, err := cuj.ReportWebRTCInternals(ctx, dump, meetingCode, meet.Bots[len(meet.Bots)-1], enterpriseEffects, meet.Present)
		if err != nil {
			return pv, errors.Wrap(err, "failed to report info from WebRTC internals dump to performance metrics")
		}
		pv.Merge(webRTCInternalsPV)
	}

	// Activate the Meet window to clean up browser zoom and effect settings.
	if err := meetWindow.ActivateWindow(ctx, tconn); err != nil {
		return pv, errors.Wrap(err, "failed to activate the Meet window")
	}

	// Reset the browser zoom, because the browser retains the zoom
	// across test variants.
	if err := kw.Accel(ctx, "Ctrl+0"); err != nil {
		testing.ContextLog(ctx, "Failed to reset browser zoom to 100%")
	}

	if meet.Effects {
		if err := SetVisualEffects(ui, TurnOffEffectsFinder)(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to turn off visual effects: ", err)
			faillog.DumpUITreeWithScreenshotOnError(closeCtx, outDir, func() bool { return true }, cr, "ui_dump_set_visual_effects")
		}
	}

	// Report WebRTC metrics for video streams. Start by closing the Meet window and
	// waiting for the video streams to be gone (according to chrome://webrtc-internals),
	// because the metrics are recorded when the video streams are ended.
	closedMeet = true
	if err := meetWindow.CloseWindow(closeCtx, tconn); err != nil {
		return pv, errors.Wrap(err, "failed to close the meeting")
	}
	if err := ui.WaitUntilGone(nodewith.NameContaining("VideoStream").First())(ctx); err != nil {
		return pv, errors.Wrap(err, "failed to wait for video stream info to disappear")
	}
	hists, err := webRTCMetricsRecorder.Histogram(ctx, tconn)
	if err != nil {
		return pv, errors.Wrap(err, "failed to gather WebRTC metrics for video streams")
	}

	for _, hist := range hists {
		count := hist.TotalCount()
		if count == 0 {
			continue
		}

		info := cujrecorder.WebRTCMetricInfo[hist.Name]
		pv.Set(perf.Metric{
			Name:      hist.Name,
			Unit:      info.Unit,
			Direction: info.Direction,
		}, float64(hist.Sum))

		var bucketMinima []float64
		var bucketMaxima []float64
		for _, bucket := range hist.Buckets {
			// Only report the bucket max + mins if there's more than 1
			// element in the bucket.
			if bucket.Count <= 1 {
				continue
			}

			for i := int64(0); i < bucket.Count; i++ {
				bucketMinima = append(bucketMinima, float64(bucket.Min))
				bucketMaxima = append(bucketMaxima, float64(bucket.Max))
			}

			pv.Set(perf.Metric{
				Name:      hist.Name,
				Variant:   "bucket_minima",
				Unit:      info.Unit,
				Direction: info.Direction,
				Multiple:  true,
			}, bucketMinima...)
			pv.Set(perf.Metric{
				Name:      hist.Name,
				Variant:   "bucket_maxima",
				Unit:      info.Unit,
				Direction: info.Direction,
				Multiple:  true,
			}, bucketMaxima...)
		}
	}

	if meet.MeasureEcho {
		downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
		if err != nil {
			return pv, errors.Wrap(err, "failed to get Downloads path")
		}
		rms, err := cuj.CalculateEchoRMS(ctx, downloadsPath)
		if err != nil {
			return pv, errors.Wrap(err, "failed to calculate echo rms")
		}

		pv.Set(perf.Metric{
			Name:      "EchoRMS",
			Unit:      "dB",
			Direction: perf.SmallerIsBetter,
		}, float64(rms))
	}

	if err := recorder.Record(ctx, pv); err != nil {
		return pv, errors.Wrap(err, "failed to record the data")
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		return pv, errors.Wrap(err, "failed to save histogram raw data")
	}
	if err := pv.Save(outDir); err != nil {
		return pv, errors.Wrap(err, "failed to save the perf data")
	}
	return pv, nil
}

// toggleFileMenuButton toggles the "File" menu button for press and release metrics.
func toggleFileMenuButton(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, kw *input.KeyboardEventWriter,
	ui *uiauto.Context, pc pointer.Context) error {
	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for the page to quiesce")
	}
	return uiauto.NamedCombine("toggle file menu button",
		// Show the menus before clicking the File menu button.
		// If the menus are not shown, the File menu button may not be clickable.
		// If the Google Docs window exists, show the doc menus.
		uiauto.IfSuccessThen(ui.Exists(googledocs.DocsWindow), googledocs.ShowTheDocMenus(tconn, kw)),
		// If the Google Slides window exists, show the slide menus.
		uiauto.IfSuccessThen(ui.Exists(googledocs.SlidesWindow), googledocs.ShowTheSlideMenus(tconn)),
		// If the Google Sheets window exists, show the sheet menus.
		uiauto.IfSuccessThen(ui.Exists(googledocs.SheetsWindow), googledocs.ShowTheSheetMenus(tconn)),
		googledocs.ClickFileMenuButtonWithJS(conn, tconn, ui, pc),
	)(ctx)
}

// ensureElementGetsScrolled ensures element gets scrolled.
func ensureElementGetsScrolled(ctx context.Context, conn *chrome.Conn, element string) error {
	testing.ContextLog(ctx, "Ensure element gets scrolled")
	var scrollTop int
	if err := conn.Eval(ctx, fmt.Sprintf("parseInt(%s.scrollTop)", element), &scrollTop); err != nil {
		return errors.Wrap(err, "failed to get the number of pixels that the scrollbar is scrolled vertically")
	}
	if scrollTop == 0 {
		return errors.Errorf("%s is not getting scrolled", element)
	}
	return nil
}

// navigate navigates to the url and waits for the page to quiesce,
// and then focus on it.
func navigate(ctx context.Context, conn *chrome.Conn, cr *chrome.Chrome, url string) error {
	if err := conn.Navigate(ctx, url); err != nil {
		return errors.Wrapf(err, "failed to navigate to %s", url)
	}

	// Some DUTs need more time to wait for quiescence. Add log for debugging
	// loading duration. If waiting for the page to quiesce fails, just print
	// the log.
	startTime := time.Now()
	if err := webutil.WaitForQuiescence(ctx, conn, 2*time.Minute); err != nil {
		testing.ContextLog(ctx, "Ignoring waiting for page to quiesce: ", err)
	} else {
		testing.ContextLog(ctx, "Loading page took: ", time.Since(startTime))
	}

	targets, err := cr.FindTargets(ctx, chrome.MatchTargetURLPrefix(url))
	if err != nil || len(targets) == 0 {
		return errors.Wrapf(err, "failed to find URL %s", url)
	}
	return conn.ActivateTarget(ctx)
}

// scrollDownPage clicks on the page, waits for the expected element, scrolls
// down the page by pressing Down key, and checks if the specified HTML
// element is scrolled.
func scrollDownPage(ctx context.Context, conn *chrome.Conn, kw *input.KeyboardEventWriter, ui *uiauto.Context,
	clickOnPage action.Action, className string) error {
	scrollDown := func(ctx context.Context) error {
		element := "document.getElementsByClassName('" + className + "')[0]"
		testing.ContextLog(ctx, "Going through the file")
		if err := inputsimulations.RepeatKeyPress(ctx, kw, "Down", 50*time.Millisecond, 60); err != nil {
			return errors.Wrap(err, `failed to repeatedly and rapidly press "Down" in between task switches`)
		}
		if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for the page to quiesce")
		}
		// Ensure the element gets scrolled.
		return ensureElementGetsScrolled(ctx, conn, element)
	}
	elementFinder := nodewith.HasClass(className).Role(role.GenericContainer)
	return uiauto.Retry(3, uiauto.NamedCombine("scroll down page",
		clickOnPage,
		ui.WaitUntilExists(elementFinder),
		scrollDown,
	))(ctx)
}

// generateMetrics generates metrics by interacting with the page and the Ash UI.
func generateMetrics(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, kw *input.KeyboardEventWriter,
	ui *uiauto.Context, pc pointer.Context, inTabletMode bool) error {
	// Collect mouse events by toggling "File" button.
	if err := toggleFileMenuButton(ctx, conn, tconn, kw, ui, pc); err != nil {
		return errors.Wrap(err, "failed to toggle the File menu button")
	}
	// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
	if err := conn.Navigate(ctx, chrome.VersionURL); err != nil {
		return errors.Wrapf(err, "failed to navigate to %s", chrome.VersionURL)
	}
	// Perform Ash workflows to get ADF metrics.
	if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
		return errors.Wrap(err, "failed to do Ash workflows")
	}
	return nil
}

// dismissPromptIfExists retry to dismiss the prompt in the background if the dialog exists.
func dismissPromptIfExists(ctx context.Context, tconn *chrome.TestConn, errCh chan error) {
	async.Run(ctx, func(ctx context.Context) {
		errCh <- uiauto.Retry(3, prompts.ClearPotentialPrompts(
			tconn,
			longUITimeout,
			prompts.OthersSeeDiffPrompt,
		))(ctx)
	}, "dismiss the prompt if it exists")
}
