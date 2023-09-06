// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package feedback

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchFeedbackFromPowerButton,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "User is able to launch feedback app from power button",
		Contacts: []string{
			"cros-feedback-app@google.com",
			"xiangdongkong@google.com",
		},
		// ChromeOS > Data > Engineering > Feedback
		BugComponent: "b:1033360",
		Attr:         []string{"group:mainline", "informational"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.inputs.KeyboardService",
			"tast.cros.ui.AutomationService",
		},
		// The feedback dep excludes a few boards where this test failed frequently.
		SoftwareDeps: []string{"chrome", "feedback"},
		Vars:         []string{"servo"},
		Timeout:      5 * time.Minute,
	})
}

// LaunchFeedbackFromPowerButton verifies launching feedback app from power button.
func LaunchFeedbackFromPowerButton(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	servoSpec := s.RequiredVar("servo")
	dut := s.DUT()

	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power on DUT at cleanup: ", err)
			}
		}
	}(cleanupCtx)

	loginChrome := func() (*rpc.Client, error) {
		cl, err := rpc.Dial(ctx, dut, s.RPCHint())
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to RPC service on DUT")
		}

		// Start Chrome on the DUT.
		cs := ui.NewChromeServiceClient(cl.Conn)
		loginReq := &ui.NewRequest{}
		if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		return cl, nil
	}

	// Perform initial Chrome login.
	cl, err := loginChrome()
	if err != nil {
		s.Fatal("Failed to login Chrome: ", err)
	}

	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)

	// The feedback app is a SWA. It may not be ready when clicking the feedback
	// button in the power menu. When the app does not show up, just try to launch
	// it again until the app is launched successfully or when it times out.
	launchApp := func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {

			// Press power button.
			if err := pxy.Servo().KeypressWithDuration(
				ctx, servo.PowerKey, servo.DurPress); err != nil {
				return errors.Wrap(err, "failed to power long press")
			}

			// Find feedback button and click.
			feedbackButtonFinder := &ui.Finder{
				NodeWiths: []*ui.NodeWith{
					{Value: &ui.NodeWith_Name{Name: "Feedback"}},
					{Value: &ui.NodeWith_First{First: true}},
				},
			}
			if _, err := uiautoSvc.WaitUntilExists(
				ctx, &ui.WaitUntilExistsRequest{Finder: feedbackButtonFinder}); err != nil {
				return errors.Wrap(err, "failed to find feedback button on DUT UI")
			}

			if _, err := uiautoSvc.LeftClick(
				ctx, &ui.LeftClickRequest{Finder: feedbackButtonFinder}); err != nil {
				return errors.Wrap(err, "failed to click feedback button")
			}

			// Verify issue description input exists.
			issueDescriptionInputFinder := &ui.Finder{
				NodeWiths: []*ui.NodeWith{
					{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_TEXT_FIELD}},
					{Value: &ui.NodeWith_First{First: true}},
				},
			}
			if _, err := uiautoSvc.WaitUntilExists(
				ctx, &ui.WaitUntilExistsRequest{
					Finder:  issueDescriptionInputFinder,
					Timeout: durationpb.New(100 * time.Millisecond)}); err != nil {
				return errors.Wrap(err, "failed to find feedback issue description input")
			}
			return nil
		}, &testing.PollOptions{Interval: 2 * time.Second, Timeout: 2 * time.Minute})
	}

	if err := launchApp(ctx); err != nil {
		s.Fatal("Failed to launch feedback app")
	}

	// Verify continue button exists.
	continueButtonFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Continue"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(
		ctx, &ui.WaitUntilExistsRequest{Finder: continueButtonFinder}); err != nil {
		s.Fatal("Failed to find feedback continue button: ", err)
	}

	// Verify five default help content links exist.
	iframeFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_IFRAME}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	for i := 0; i < 5; i++ {
		helpLinkFinder := &ui.Finder{
			NodeWiths: []*ui.NodeWith{
				{Value: &ui.NodeWith_Ancestor{Ancestor: iframeFinder}},
				{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_LINK}},
				{Value: &ui.NodeWith_Nth{Nth: int32(i)}},
			},
		}
		if _, err := uiautoSvc.WaitUntilExists(
			ctx, &ui.WaitUntilExistsRequest{Finder: helpLinkFinder}); err != nil {
			s.Fatal("Failed to find feedback default help link: ", err)
		}
	}
}
