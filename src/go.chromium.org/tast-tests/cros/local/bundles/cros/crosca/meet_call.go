// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crosca contains a collections of performance tests
package crosca

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/bond"
	ch "go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/crosca/histogram"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Default runtime should be about 5 mins although manual runtime is expected
// to be about 30 mins controlled by variable.
const (
	testTimeout   = time.Hour
	longUITimeout = time.Minute
)

var (
	meetRootWebArea   = nodewith.NameContaining("Meet").Role(role.RootWebArea)
	histogramsWebArea = nodewith.Name("Histograms").Role(role.RootWebArea)

	// meetCallHistograms records histograms of MeetCall performance comparisons.
	meetCallHistograms = []string{
		"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
		"EventLatency.KeyPressed.TotalLatency",
		"EventLatency.MousePressed.TotalLatency",
		"PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
		"PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
		"PageLoad.InteractiveTiming.InputDelay3",
		"PageLoad.InteractiveTiming.TimeToNextPaint",
		"PageLoad.Experimental.NavigationTiming.NavigationStartToFirstResponseStart",
		"Graphics.Smoothness.Jank3.AllSequences",
	}
)

type meetTest struct {
	bots        []int
	botsOptions []bond.AddBotsOption // Customizes the meeting participant bots.
}

// crosca.MeetCall.meetingCode allows user to specify the meeting room code
var meetingCodeVarString = testing.RegisterVarString(
	"crosca.MeetCall.meetingCode",
	"",
	"A string value signifying the meeting code for joining meet call",
)

// crosca.MeetCall.meetingCode allows user to specify the duration of typing
// in chat
var typingTimeoutVarString = testing.RegisterVarString(
	"crosca.MeetCall.typingTimeout",
	"",
	"A integer value signifying the duration of chat typing during the meet call",
)

// crosca.MeetCall.meetingCode allows user to specify whether to pin self
// during the meeting
var pinSelfVarString = testing.RegisterVarString(
	"crosca.MeetCall.pinSelf",
	"",
	"A BOOL value to indicate whether to pin self",
)

// crosca.MeetCall.meetingCode allows user to specify the brightness of
// the display
var brightnessVarString = testing.RegisterVarString(
	"crosca.MeetCall.brightness",
	"",
	"A integer value signifying the brightness of the display",
)

// crosca.MeetCall.meetingCode allow user to specify delay after each character
// type
var typingDelayVarString = testing.RegisterVarString(
	"crosca.MeetCall.typingDelay",
	"",
	"A integer value signifying the delay after typing each character in chat window",
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MeetCall,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Host a Google Meet video conference and type in the chat window",
		Contacts: []string{
			"chromeos-competitive-analysis@google.com",
			"williskung@google.com",
		},
		BugComponent: "b:1485133", // ChromeOS > Platform > baseOS > Performance > Competitive Analysis
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"crosca.MeetCall.bond_credentials",
		},
		Params: []testing.Param{
			{
				Name:    "2p",
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
				Timeout: testTimeout,
				Val: meetTest{
					bots:        []int{1},
					botsOptions: []bond.AddBotsOption{bond.WithDefaultVideo("jamboard_three_close_video_hd.1280_720.yuv")},
				},
			},
			{
				Name:    "16p",
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
				Timeout: testTimeout,
				Val: meetTest{
					bots:        []int{15},
					botsOptions: []bond.AddBotsOption{bond.WithDefaultVideo("jamboard_three_close_video_hd.1280_720.yuv")},
				},
			},
			{
				Name:    "2p_disable_arc",
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingDisableARC",
				Timeout: testTimeout,
				Val: meetTest{
					bots:        []int{1},
					botsOptions: []bond.AddBotsOption{bond.WithDefaultVideo("jamboard_three_close_video_hd.1280_720.yuv")},
				},
			},
			{
				Name:    "16p_disable_arc",
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingDisableARC",
				Timeout: testTimeout,
				Val: meetTest{
					bots:        []int{15},
					botsOptions: []bond.AddBotsOption{bond.WithDefaultVideo("jamboard_three_close_video_hd.1280_720.yuv")},
				},
			},
		},
	})
}

