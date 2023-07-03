// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MeetMultiTaskingCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the total performance of multi-tasking with video conferencing CUJ",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      10 * time.Minute,
		VarDeps: []string{
			"ui.bond_credentials",
		},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "loggedInToCUJUser",
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			Fixture:           "loggedInToCUJUserLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// MeetMultiTaskingCUJ measures the performance of critical user journeys for multi-tasking with video conference.
//
// Pre-preparation:
//   - Open a Meet window and grant permissions.
//
// During recording:
//   - Join the meeting.
//   - Add a participant (bot) to the meeting.
//   - Open a large Google Docs file and scroll down.
//   - Open a large Google Slides file and go down.
//   - Open the Gmail inbox and scroll down.
//
// After recording:
//   - Record and save metrics.
func MeetMultiTaskingCUJ(ctx context.Context, s *testing.State) {
	const (
		createConfTimeout   = 30 * time.Second
		addBotTimeout       = 100 * time.Second
		docsScrollTimeout   = 30 * time.Second
		slidesScrollTimeout = 30 * time.Second
		gmailURL            = "https://gmail.com"
		gmailScrollTimeout  = 10 * time.Second
		newTabTitle         = "New Tab"
		meetTimeout         = 2 * time.Minute
		meetLayout          = "Auto"
	)

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	var l *lacros.Lacros
	var cs ash.ConnSource
	var bTconn *chrome.TestConn
	switch bt {
	case browser.TypeLacros:
		var err error
		if cr, l, cs, err = lacros.Setup(ctx, s.FixtValue(), browser.TypeLacros); err != nil {
			s.Fatal("Failed to initialize test: ", err)
		}
		if bTconn, err = l.TestAPIConn(ctx); err != nil {
			s.Fatal("Failed to get lacros TestAPIConn: ", err)
		}
		defer lacros.CloseLacros(closeCtx, l)
	case browser.TypeAsh:
		cs = cr
		bTconn = tconn
	default:
		s.Fatal("Unrecognized browser type: ", bt)
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to detect it is in tablet-mode or not: ", err)
	}
	var pc pointer.Context
	if inTabletMode {
		// If it is in tablet mode, ensure it it in landscape orientation.
		orientation, err := display.GetOrientation(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get display orientation: ", err)
		}
		if orientation.Type == display.OrientationPortraitPrimary {
			info, err := display.GetPrimaryInfo(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get the primary display info: ", err)
			}
			s.Log("Rotating display 90 degrees")
			if err := display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate90); err != nil {
				s.Fatal("Failed to rotate display: ", err)
			}
			defer display.SetDisplayRotationSync(closeCtx, tconn, info.ID, display.Rotate0)
		}
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to create a touch controller: ", err)
		}
	} else {
		// Make it into a maximized window if it is in clamshell-mode.
		if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
			return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
		}); err != nil {
			s.Fatal("Failed to turn all windows into maximized state: ", err)
		}
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)

	creds := s.RequiredVar("ui.bond_credentials")
	bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
	if err != nil {
		s.Fatal("Failed to create a bond client: ", err)
	}
	defer bc.Close()

	var meetingCode string
	func() {
		// createConfTimeout(30s) would allow 3 bond.defaultSendTimeout(8s)
		// attempts to request the bond server to create conference.
		sctx, cancel := context.WithTimeout(ctx, createConfTimeout)
		defer cancel()
		meetingCode, err = bc.CreateConference(sctx)
		if err != nil {
			s.Fatal("Failed to create a conference room: ", err)
		}
	}()
	s.Log("Created a room with the code ", meetingCode)

	// addBotTimeout(100s) would allow 3 bond.longerSendTimeout(30s) attempts
	// to request the bond server to add bots.
	sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
	defer cancel()
	// Add 30 seconds to the bot duration to make sure that bots do not leave
	// slightly earlier than the test scenario.
	if _, _, err := bc.AddBots(sctx, meetingCode, 1, meetTimeout+30*time.Second); err != nil {
		s.Fatal("Failed to create 1 bot: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Removing all bots from the call")
		if _, _, err := bc.RemoveAllBots(ctx, meetingCode); err != nil {
			s.Fatal("Failed to remove all bots: ", err)
		}
	}(closeCtx)

	meetConn, err := cs.NewConn(ctx, "https://meet.google.com/"+meetingCode, browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to open the hangout meet website: ", err)
	}
	defer meetConn.Close()
	defer meetConn.CloseTarget(closeCtx)
	defer faillog.DumpUITreeOnError(closeCtx, s.OutDir(), s.HasError, tconn)

	// Lacros specific setup.
	if bt == browser.TypeLacros {
		if err := l.Browser().CloseWithURL(ctx, chrome.NewTabURL); err != nil {
			s.Fatal("Failed to close blank tab: ", err)
		}
	}

	// Create a virtual trackpad.
	tpw, err := input.Trackpad(ctx)
	if err != nil {
		s.Fatal("Failed to create a trackpad device: ", err)
	}
	defer tpw.Close(ctx)
	tw, err := tpw.NewMultiTouchWriter(2)
	if err != nil {
		s.Fatal("Failed to create a multi touch writer: ", err)
	}
	defer tw.Close()

	// Create a virtual keyboard.
	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kw.Close(ctx)

	ui := uiauto.New(tconn)

	// Find the web view of Meet window.
	webview := nodewith.ClassName("ContentsWebView").Role(role.WebView)

	// Check and grant permissions.
	if err := prompts.ClearPotentialPrompts(
		tconn,
		time.Minute,
		prompts.ShowNotificationsPrompt,
		prompts.AllowAVPermissionPrompt,
		prompts.AllowMicrophoneAndCameraPermissionPrompt,
	)(ctx); err != nil {
		s.Fatal("Failed to grant permissions: ", err)
	}

	rightSnapAllWindows := func() error {
		if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
			return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateSecondarySnapped)
		}); err != nil {
			return errors.Wrap(err, "failed to turn all windows into right snapped state")
		}
		return nil
	}

	leftSnapNonRightSnappedWindows := func() error {
		if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
			if w.State != ash.WindowStateSecondarySnapped {
				return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStatePrimarySnapped)
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to turn non right snapped windows into left snapped state")
		}
		return nil
	}

	ensureElementGetsScrolled := func(ctx context.Context, conn *chrome.Conn, element string) error {
		s.Log("Ensure element gets scrolled")
		var scrollTop int
		if err := conn.Eval(ctx, fmt.Sprintf("parseInt(%s.scrollTop)", element), &scrollTop); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get the number of pixels that the scrollbar is scrolled vertically"))
		}
		if scrollTop == 0 {
			return errors.Errorf("%s is not getting scrolled", element)
		}
		return nil
	}

	pv := perf.NewValues()
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a new CUJ recorder: ", err)
	}
	defer func() {
		if err := recorder.Close(closeCtx); err != nil {
			s.Error("Failed to stop recorder: ", err)
		}
	}()

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	// Add an empty screenshot recorder.
	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	if err := recorder.Run(ctx, func(ctx context.Context) (retErr error) {
		// Hide notifications so that they won't overlap with other UI components.
		if err := ash.CloseNotifications(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all notifications")
		}
		shareMessage := "Share this info with people you want in the meeting"
		if ui.WaitUntilExists(nodewith.Name(shareMessage).Ancestor(webview))(ctx) == nil {
			// "Share this code" popup appears, dismissing by close button.
			if err := uiauto.Combine(
				"click the close button and wait for the popup to disappear",
				pc.Click(nodewith.Name("Close").Role(role.Button).Ancestor(webview)),
				ui.WaitUntilGone(nodewith.Name(shareMessage).Ancestor(webview)),
			)(ctx); err != nil {
				return err
			}
		}

		if err := meetConn.WaitForExpr(ctx, "hrTelemetryApi.isInMeeting()"); err != nil {
			return errors.Wrap(err, "failed to wait for entering meeting")
		}
		if err := meetConn.Eval(ctx, "hrTelemetryApi.setMicMuted(false)", nil); err != nil {
			return errors.Wrap(err, "failed to turn on mic")
		}
		if err := meetConn.Eval(ctx, "hrTelemetryApi.setCameraMuted(false)", nil); err != nil {
			return errors.Wrap(err, "failed to turn on camera")
		}

		var participantCount int
		if err := meetConn.Eval(ctx, "hrTelemetryApi.getParticipantCount()", &participantCount); err != nil {
			return errors.Wrap(err, "failed to get participant count")
		}
		if participantCount != 2 {
			return errors.Errorf("got %d participants, expected 2", participantCount)
		}

		// Hide notifications so that they won't overlap with other UI components.
		if err := ash.CloseNotifications(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all notifications")
		}
		if err := meetConn.Eval(ctx, fmt.Sprintf("hrTelemetryApi.set%sLayout()", meetLayout), nil); err != nil {
			return errors.Wrapf(err, "failed to set %s layout", meetLayout)
		}

		if err := rightSnapAllWindows(); err != nil {
			return err
		}

		if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
			return errors.Wrap(err, "failed to do Ash workflows")
		}

		// See go/trace-in-cuj-tests about rules for tracing.
		if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
			return errors.Wrap(err, "failed to start tracing")
		}

		// 1. Multi-tasking with Google Docs by opening a large Docs file and scrolling through the file.
		// ================================================================================

		docsURL, err := cuj.GetTestDocURL(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get Google Doc URL")
		}
		docsConn, err := cs.NewConn(ctx, docsURL, browser.WithNewWindow())
		if err != nil {
			return errors.Wrap(err, "failed to open the google docs website")
		}
		defer docsConn.Close()
		defer docsConn.CloseTarget(closeCtx)
		defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), func() bool { return retErr != nil }, cr, "docs_dump")

		// Left snap the Docs window.
		if err := leftSnapNonRightSnappedWindows(); err != nil {
			return err
		}

		// Click "Got it" to close the pop-up if it exists.
		docsRootWebArea := nodewith.NameContaining("Google Docs").Role(role.RootWebArea)
		gotItButton := nodewith.NameContaining("Got it").Role(role.Button).Ancestor(docsRootWebArea)
		if err := uiauto.IfSuccessThen(ui.WithTimeout(10*time.Second).WaitUntilExists(gotItButton), ui.LeftClick(gotItButton))(ctx); err != nil {
			return errors.Wrap(err, "failed to click the Got it button")
		}

		if err := cuj.DismissCriticalSecurityAlert(ctx, tconn, docsConn); err != nil {
			return errors.Wrap(err, "failed to dismiss critical security alert")
		}

		// Move mouse to the google docs website window.
		docsCanvas := nodewith.Role(role.Canvas).Ancestor(docsRootWebArea).Onscreen().First()
		if err := ui.MouseMoveTo(docsCanvas, 0)(ctx); err != nil {
			return errors.Wrap(err, "failed to move mouse to the google docs website")
		}
		// Capture screenshot to see if the cursor is on the docs canvas.
		recorder.CustomScreenshot(ctx)

		// Scroll down the Docs file.
		s.Logf("Scrolling down the Google Docs file for %s", docsScrollTimeout)
		if err := inputsimulations.ScrollDownFor(ctx, tpw, tw, 500*time.Millisecond, docsScrollTimeout); err != nil {
			return err
		}

		// Ensure the file gets scrolled.
		if err := ensureElementGetsScrolled(ctx, docsConn, "document.getElementsByClassName('kix-appview-editor')[0]"); err != nil {
			return err
		}

		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := docsConn.Navigate(ctx, "chrome://version"); err != nil {
			return errors.Wrap(err, "failed to navigate to chrome://version")
		}

		if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
			return errors.Wrap(err, "failed to do Ash workflows")
		}

		// 2. Multi-tasking with Google Slides by opening a large Slides file and going through the deck.
		// ================================================================================

		slidesURL, err := cuj.GetTestSlidesURL(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get Google Slides URL")
		}
		slidesConn, err := cs.NewConn(ctx, slidesURL, browser.WithNewWindow())
		if err != nil {
			return errors.Wrap(err, "failed to open the google slides website")
		}
		defer slidesConn.Close()
		defer slidesConn.CloseTarget(closeCtx)

		// Left snap the Slides window.
		if err := leftSnapNonRightSnappedWindows(); err != nil {
			return err
		}

		// Go through the Slides deck.
		s.Logf("Going through the Google Slides file for %s", slidesScrollTimeout)
		if err := inputsimulations.RepeatKeyPressFor(ctx, kw, "Down", time.Second, slidesScrollTimeout); err != nil {
			return err
		}

		// Ensure the slides deck gets scrolled.
		if err := ensureElementGetsScrolled(ctx, slidesConn, "document.getElementsByClassName('punch-filmstrip-scroll')[0]"); err != nil {
			return err
		}

		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := slidesConn.Navigate(ctx, "chrome://version"); err != nil {
			return errors.Wrap(err, "failed to navigate to chrome://version")
		}

		if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
			return errors.Wrap(err, "failed to do Ash workflows")
		}

		// 3. Multi-tasking with Gmail by opening the inbox and scrolling down.
		// ================================================================================

		gmailConn, err := cs.NewConn(ctx, gmailURL, browser.WithNewWindow())
		if err != nil {
			return errors.Wrap(err, "failed to open the Gmail inbox")
		}
		defer gmailConn.Close()
		defer gmailConn.CloseTarget(closeCtx)

		// Left snap the Gmail window.
		if err := leftSnapNonRightSnappedWindows(); err != nil {
			return err
		}

		// Scroll down the Gmail inbox.
		s.Logf("Scrolling down the Gmail inbox for %s", gmailScrollTimeout)
		if err := inputsimulations.ScrollDownFor(ctx, tpw, tw, 500*time.Millisecond, gmailScrollTimeout); err != nil {
			return err
		}

		if err := recorder.StopTracing(ctx); err != nil {
			return errors.Wrap(err, "failed to stop tracing")
		}

		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := gmailConn.Navigate(ctx, "chrome://version"); err != nil {
			return errors.Wrap(err, "failed to navigate to chrome://version")
		}

		if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
			return errors.Wrap(err, "failed to do Ash workflows")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
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
}
