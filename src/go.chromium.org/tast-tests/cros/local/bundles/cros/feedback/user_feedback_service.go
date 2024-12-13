// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package feedback

import (
	"context"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/tast-tests/cros/local/apps"
	fpb "go.chromium.org/tast-tests/cros/local/bundles/cros/feedback/proto"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/feedbackapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/input"
	pb "go.chromium.org/tast-tests/cros/services/cros/feedback"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var userFeedbackService UserFeedbackService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			userFeedbackService = UserFeedbackService{sharedObject: common.SharedObjectsForServiceSingleton}
			pb.RegisterUserFeedbackServiceServer(srv, &userFeedbackService)
		},
		GuaranteeCompatibility: true,
	})
}

// UserFeedbackService implements tast.cros.feedback.UserFeedbackService
// It provides access to user feedback report.
type UserFeedbackService struct {
	sharedObject *common.SharedObjectsForService
	mutex        sync.Mutex
}

// GetUserFeedback retrieves user feedback report.
func (svc *UserFeedbackService) GetUserFeedback(ctx context.Context, req *pb.GetUserFeedbackRequest) (*pb.GetUserFeedbackResponse, error) {
	//Follow the same locking sequence from svc lock to shared object lock to avoid deadlock
	svc.mutex.Lock()
	defer svc.mutex.Unlock()
	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	cr := svc.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	// Clean up in both beginning and the end.
	os.RemoveAll(feedbackapp.ReportPath)
	defer os.RemoveAll(feedbackapp.ReportPath)

	ui := uiauto.New(tconn)

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(ctx)

	// Trigger feedback app with shortcuts and poll until the app is launched.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Launch Feedback app with alt+shift+i.
		if err := kb.Accel(ctx, "Alt+Shift+I"); err != nil {
			return errors.Wrap(err, "failed pressing alt+shift+i")
		}
		// Verify Feedback app is launched.
		if err = ash.WaitForApp(ctx, tconn, apps.Feedback.ID, 20*time.Second); err != nil {
			return errors.Wrap(err, "could not find app in shelf after launch")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed launching Feedback app")
	}
	if err := feedbackapp.VerifyFeedbackAppIsLaunched(ctx, tconn, ui); err != nil {
		return nil, errors.Wrap(err, "failed to verify that the Feedback app is launched")
	}

	// Find the issue description text input.
	const inputName = "Description Suggestions are based on your description"
	issueDescriptionInput := nodewith.NameStartingWith(inputName)

	// On some devices, there is some timing difference after waitForApp and the
	// time UI tree is updated with the "Description Suggestions are based on your
	// description" textfield. So adding the waitUntilExists before ensuring
	// focus.
	if err := ui.WaitUntilExists(issueDescriptionInput)(ctx); err != nil {
		return nil, errors.Wrap(err, "could not find the description field")
	}

	if err := ui.EnsureFocused(issueDescriptionInput)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find the issue description text input")
	}

	// Type issue description.
	if err := kb.Type(ctx, feedbackapp.IssueText); err != nil {
		return nil, errors.Wrap(err, "failed to type issue description")
	}

	// Find continue button and click.
	button := nodewith.Name("Continue").Role(role.Button)
	if err := ui.DoDefault(button)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to click continue button")
	}

	// Submit the feedback and verify confirmation page title exists.
	sendButton := nodewith.Name("Send").Role(role.Button)
	confirmationPageTitle := nodewith.Name("Thanks for your feedback").Role(
		role.StaticText)

	if err := uiauto.Combine("Submit feedback and verify",
		ui.DoDefault(sendButton),
		ui.WaitUntilExists(confirmationPageTitle),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to submit feedback and verify")
	}

	// Click Done button to close the dialog
	doneButton := nodewith.Name("Done").Role(role.Button)
	if err := ui.DoDefault(doneButton)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to close the dialog")
	}

	// Read feedback report content.
	var content []byte
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		content, err = os.ReadFile(feedbackapp.ReportPath)
		if err != nil {
			return errors.Wrap(err, "failed to read user report report content")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to read user report report content")
	}

	// Convert user feedback proto message into json format and return to caller.
	report := &fpb.ExtensionSubmit{}
	if err = proto.Unmarshal(content, report); err != nil {
		return nil, errors.Wrap(err, "failed to parse user feedback report")
	}
	marshaler := protojson.MarshalOptions{Indent: "  "}
	userFeedbackJSON := marshaler.Format(report)

	res := &pb.GetUserFeedbackResponse{
		UserFeedback: userFeedbackJSON,
	}

	return res, nil
}
