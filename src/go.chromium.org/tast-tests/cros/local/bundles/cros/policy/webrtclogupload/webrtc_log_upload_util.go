// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtclogupload

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// EventLogCollectionHashID is hashcode of network annotation webrtc_event_log_uploader.
	EventLogCollectionHashID = "24186190"
	// TextLogCollectionHashID is hashcode of network annotation webrtc_log_upload.
	TextLogCollectionHashID = "62443804"
)

// TestCase defines test expectations based on the value of policy.
type TestCase struct {
	Name                  string
	AnnotationLogExpected bool
	Policies              []policy.Policy // policies to check.
}

// AnnotationTestParam contains objects from test setup.
type AnnotationTestParam struct {
	AnnotationHashCode string
	Bt                 browser.Type
	Creds              string
	Tc                 TestCase
	Timeout            time.Duration
	Interval           time.Duration
}

// NetLogAnnotationTest starts and stops net export, trigger WebRTC logs and
// polls for appearance of annotation hashcode in net export log.
func NetLogAnnotationTest(ctx context.Context, fdms *fakedms.FakeDMS, cr *chrome.Chrome,
	tconn *chrome.TestConn, runParam AnnotationTestParam) (err error) {
	testCase := runParam.Tc
	closeCtx := ctx

	// Perform cleanup.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		return errors.Wrap(err, "failed to clean up")
	}

	// Update policies.
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, testCase.Policies); err != nil {
		return errors.Wrap(err, "failed to update policies")
	}

	// Setup browser and connSource by type of browser.
	var cs ash.ConnSource
	var br *browser.Browser
	switch runParam.Bt {
	case browser.TypeLacros:
		// Launch lacros.
		l, err := lacros.Launch(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to launch lacros")
		}
		defer l.Close(closeCtx)
		cs = l
		br = l.Browser()
	case browser.TypeAsh:
		cs = cr
		br = cr.Browser()
	}

	// Open the net-export page and start logging.
	if err := annotations.StartLogging(ctx, cr, br); err != nil {
		return errors.Wrap(err, "failed to start logging")
	}

	if err := TriggerWebRTCLogUploads(ctx, cr, br, tconn, runParam.Creds, cs); err != nil {
		return errors.Wrap(err, "failed to launch Meet client")
	}

	var errorCheckingLogs error
	var foundAnnotation bool
	testing.Poll(ctx, func(ctx context.Context) error {
		foundAnnotation, errorCheckingLogs = annotations.CheckLogs(ctx, cr, runParam.AnnotationHashCode)

		if foundAnnotation || errorCheckingLogs != nil {
			return nil
		}

		// Annotation not found yet, keep polling.
		return errors.New("annotation ID not found yet")
	}, &testing.PollOptions{
		Timeout:  runParam.Timeout,
		Interval: runParam.Interval,
	})

	if errorCheckingLogs != nil {
		return errors.Wrap(errorCheckingLogs, "failed to check network logs")
	}

	// Stop logging.
	if err := annotations.StopLogging(ctx, cr, br); err != nil {
		return errors.Wrap(err, "failed to stop logging and check logs")
	}

	if testCase.AnnotationLogExpected != foundAnnotation {
		return errors.Errorf("unexpected annotation; expected: %t, got: %t", testCase.AnnotationLogExpected, foundAnnotation)
	}

	return nil
}

// TriggerWebRTCLogUploads is used to create a Meet client and trigger feedback to create WebRTC logs.
func TriggerWebRTCLogUploads(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, tconn *chrome.TestConn, creds string, connSource ash.ConnSource) (errr error) {
	const (
		createConfTimeout = 30 * time.Second
		meetTimeout       = 10 * time.Minute
	)
	closeCtx := ctx

	// Ensure that we close the Meet window at the end of the test in case
	// the test fails.
	closedMeet := false
	defer func() {
		if closedMeet {
			return
		}
		// Close the windows to finish the meeting.
		if err := ash.CloseAllWindows(closeCtx, tconn); err != nil {
			errors.Wrap(err, "failed to close all windows")
		}
	}()

	const addBotTimeout = 40 * time.Second
	botCtx, cancel := context.WithTimeout(ctx, addBotTimeout)
	defer cancel()

	bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
	if err != nil {
		return errors.Wrap(err, "failed to create a bond client")
	}
	defer bc.Close()

	var meetingCode string
	func() {
		// Request the bond server to create conference.
		meetCtx, cancel := context.WithTimeout(ctx, createConfTimeout)
		defer cancel()
		meetingCode, err = bc.CreateConference(meetCtx)
		if err != nil {
			errors.Wrap(err, "failed to create a conference room")
		}
	}()

	defer func(ctx context.Context) {
		bc.RemoveAllBots(ctx, meetingCode)
	}(closeCtx)

	wait := 100 * time.Millisecond
	for i := 0; i < 3; i++ {
		// GoBigSleepLint: A short sleep before next call to Bond API.
		if err := testing.Sleep(ctx, wait); err != nil {
			return errors.Wrapf(err, "failed to sleep for %v", wait)
		}
		// Exponential backoff. The wait time is 0.1s, 1s and 10s before each retry.
		wait *= 10
		// Add 5 minutes to the bot duration, to ensure that the bots stay long
		// enough to complete the test.
		botList, numFailures, err := bc.AddBots(botCtx, meetingCode, 1, meetTimeout+5*time.Minute)
		if err != nil {
			return errors.Wrap(err, "failed to create bot")
		}

		testing.ContextLogf(ctx, "%d bots started, %d bots failed", len(botList), numFailures)
		if numFailures == 0 {
			break
		}
	}

	meetHelper := googlemeet.NewHRTelemetryHelper(connSource, tconn)

	if err := meetHelper.JoinMeeting(ctx, meetingCode, browser.WithNewWindow()); err != nil {
		return errors.Wrap(err, "failed to open the hangout Meet website")
	}
	defer meetHelper.Close(closeCtx)

	// Match window titles `Google Meet` and `meet.google.com`.
	meetRE := regexp.MustCompile(`\bMeet\b|\bmeet\.\b`)
	meetWindow, err := ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return meetRE.MatchString(w.Title) })
	if err != nil {
		return errors.Wrap(err, "failed to find the Meet window")
	}

	if err := ensureMicAndCamAllowed(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to ensure microphone and camera are allowed")
	}

	const longUITimeout = time.Minute
	// Check and grant permissions.
	if err := prompts.ClearPotentialPrompts(tconn, longUITimeout, prompts.ShowNotificationsPrompt, prompts.AllowAVPermissionPrompt)(ctx); err != nil {
		return errors.Wrap(err, "failed to grant permissions")
	}

	// Verify that user has entered the meeting.
	if err := meetHelper.IsInMeeting(ctx, 10*time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait to enter the meeting")
	}

	// Send feedback from meet to generate WebRTC log content.
	if err := sendSuggestionFeedBack(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to create log content")
	}

	closedMeet = true
	if err := meetWindow.CloseWindow(closeCtx, tconn); err != nil {
		return errors.Wrap(err, "failed to close the meeting")
	}

	return nil
}

