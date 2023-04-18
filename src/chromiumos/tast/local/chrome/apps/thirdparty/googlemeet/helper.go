// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
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
}

// HRTelemetryHelper helps to perform Meet operations with hrTelemetryApi.
type HRTelemetryHelper struct {
	cs       ash.ConnSource
	tconn    *chrome.TestConn
	meetConn *chrome.Conn
}

// NewHRTelemetryHelper returns a new HRTelemetryHelper object.
func NewHRTelemetryHelper(cs ash.ConnSource, tconn *chrome.TestConn) *HRTelemetryHelper {
	return &HRTelemetryHelper{
		cs:    cs,
		tconn: tconn,
	}
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
	err := h.meetConn.WaitForExprWithTimeout(ctx, "hrTelemetryApi.isInMeeting()", time.Minute)
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
		errAPINotDefined    = "hrTelemetryApi is not defined"
		errSignedOut        = "the account has been signed out"
		errConnectionFailed = "failed to connect to meeting room"
	)

	// Use string comparison because error loses its type after wrapping.
	// If the error doesn't contain "hrTelemetryApi is not defined", the original error is returned.
	if !strings.Contains(err.Error(), errAPINotDefined) {
		return err
	}

	ui := uiauto.New(h.tconn)
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
	return h.meetConn.Eval(ctx, fmt.Sprintf("hrTelemetryApi.setCameraMuted(%t)", !expectedOn), nil)
}

// GetParticipantCount gets the number of the participants in the meeting room.
func (h *HRTelemetryHelper) GetParticipantCount(ctx context.Context) (int, error) {
	var participantCount int
	if err := h.meetConn.Eval(ctx, "hrTelemetryApi.getParticipantCount()", &participantCount); err != nil {
		return 0, err
	}
	return participantCount, nil
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
	return h.meetConn.Eval(ctx, "hrTelemetryApi.presentation.presentTab()", nil)
}

var _ MeetHelper = (*HRTelemetryHelper)(nil)
