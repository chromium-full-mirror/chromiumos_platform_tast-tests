// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
)

// MeetHelper is an interface defines the operations performed in MeetCUJ.
type MeetHelper interface {
	JoinMeeting(ctx context.Context, meetingCode string, opts ...browser.CreateTargetOption) error
	Close(ctx context.Context) error
	IsInMeeting(ctx context.Context, timeout time.Duration) error
	SetMicrophone(ctx context.Context, expectedOn bool) error
	SetCamera(ctx context.Context, expectedOn bool) error
	GetParticipantCount(ctx context.Context) (participantCount int, err error)
	ChangeLayoutOption(ctx context.Context, layoutOption LayoutOption) error
	SetSendResolution720p(ctx context.Context) error
	SetReceiveResolution720p(ctx context.Context) error
	OpenPresentDialog(ctx context.Context) error
	PresentTab(ctx context.Context, conn *chrome.Conn, kw *input.KeyboardEventWriter, presentTabTitle string) error
}

// HRTelemetryHelper helps to perform Meet operations with hrTelemetryApi.
type HRTelemetryHelper struct {
	cs           ash.ConnSource
	tconn        *chrome.TestConn
	meetConn     *chrome.Conn
	ui           *uiauto.Context
	isPresenting bool
}

var (
	stopPresentingRe = regexp.MustCompile("(Stop presenting|Stop sharing)")
	// StopPresentingButton is the finder for stop presenting button.
	StopPresentingButton = nodewith.NameRegex(stopPresentingRe).Role(role.Button).First()
)

// NewHRTelemetryHelper returns a new HRTelemetryHelper object.
func NewHRTelemetryHelper(cs ash.ConnSource, tconn *chrome.TestConn) *HRTelemetryHelper {
	ui := uiauto.New(tconn)
	return &HRTelemetryHelper{
		cs:    cs,
		tconn: tconn,
		ui:    ui,
	}
}

// JoinMeetingWithDisabledExperiments joins the meeting room with the passed in
// list of experiments disabled.
func (h *HRTelemetryHelper) JoinMeetingWithDisabledExperiments(ctx context.Context, meetingCode string, disabledExperiments []string, opts ...browser.CreateTargetOption) (err error) {
	if h.meetConn != nil {
		return errors.New("already joined a meeting")
	}

	// We need to navigate to the Meet window first when using the "e" URL
	// parameter. This probably has to do with verifying that the account
	// is an allowlisted account - the e parameter is only valid for
	// specific accounts, and if we try to use it before Meet sees its an
	// internal account, it would reject the URL as malformed.
	h.meetConn, err = h.cs.NewConn(ctx, "https://meet.google.com", opts...)
	if err != nil {
		return errors.Wrap(err, "failed to navigate to the Meet homepage")
	}

	if err := webutil.ReloadIfSiteUnreachable(h.ui, h.meetConn, meetRootWebArea)(ctx); err != nil {
		return errors.Wrap(err, "failed to reload the page")
	}

	startTime := time.Now()
	if err := webutil.WaitForQuiescence(ctx, h.meetConn, time.Minute); err != nil {
		testing.ContextLog(ctx, "Failed to wait for Meet homepage to quiesce: ", err)
		outdir, ok := testing.ContextOutDir(ctx)
		if !ok {
			testing.ContextLog(ctx, "Failed to get OutDir")
		} else {
			path := filepath.Join(outdir, "meet_homepage.jpg")
			if err := screenshot.Capture(ctx, path); err != nil {
				testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
			}
		}
	}
	testing.ContextLog(ctx, "Loading Meet homepage took: ", time.Since(startTime))

	// Experiments in the url are disabled using the e= parameter, where
	// each experiment is prefixed with - to mark it as disabled.
	if err := h.meetConn.Navigate(ctx, fmt.Sprintf(
		"https://meet.google.com/%s/?e=-%s",
		meetingCode,
		strings.Join(disabledExperiments, ",-"),
	)); err != nil {
		return err
	}

	if err := webutil.WaitForQuiescence(ctx, h.meetConn, 30*time.Second); err != nil {
		testing.ContextLog(ctx, "Failed to wait for meeting page to quiesce: ", err)
	}
	return nil
}

// JoinMeeting joins the meeting room with the conn source.
func (h *HRTelemetryHelper) JoinMeeting(ctx context.Context, meetingCode string, opts ...browser.CreateTargetOption) (err error) {
	if h.meetConn != nil {
		return errors.New("already joined a meeting")
	}
	h.meetConn, err = h.cs.NewConn(ctx, "https://meet.google.com/"+meetingCode, opts...)
	return err
}