// ensureMicAndCamAllowed checks if modal is present on Meets app and
// permits microphone and camera use.
func ensureMicAndCamAllowed(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	allowCam := nodewith.Name("Allow camera").Role(role.Button)
	allowMicCam := nodewith.Name("Allow microphone and camera").Role(role.Button)

	// GoBigSleepLint: Wait to let modal become available on screen.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for modal")
	}

	// Click the "Allow camera" button if present.
	if err := ui.Exists(allowCam)(ctx); err == nil {
		if err := uiauto.Combine("Click 'Allow camera' button",
			ui.DoDefault(allowCam),
			ui.WaitUntilGone(allowCam),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'Allow camera' button")
		}
	}

	// Click the "Allow microphone and camera" button if present.
	if err := ui.Exists(allowMicCam)(ctx); err == nil {
		if err := uiauto.Combine("Click 'Allow microphone and camera' button",
			ui.DoDefault(allowMicCam),
			ui.WaitUntilGone(allowMicCam),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'Allow microphone and camera' button")
		}
	}

	return nil
}

// sendSuggestionFeedBack helps to create content in WebRTC text and event logs.
// Log file should have some content to trigger log upload and annotation.
func sendSuggestionFeedBack(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	uiLongWait := ui.WithTimeout(time.Minute)

	meetRootWebArea := nodewith.NameContaining("Meet").Role(role.RootWebArea)
	moreOptions := nodewith.Name("More options").Role(role.PopUpButton)
	reportProblem := nodewith.Name("Report a problem").Role(role.MenuItem)
	suggestIdea := nodewith.Name("Suggest an idea").Role(role.Button)
	issueDescriptionInput := nodewith.Role(role.TextField).Ancestor(meetRootWebArea)
	suggestSend := nodewith.Name("Send").Ancestor(meetRootWebArea).Role(role.Button)

	if err := uiauto.Combine("Click 'Report a problem' button",
		// More than one 'More options' buttons are present in Meets app with
		// same name and properties.
		// First Button in ui tree is in participant picture area.
		// Required button in main viewing area is present second in order in ui.
		ui.DoDefault(moreOptions.Nth(1)),
		uiLongWait.WaitUntilExists(reportProblem),

		ui.DoDefault(reportProblem),
		uiLongWait.WaitUntilExists(suggestIdea),

		ui.DoDefault(suggestIdea),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to trigger suggestion")
	}

	if err := ui.EnsureFocused(issueDescriptionInput)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the issue description text input")
	}

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(ctx)

	// Enter issue description.
	description := `Writing a long description so that test does not run into "Add more details" warning message. Please ignore this text.`
	if err := kb.Type(ctx, description); err != nil {
		return errors.Wrap(err, "failed to enter issue description")
	}

	if err := uiLongWait.RetryUntil(ui.LeftClick(suggestSend), ui.Gone(suggestSend))(ctx); err != nil {
		return errors.Wrap(err, "failed to click Send on Suggestion window")
	}

	feedbackManager := nodewith.ClassNameRegex(regexp.MustCompile("uvFeedbackThankyoupageroot.*")).Role(role.GenericContainer)
	suggestClose := nodewith.Name("Close").Role(role.Button).Focusable().Ancestor(feedbackManager).First()

	if err := uiauto.Combine("Click 'Suggest close' button",
		ui.WaitUntilExists(suggestClose),
		ui.LeftClick(suggestClose),
		ui.WaitUntilGone(suggestClose),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to close Suggestion window")
	}

	callEndButton := nodewith.NameRegex(regexp.MustCompile("Leave call.*")).Role(role.Button)
	if err := uiauto.Combine("Click 'Leave call' button",
		uiLongWait.WaitUntilExists(callEndButton),
		uiLongWait.WaitUntilEnabled(callEndButton),
		ui.LeftClick(callEndButton),
		ui.WaitUntilGone(callEndButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to end Meet call")
	}

	return nil
}
