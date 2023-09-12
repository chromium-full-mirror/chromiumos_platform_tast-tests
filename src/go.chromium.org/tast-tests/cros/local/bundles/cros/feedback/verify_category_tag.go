// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package feedback

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/apps"
	fpb "go.chromium.org/tast-tests/cros/local/bundles/cros/feedback/proto"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/feedbackapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const cameraCategoryTag = "chromeos-camera-app"

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyCategoryTag,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify category_tag value in the report",
		Contacts: []string{
			"cros-feedback-app@google.com",
			"xiangdongkong@google.com",
		},
		// ChromeOS > Data > Engineering > Feedback
		BugComponent: "b:1033360",
		Fixture:      "chromeLoggedInWithOsFeedbackSaveReportToLocalForE2ETesting",
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-3f028d06-0100-4b5b-b1f3-99ceeaf3d62b",
			},
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", caps.BuiltinCamera},
		Timeout:      5 * time.Minute,
	})
}

// VerifyCategoryTag verifies the category_tag value in the report. Open Feedback
// app from the Camera app, the category_tag in the report should be chromeos-camera-app.
func VerifyCategoryTag(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	// Clean up in both beginning and the end.
	cleanUp := func() {
		if err := os.RemoveAll(feedbackapp.ReportPath); err != nil {
			s.Error("Failed to remove feedback report: ", err)
		}
	}
	cleanUp()
	defer cleanUp()

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Open Camera app.
	if err := apps.Launch(ctx, tconn, apps.Camera.ID); err != nil {
		s.Fatal("Failed to launch the Camera app: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.Camera.ID, time.Minute); err != nil {
		s.Fatal("Failed to wait for the Camera app")
	}

	// Open feedback app in the Camera app.
	cameraWebArea := nodewith.NameContaining("Camera").Role(role.RootWebArea)
	settingsButton := nodewith.Name("Settings").Ancestor(cameraWebArea)
	sendFeedbackButton := nodewith.Name("Send feedback").Role(role.Button)

	// On some devices, there is delay for the camera app displaying the settings
	// button. Add the WaitUntilExists before clicking the button.
	if err := ui.WaitUntilExists(settingsButton)(ctx); err != nil {
		s.Fatal("Could not find the settings button: ", err)
	}

	if err := ui.DoDefault(settingsButton)(ctx); err != nil {
		s.Fatal("Failed to click Settings button: ", err)
	}
	if err := uiauto.Combine("Open feedback app in the Camera app",
		ui.DoDefault(settingsButton),
		ui.WaitUntilExists(sendFeedbackButton),
		ui.DoDefault(sendFeedbackButton),
	)(ctx); err != nil {
		s.Fatal("Failed to open feedback app in the Camera app: ", err)
	}

	// Verify Feedback app is launched.
	if err = ash.WaitForApp(ctx, tconn, apps.Feedback.ID, time.Minute); err != nil {
		s.Fatal("Could not find app in shelf after launch: ", err)
	}

	// Find the issue description text input.
	const inputName = "Description Suggestions are based on your description"
	issueDescriptionInput := nodewith.NameStartingWith(inputName)

	// On some devices, there is some timing difference after waitForApp and the
	// time UI tree is updated with the "Description Suggestions are based on your
	// description" textfield. So adding the waitUntilExists before ensuring
	// focus.
	if err := ui.WaitUntilExists(issueDescriptionInput)(ctx); err != nil {
		s.Fatal("Could not find the description field: ", err)
	}

	if err := ui.EnsureFocused(issueDescriptionInput)(ctx); err != nil {
		s.Fatal("Failed to find the issue description text input: ", err)
	}

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Type issue description.
	if err := kb.Type(ctx, feedbackapp.IssueText); err != nil {
		s.Fatal("Failed to type issue description: ", err)
	}

	// Find continue button and click.
	button := nodewith.Name("Continue").Role(role.Button)
	if err := ui.DoDefault(button)(ctx); err != nil {
		s.Fatal("Failed to click continue button")
	}

	// Submit the feedback and verify confirmation page title exists.
	sendButton := nodewith.Name("Send").Role(role.Button)
	confirmationPageTitle := nodewith.Name("Thanks for your feedback").Role(
		role.StaticText)

	if err := uiauto.Combine("Submit feedback and verify",
		ui.DoDefault(sendButton),
		ui.WaitUntilExists(confirmationPageTitle),
	)(ctx); err != nil {
		s.Fatal("Failed to submit feedback and verify: ", err)
	}

	// Read feedback report content.
	var content []byte

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		content, err = ioutil.ReadFile(feedbackapp.ReportPath)
		if err != nil {
			return errors.Wrap(err, "failed to read report content")
		}

		return nil
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		s.Fatal("Failed to read report content: ", err)
	}

	// Verify the category_tag value in the report.
	report := &fpb.ExtensionSubmit{}
	if err = proto.Unmarshal(content, report); err != nil {
		s.Fatal("Failed to parse report: ", err)
	}
	categoryTag := report.GetBucket()
	if categoryTag != cameraCategoryTag {
		s.Fatal("Failed to get the correct camera category tag")
	}
}