// Close closes the connection to the Meet page. The web page is not closed by this function.
func (h *HRTelemetryHelper) Close(ctx context.Context) error {
	if h.meetConn == nil {
		testing.ContextLog(ctx, "The Meet connection is already closed")
		return nil
	}
	return h.meetConn.Close()
}

// IsInMeeting checks whether the connection is in a meeting room.
// hrTelemetryApi is defined only after granting video permissions.
func (h *HRTelemetryHelper) IsInMeeting(ctx context.Context, timeout time.Duration) error {
	if err := webutil.WaitForQuiescence(ctx, h.meetConn, time.Minute); err != nil {
		testing.ContextLog(ctx, "Failed to wait for meet page to achieve quiescence: ", err)
	}
	err := h.meetConn.WaitForExprWithTimeout(ctx, "hrTelemetryApi.isInMeeting()", timeout)
	return h.checkError(ctx, err)
}

// checkError checks the actual error message when hrTelemetryApi is not defined.
// If the account is signed out, wraps the given error with signed out error.
// If there is connection message, wraps the given error with connection error.
// If any other error occurs, the original error will be returned.
func (h *HRTelemetryHelper) checkError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	const (
		errIsNotInMeeting     = "\"!!(hrTelemetryApi.isInMeeting())\" is false"
		errAPINotDefined      = "hrTelemetryApi is not defined"
		errSignedOut          = "the account has been signed out"
		errConnectionFailed   = "failed to connect to meeting room"
		errNavigateToHomePage = "navigate to the meet home page for unknown reason"
	)

	ui := h.ui
	// Wrap error message navigating to the meet home page for unknown reason.
	if strings.Contains(err.Error(), errIsNotInMeeting) {
		newMeetingButton := nodewith.Name("New meeting").Role(role.Button)
		if ui.Exists(newMeetingButton)(ctx) == nil {
			return errors.Wrap(err, errNavigateToHomePage)
		}
		return err
	}

	// Use string comparison because error loses its type after wrapping.
	// If the error doesn't contain "hrTelemetryApi is not defined", the original error is returned.
	if !strings.Contains(err.Error(), errAPINotDefined) {
		return err
	}

	// There may be multiple connection messages with same ancestor, so add First() here.
	connectionMessage := nodewith.Name("Still trying to get in...").Role(role.StaticText).First()
	signInLink := nodewith.Name("Sign in").Role(role.Link)
	signInButton := nodewith.Name("Sign in").Role(role.Button)
	signedOutMessages := nodewith.NameRegex(regexp.MustCompile("(Sign in to add a Google account|You have been signed out).*")).First()
	errorNode, existsErr := ui.FindAnyExists(ctx, connectionMessage, signInLink, signInButton, signedOutMessages)
	// If there are no signout and connection errors, the original error will be returned.
	if existsErr != nil {
		return err
	}
	if errorNode == connectionMessage {
		return errors.Wrap(err, errConnectionFailed)
	}
	return errors.Wrap(err, errSignedOut)
}

// SetMicrophone sets the microphone to the given state.
func (h *HRTelemetryHelper) SetMicrophone(ctx context.Context, expectedOn bool) error {
	return h.meetConn.Eval(ctx, fmt.Sprintf("hrTelemetryApi.setMicMuted(%t)", expectedOn), nil)
}

// SetCamera sets the camera to the given state.
func (h *HRTelemetryHelper) SetCamera(ctx context.Context, expectedOn bool) error {
	setCamera := func(ctx context.Context) error {
		testing.ContextLog(ctx, "Set camera to ", expectedOn)
		return h.meetConn.Eval(ctx, fmt.Sprintf("hrTelemetryApi.setCameraMuted(%t)", !expectedOn), nil)
	}
	ui := h.ui
	cameraName := "Turn off camera"
	if !expectedOn {
		cameraName = "Turn on camera"
	}
	// The camera button's role is sometimes a toggle button and sometimes
	// a button, so the role is not included here.
	cameraButton := nodewith.Name(cameraName).Ancestor(meetRootWebArea)
	if err := uiauto.IfSuccessThen(ui.Gone(cameraButton),
		ui.RetryUntil(setCamera, ui.WithTimeout(5*time.Second).WaitUntilExists(cameraButton)))(ctx); err != nil {
		return errors.Wrapf(err, "failed to set camera to %t", expectedOn)
	}
	return nil
}

// GetParticipantCount gets the number of the participants in the meeting room.
func (h *HRTelemetryHelper) GetParticipantCount(ctx context.Context) (int, error) {
	var participantCount int
	if err := h.meetConn.Eval(ctx, "hrTelemetryApi.getParticipantCount()", &participantCount); err != nil {
		return 0, err
	}
	return participantCount, nil
}