// MeetCall measures the performance of critical user journeys for Google Meet.
// This tast test is structured to create similar workload compared to the corresponding
// Windows python test.
func MeetCall(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	bTconn := tconn
	ui := uiauto.New(tconn)
	meetingCode := ""
	if strings.ToLower(meetingCodeVarString.Value()) != "" {
		meetingCode := strings.ToLower(meetingCodeVarString.Value())
		s.Log("crosca.MeetCall.meetingCode value: ", meetingCode)
	}

	typingTimeout := 2 * time.Minute
	if strings.ToLower(typingTimeoutVarString.Value()) != "" {
		typingTimeoutStr := strings.ToLower(typingTimeoutVarString.Value())
		s.Log("crosca.MeetCall.typingTimeout: ", typingTimeoutStr)
		t, err := strconv.Atoi(typingTimeoutStr)
		if err != nil {
			s.Fatal("Failed to parse crosca.MeetCall.typingTimeout: ", err)
		}
		typingTimeout = time.Duration(t) * time.Minute
	}

	var brightness uint
	brightness, err = backlightBrightness(ctx)
	if err != nil {
		s.Fatal("Failed to read brightness before joining meet call")
	}
	s.Log("Display brightness before joining meet call: ", brightness)

	if strings.ToLower(brightnessVarString.Value()) != "" {
		brightnessStr := strings.ToLower(brightnessVarString.Value())
		s.Log("crosca.MeetCall.brightness value: ", brightnessStr)
		displayBrightness, err := strconv.Atoi(brightnessStr)
		if err != nil {
			s.Fatal("Failed to parse crosca.MeetCall.brightness: ", err)
		}
		brightnessArg := "--set_brightness=" + strconv.FormatUint(uint64(displayBrightness), 10)
		err = testexec.CommandContext(ctx, "backlight_tool", brightnessArg).Run(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("unable to set backlight brightness")
		}
	}

	const botDurationMinutes = 45

	if meetingCode == "" {
		const addBotTimeout = 100 * time.Second
		meet := s.Param().(meetTest)
		if len(meet.bots) == 0 {
			s.Fatal("Must have at least 1 bot count")
		}
		if meet.bots[0] == 0 {
			s.Fatal("First bot count must have at least 1 bot, to add the spotlight bot")
		}
		creds := s.RequiredVar("crosca.MeetCall.bond_credentials")
		bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
		if err != nil {
			s.Fatal("Failed to create a bond client: ", err)
		}
		defer bc.Close()
		meetingCode, err = bc.CreateConference(ctx)
		if err != nil {
			s.Fatal("Failed to create a conference room: ", err)
		}
		s.Log("Created a room with the code ", meetingCode)
		// Variable addBotTimeout(100s) would allow 3 bond.longerSendTimeout(30s) attempts
		// to request the bond server to add bots.
		sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
		defer cancel()
		// Create a bot with spotlight layout to request HD video.
		spotlightBotList, _, err := bc.AddBots(sctx, meetingCode, 1, typingTimeout+botDurationMinutes*time.Minute, append(meet.botsOptions, bond.WithLayout("SPOTLIGHT"))...)
		if err != nil {
			s.Fatal("Failed to create bot with spotlight layout: ", err)
		}
		if len(spotlightBotList) != 1 {
			s.Fatalf("Unexpected number of bots with spotlight layout successfully started; got %d, expected 1", len(spotlightBotList))
		}
		// Keep track of how many bots are already in the call, so we can add the
		// right number of bots later in the test.
		botsInCall := 1
		addBots := func(ctx context.Context, numBots int) error {
			sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
			defer cancel()
			testing.ContextLogf(ctx, "Adding %d bots to the call", numBots)
			if numBots == 0 {
				return nil
			}
			wait := 100 * time.Millisecond
			for i := 0; i < 3; i++ {
				// GoBigSleepLint: A short sleep before next call to Bond API.
				if err := testing.Sleep(ctx, wait); err != nil {
					s.Errorf("Failed to sleep for %v: %v", wait, err)
				}
				// Exponential backoff. The wait time is 0.1s, 1s and 10s before each retry.
				wait *= 10
				// Add 45 (botDurationMinutes) minutes to the bot duration, to ensure that the bots stay long
				// enough for the test to get info from chrome://webrtc-internals.
				botList, numFailures, err := bc.AddBots(sctx, meetingCode, numBots, typingTimeout+botDurationMinutes*time.Minute, append(meet.botsOptions, bond.WithDefaultVideo("jamboard_three_close_video_hd.1280_720.yuv"))...)
				if err != nil {
					s.Fatalf("Failed to create %d bots: %v", numBots, err)
				}
				s.Logf("%d bots started, %d bots failed", len(botList), numFailures)
				if numFailures == 0 {
					break
				}
				numBots -= len(botList)
			}
			return nil
		}
		numBotsToAdd := meet.bots[0] - botsInCall
		if err := addBots(ctx, numBotsToAdd); err != nil {
			s.Fatalf("Failed to initially add %d bots: %v", numBotsToAdd, err)
		}
		botsInCall += numBotsToAdd
		s.Log("Total number of bots ", botsInCall)
	}
	// Shorten context to allow for cleanup. Reserve one minute in case of power test.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kb.Close(closeCtx)

	// Additional delay after kb.sleepAfterType in ms. This additional delay is here to allow each type
	// characters to be shown on chat windows distinctly.
	kb.AdditionDelay10msIncrementCounter = 0
	if strings.ToLower(typingDelayVarString.Value()) != "" {
		typingDelayStr := strings.ToLower(typingDelayVarString.Value())
		s.Log("crosca.MeetCall.typingDelay value: ", typingDelayStr)
		kb.AdditionDelay10msIncrementCounter, err = strconv.Atoi(typingDelayStr)
		if err != nil {
			s.Fatal("Failed to parse crosca.MeetCall.typingDelay: ", err)
		}
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{
		Mode: cujrecorder.Benchmark,
	})
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}
	defer recorder.Close(closeCtx)
	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}
	// Open chrome://webrtc-internals now so it will collect data on the meeting's streams.
	webrtcInternals, err := recorder.NewConn(ctx, cr.Browser(), "WebRTC_Internals", "chrome://webrtc-internals", browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to open chrome://webrtc-internals: ", err)
	}
	defer webrtcInternals.Close()
	webRTCInternalsWindow, err := ash.FindOnlyWindow(ctx, tconn, ash.BrowserTitleMatch(browser.TypeAsh, "WebRTC Internals"))
	if err != nil {
		s.Fatal("Failed to find the WebRTC Internals window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, webRTCInternalsWindow.ID, ash.WindowStateMaximized); err != nil {
		s.Log("Failed to ensure that the WebRTC Internals window is maximized: ", err)
	}
	var conns []*chrome.Conn
	defer func(ctx context.Context) {
		faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")
		s.Log("Skip closing all the histogram tabs for debug")
		if len(conns) > 0 {
			for _, conn := range conns {
				conn.CloseTarget(ctx)
				conn.Close()
			}
		}
		webrtcInternals.Close()
	}(closeCtx)

	// Open histogram detail page for each of the metrics to match windows python test operations.
	tabs, err := histogram.CreateHistogramTabs(ctx, cr, meetCallHistograms)
	if err != nil {
		s.Fatal("Failed to create histogram tabs: ", err)
	}
	defer func(ctx context.Context) {
		for _, tab := range tabs {
			if err := tab.Close(ctx); err != nil {
				s.Log("Failed to close histogram tab: ", err)
			}
		}
	}(closeCtx)

	histogramsAfterWebrtcAndMetrics, err := metrics.GetHistograms(ctx, tconn, meetCallHistograms)
	if err != nil {
		s.Log("Failed to open histogram pages: ", err)
	}
	s.Log("histogram snapshot after webrtc and metric tabs open: ", histogramsAfterWebrtcAndMetrics)

	switchToWebRTCInternals := func() error {
		// Sometimes the nodes on the background window cannot be found.
		// Activate the window to download the dump from the WebRTC-internals window.
		if err := webRTCInternalsWindow.ActivateWindow(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to activate the WebRTC-internals window")
		}
		if err := kb.Accel(ctx, "Ctrl+1"); err != nil {
			return errors.Wrap(err, "failed to switch to first tab")
		}
		return nil
	}
	if err := switchToWebRTCInternals(); err != nil {
		s.Fatal("Failed to switch to WebRTC-internals tab: ", err)
	}
	// GoBigSleepLint: A short delay is to allow tab switch to WebRTC to complete.
	testing.Sleep(ctx, 1*time.Second)

	if err := histogram.SwitchToMonitoringMode(ctx, ui, kb, tabs); err != nil {
		s.Fatal("Failed to switch to monitoring mode: ", err)
	}

	histogramsAfterMonitoringModeEnabled, err := metrics.GetHistograms(ctx, tconn, meetCallHistograms)
	if err != nil {
		s.Log("Failed to open histogram pages: ", err)
	}
	s.Log("histogram snapshot after metric monitoring mode enabled (before typing): ", histogramsAfterMonitoringModeEnabled)

	deltaHistograms, err := ch.DiffHistograms(histogramsAfterWebrtcAndMetrics, histogramsAfterMonitoringModeEnabled)
	if err != nil {
		s.Log("Failed to diff histograms")
	}
	s.Log("delta histogram from webrtc_and_tab to Monitoring Mode enabled", deltaHistograms)

	// Set audio volume to zero.
	const expectedVolumePercent = 0
	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create a volumeHelper")
	}
	originalVolume, err := vh.GetVolume(ctx)
	s.Log("Set volumen from Original volume to zero from ", originalVolume)
	if err != nil {
		s.Fatal("Failed to get volume")
	}
	testing.ContextLogf(ctx, "Setting the audio volume to %d%% of the maximum volume. Current volume: %d", expectedVolumePercent, originalVolume)
	if err := vh.SetVolume(ctx, expectedVolumePercent); err != nil {
		s.Fatal("Failed to set volume to ", expectedVolumePercent)
	}

	defer vh.SetVolume(ctx, originalVolume)

	conn, err := cr.NewConn(ctx, "https://meet.google.com/"+meetingCode, browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to open the meet website: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(closeCtx)
	// Match window titles `Google Meet` and `meet.google.com`.
	meetRE := regexp.MustCompile(`\bMeet\b|\bmeet\.\b`)
	meetWindow, err := ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return meetRE.MatchString(w.Title) })
	if err != nil {
		s.Fatal("Failed to find the Meet window: ", err)
	}
	// Activate the Meet window to clean up browser zoom and effect settings.
	if err := meetWindow.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to activate the Meet window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, meetWindow.ID, ash.WindowStateFullscreen); err != nil {
		s.Fatal("Failed to set Meet window state to fullscreen: ", err)
	}
	if err := prompts.ClearPotentialPrompts(
		tconn,
		longUITimeout,
		prompts.ShowNotificationsPrompt,
		prompts.AllowAVPermissionPrompt,
		prompts.AllowMicrophoneAndCameraPermissionPrompt,
		prompts.AllowCaptionPrompt,
	)(ctx); err != nil {
		s.Fatal("Failed to grant permissions: ", err)
	}

	pinSelf := false
	if strings.ToLower(pinSelfVarString.Value()) == "true" {
		s.Log("crosca.MeetCall.pinSelf value: true")
		pinSelf = true
	}

	if pinSelf == true {
		if err := pinToScreen(ui, kb)(ctx); err != nil {
			s.Fatal("Failed to pin yourself to screen: ", err)
		}
	}

	s.Log("Check bluetooth enablement status")
	// Use a short context to ensure bluetooth checking returns.
	btCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	isBtEnabled, err := bluetooth.IsEnabled(btCtx)
	if err != nil {
		s.Log("Failed to check bluetooth enablement status: ", err)
	}
	if isBtEnabled {
		s.Log("Start to disable bluetooth")
		if err := bluetooth.Disable(ctx); err != nil {
			s.Fatal("Failed to disable bluetooth: ", err)
		}
		s.Log("Bluetooth disabled")
		defer func(ctx context.Context) {
			if err := bluetooth.Enable(ctx); err != nil {
				s.Fatal("Failed to connect bluetooth: ", err)
			}
		}(closeCtx)
	}

	s.Log("Start to turn off camera")
	// Turn off camera to avoid background inconsistency.
	cameraButton := nodewith.NameStartingWith("Turn off camera").Role(role.Button)
	if err := uiauto.Combine("Turn off camera",
		kb.AccelAction("Ctrl+Tab"),
		ui.WaitUntilExists(cameraButton),
		ui.LeftClick(cameraButton),
	)(ctx); err != nil {
		s.Fatal("Failed to turn off camera: ", err)
	}

	// Mute default audio device
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create cras for default audio device: ", err)
	}
	s.Log("Mute default audio device")
	if err := cras.SetInputMute(ctx, true); err != nil {
		s.Fatal("Failed to mute default audio device: ", err)
	}

	s.Log("Add 2s delay for display to be stabilized")
	// GoBigSleepLint: A short delay is to allow all the background to be stablized.
	testing.Sleep(ctx, 2*time.Second)

	if err := meetWindow.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to activate the Meet window: ", err)
	}
	counter := 0
	pv := perf.NewValues()
	if err := recorder.Run(ctx, func(ctx context.Context) error {
		s.Log("Types the sequence of characters for ", typingTimeout)

		histogramsBeforeTyping, err := metrics.GetHistograms(ctx, tconn, meetCallHistograms)
		if err != nil {
			s.Log("Failed to open histogram pages: ", err)
		}
		s.Log("histogram after metric before typing: ", histogramsBeforeTyping)

		deltaHistograms, err := ch.DiffHistograms(histogramsAfterMonitoringModeEnabled, histogramsBeforeTyping)
		if err != nil {
			s.Log("Failed to diff histograms")
		}
		s.Log("delta histogram from monitoring mode enabled to Before Typing: ", deltaHistograms)

		brightness, err = backlightBrightness(ctx)
		if err != nil {
			s.Fatal("unable to parse current backlight brightness before typing")
		}
		s.Log("Display brightness before typing: ", brightness)

		s.Log("Start typing and beginning counter value before typing: ", counter)
		// GoBigSleepLint: A short countdown to signal to user to start power meter manually.
		testing.Sleep(ctx, 1*time.Second)

		perfCmd := testexec.CommandContext(ctx, "perf", "stat", "-a", "-e", "cycles,instructions,cache-references,cache-misses,bus-cycles,page-faults", "-o", "/var/log/perf_output.txt")
		if err := perfCmd.Start(); err != nil {
			perfCmd.DumpLog(ctx)
			s.Log("perf stat -e cycles,instructions -o /var/log/perf_output.txt")
		}

		for endTime := time.Now().Add(typingTimeout); time.Now().Before(endTime); {
			for _, msg := range []string{
				// "Wear two pairs of thick socks. Wear two pairs of thick socks. Wear a pair of thick socks",
				// "Running on a cold and cloudy day Running on a cold rainy day Running on a cold rainy day",
				"Wear two pairs of thick socks",
				"to run on a freezing cold day",
			} {
				strCounter := fmt.Sprintf("%v", counter)
				if counter <= 9 {
					strCounter = "00" + strCounter
				} else if counter <= 99 {
					strCounter = "0" + strCounter
				} else {
				}
				// Type counter value to chat window to show progress to user.
				msgWithCounter := msg + "-" + strCounter + "-c-count"
				// msgWithCounter := msg + "-" + strCounter

				if err := uiauto.Combine("send message to meeting",
					typingInChat(ui, kb, msgWithCounter),
				)(ctx); err != nil {
					return err
				}
			}
			counter++
		}

		perfCmd.Signal(unix.SIGINT)
		err = perfCmd.Wait()
		if err != nil {
			s.Log("Failed closing perfCmd file")
		}

		perfOutput, err := testexec.CommandContext(ctx, "cat", "/var/log/perf_output.txt").Output()
		if err != nil {
			s.Log("Failed reading perf_output.txt")
		} else {
			perfOutputStr := fmt.Sprintf("\n%s", string(perfOutput))
			s.Log("perf command output begin", perfOutputStr)
			s.Log("perf command output end")
		}

		s.Log("completed typing and final counter value after typing: ", counter)
		// GoBigSleepLint: A short countdown to allow user to stop power meter manually.
		testing.Sleep(ctx, 1*time.Second)

		brightness, err = backlightBrightness(ctx)
		if err != nil {
			s.Fatal("unable to parse current backlight brightness after typing")
		}
		s.Log("Display brightness after typing: ", brightness)

		histogramsAfterTyping, err := metrics.GetHistograms(ctx, tconn, meetCallHistograms)
		if err != nil {
			s.Log("Failed to open histogram pages: ", err)
		}
		s.Log("histogram after typing: ", histogramsAfterTyping)

		deltaHistograms, err = ch.DiffHistograms(histogramsBeforeTyping, histogramsAfterTyping)
		if err != nil {
			s.Log("Failed to diff histograms")
		}
		s.Log("delta histogram from Before to After Typing: ", deltaHistograms)

		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	// Take screenshot before closing meet windows to record number of bots at the end of the test.
	filename := fmt.Sprintf("%s_%s.png", "MeetWindows_before_CloseWindow", time.Now().Format("2006-01-02-15-04-05"))
	path := filepath.Join(s.OutDir(), filename)
	if err := screenshot.Capture(ctx, path); err != nil {
		testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
	}

	if err := meetWindow.CloseWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to close Meet window: ", err)
	}
	if err := switchToWebRTCInternals(); err != nil {
		s.Fatal("Failed to switch to WebRTC-internals tab: ", err)
	}
	// Report info from chrome://webrtc-internals.
	webRTCUI := ui.WithTimeout(3 * time.Minute)
	if path, err := cuj.DumpWebRTCInternals(ctx, tconn, webRTCUI, cr.NormalizedUser()); err != nil {
		s.Error("Failed to download dump from chrome://webrtc-internals: ", err)
		// Take a screenshot with the chrome://webrtc-internals tab in
		// the foreground, to facilitate investigation of b/255343902.
		// TODO(b/255343902): Remove this when the bug is fixed.
		recorder.CustomScreenshot(ctx)
	} else {
		dump, readErr := os.ReadFile(path)
		if readErr != nil {
			s.Error("Failed to read WebRTC internals dump from Downloads folder: ", readErr)
		}
		if err := os.Remove(path); err != nil {
			s.Error("Failed to remove WebRTC internals dump from Downloads folder: ", err)
		}
		if readErr == nil {
			if err := os.WriteFile(filepath.Join(s.OutDir(), "webrtc-internals.json"), dump, 0644); err != nil {
				s.Error("Failed to write WebRTC internals dump to test results folder: ", err)
			}
		}
	}

	if err := histogram.GenerateResults(ctx, ui, tabs, s.OutDir()); err != nil {
		s.Fatal("Failed to generate results: ", err)
	}
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
	if err := recorder.SaveHistograms(s.OutDir()); err != nil {
		s.Error("Failed to save histogram raw data")
	}
}