// CheckParticipantCount verifies that the number of participants matches the expected count.
func (h *HRTelemetryHelper) CheckParticipantCount(ctx context.Context, expectedCount int) error {
	participantCount, err := h.GetParticipantCount(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get participant count")
	}
	if participantCount != expectedCount {
		return errors.Errorf("got %d participants, expected %d", participantCount, expectedCount)
	}
	return nil
}

// ChangeLayoutOption changes the layout to the given option.
func (h *HRTelemetryHelper) ChangeLayoutOption(ctx context.Context, layoutOption LayoutOption) error {
	return h.meetConn.Eval(ctx, fmt.Sprintf("hrTelemetryApi.set%sLayout()", layoutOption), nil)
}

// SetSendResolution720p sets the stream sending resolution to 720p.
func (h *HRTelemetryHelper) SetSendResolution720p(ctx context.Context) error {
	return h.meetConn.Eval(ctx, "hrTelemetryApi.streamQuality.send720p()", nil)
}

// SetReceiveResolution720p sets the stream receiving resolution to 720p.
func (h *HRTelemetryHelper) SetReceiveResolution720p(ctx context.Context) error {
	return h.meetConn.Eval(ctx, "hrTelemetryApi.streamQuality.receive720p()", nil)
}

// OpenPresentDialog opens the presentation dialog with "A tab" mode.
func (h *HRTelemetryHelper) OpenPresentDialog(ctx context.Context) error {
	ui := h.ui
	present := func(ctx context.Context) error {
		alertDialog := nodewith.Name("Can't share your screen").Role(role.AlertDialog)
		okText := nodewith.Name("OK").Role(role.StaticText).Ancestor(alertDialog)
		if err := uiauto.IfSuccessThen(ui.Exists(alertDialog), ui.DoDefault(okText))(ctx); err != nil {
			return err
		}
		return h.meetConn.Eval(ctx, "hrTelemetryApi.presentation.present()", nil)
	}

	chromeTab := nodewith.Name("Chrome Tab").Role(role.Tab)
	if err := ui.WithTimeout(time.Minute).RetryUntil(present, ui.WaitUntilExists(chromeTab))(ctx); err != nil {
		return errors.Wrap(err, "failed to find the screen-sharing popup")
	}
	return nil
}

// PresentTab presents the tab with |presentTabTitle|.
func (h *HRTelemetryHelper) PresentTab(ctx context.Context, conn *chrome.Conn, kw *input.KeyboardEventWriter, presentTabTitle string) error {
	ui := h.ui
	if err := ui.Exists(StopPresentingButton)(ctx); err == nil {
		return nil
	}

	if err := h.OpenPresentDialog(ctx); err != nil {
		return errors.Wrap(err, "failed to start to present a tab")
	}

	// Select the tab to present.
	waitForPresentTabFocus := ui.WithTimeout(5 * time.Second).WaitUntilExists(nodewith.NameContaining(presentTabTitle).HasClass("AXVirtualView").Focused())
	if err := uiauto.NamedCombine(fmt.Sprintf("select tab %q to screenshare", presentTabTitle),
		ui.EnsureFocused(nodewith.HasClass("TableView").Role(role.ListGrid)),
		// If the presenting tab is not focused, press the down
		// arrow until it is.
		uiauto.IfFailThen(
			waitForPresentTabFocus,
			ui.RetryUntil(
				kw.AccelAction("Down"),
				waitForPresentTabFocus,
			),
		),
		kw.AccelAction("Enter"),
		// Some low-end DUTs may take a long time to actually get to
		// the presenting page. Wait for the "Stop presenting" to appear
		// to ensure the page is being shared.
		ui.WithTimeout(time.Minute).WaitUntilExists(StopPresentingButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to select the tab to share")
	}

	startTime := time.Now()
	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		testing.ContextLog(ctx, "Ignoring waiting for page to quiesce: ", err)
	} else {
		testing.ContextLog(ctx, "Loading page took: ", time.Since(startTime))
	}

	h.isPresenting = true

	return nil
}

// StopPresenting stops presenting in Google Meet.
func (h *HRTelemetryHelper) StopPresenting(ctx context.Context) error {
	ui := h.ui
	if !h.isPresenting || ui.Gone(StopPresentingButton) == nil {
		return errors.New("failed to stop presenting, because no screenshare is active")
	}

	stopPresenting := uiauto.NamedAction("stop presenting",
		ui.WithTimeout(time.Minute).DoDefaultUntil(
			StopPresentingButton,
			ui.WaitUntilGone(StopPresentingButton),
		))
	return uiauto.Retry(3,
		uiauto.IfSuccessThenWithLog(
			ui.Exists(StopPresentingButton),
			stopPresenting,
		))(ctx)
}

var _ MeetHelper = (*HRTelemetryHelper)(nil)