func pinToScreen(ui *uiauto.Context, kb *input.KeyboardEventWriter) uiauto.Action {
	showEveryone := nodewith.Name("Show everyone").Role(role.ToggleButton).Ancestor(meetRootWebArea)
	participantsList := nodewith.Name("Participants").Role(role.List)
	botListItem := nodewith.Role(role.ListItem).Nth(1).Ancestor(participantsList)
	moreActions := nodewith.Name("More actions").Role(role.PopUpButton).Ancestor(botListItem)
	pinToScreen := nodewith.Name("Pin").Role(role.Button)
	return uiauto.NamedCombine("pin to screen",
		ui.LeftClick(showEveryone),
		ui.LeftClick(moreActions),
		ui.LeftClick(pinToScreen),
		kb.AccelAction("Esc"),
	)
}

func typingInChat(ui *uiauto.Context, kb *input.KeyboardEventWriter, message string) uiauto.Action {
	const retryTimes = 3
	meetRootWebArea := nodewith.NameContaining("Meet").Role(role.RootWebArea)
	firstText := nodewith.Role(role.StaticText).Ancestor(meetRootWebArea).First()
	chatButton := nodewith.Name("Chat with everyone").Role(role.ToggleButton).Ancestor(meetRootWebArea)
	chatText := nodewith.NameContaining("Send a message")
	// There may be multiple "Send a message" fields, so add First() here.
	chatTextField := chatText.Role(role.TextField).First()
	openChatPanel := uiauto.NamedCombine("open chat panel",
		ui.LeftClick(firstText),
		ui.DoDefault(chatButton),
		// Some low end DUTs need very long time to load chat window in 49 tiles.
		ui.WithTimeout(2*time.Minute).WaitUntilExists(chatTextField.Focusable()),
	)
	// There may be multiple "Send a message" buttons, so add First() here.
	chatTextButton := chatText.Role(role.Button).First()
	// There may be multiple message texts, so add First() here.
	messageRe := regexp.MustCompile("(?i)" + message)
	messageText := nodewith.NameRegex(messageRe).Role(role.StaticText).First()
	sendMessageButton := nodewith.Name("Send a message").Role(role.Button)
	enterText := uiauto.Combine("type message",
		ui.WithTimeout(longUITimeout).DoDefaultUntil(chatTextButton,
			ui.WaitUntilExists(chatTextField.Editable().Focused())),
		kb.TypeAction(message),
		uiauto.Sleep(1*time.Second),
		ui.WaitUntilExists(messageText),
		// kb.AccelAction("enter"),
		ui.DoDefault(sendMessageButton),
		ui.WithTimeout(longUITimeout).WaitUntilExists(messageText),
	)
	chatPanelHeading := nodewith.Name("In-call messages").Role(role.Heading).Ancestor(meetRootWebArea)
	return ui.Retry(retryTimes, uiauto.Combine("open chat panel and type message",
		ui.Retry(retryTimes, uiauto.IfSuccessThen(ui.Gone(chatPanelHeading), openChatPanel)),
		ui.Retry(retryTimes, enterText)))
}

func backlightBrightness(ctx context.Context) (uint, error) {
	output, err := testexec.CommandContext(ctx, "backlight_tool", "--get_brightness").Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get current backlight brightness")
	}
	brightness, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current backlight brightness from %q", output)
	}
	return uint(brightness), nil
}
