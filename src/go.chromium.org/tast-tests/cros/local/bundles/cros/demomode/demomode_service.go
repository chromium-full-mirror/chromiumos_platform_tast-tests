// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/session"
	pb "go.chromium.org/tast-tests/cros/services/cros/demomode"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			demoModeService := Service{state: s, sharedObject: common.SharedObjectsForServiceSingleton}
			pb.RegisterDemoModeServiceServer(srv, &demoModeService)
		},
	})
}

// Service implements tast.cros.demomode.DemoModeService
type Service struct {
	state        *testing.ServiceState
	sharedObject *common.SharedObjectsForService
}

// SetUpDemoMode clicks through the Demo Mode OOBE setup UI to place the device in
// Demo Mode for further testing to run
func (service *Service) SetUpDemoMode(ctx context.Context, req *pb.SetUpDemoModeRequest) (*empty.Empty, error) {
	service.sharedObject.ChromeMutex.Lock()
	defer service.sharedObject.ChromeMutex.Unlock()
	cr := service.sharedObject.Chrome

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OOBE connection")
	}
	defer oobeConn.Close()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the signin profile test API connection")
	}

	ui := uiauto.New(tconn).WithTimeout(50 * time.Second)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get keyboard handle")
	}
	defer kb.Close(ctx)

	if err := enterDemoModeSetupFlow(ctx, oobeConn, ui, kb); err != nil {
		return nil, errors.Wrap(err, "failed to enter Demo Mode setup flow")
	}

	if err := proceedThroughNetworkScreen(ctx, oobeConn, ui); err != nil {
		return nil, errors.Wrap(err, "failed to proceed through network screen")
	}

	if err := proceedThroughPreferencesScreen(ctx, oobeConn, ui, kb); err != nil {
		return nil, errors.Wrap(err, "failed to proceed through preferences screen")
	}

	if err := proceedThroughTOSAndSetupScreens(ctx, oobeConn, ui, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to proceed through setup or TOS screens")
	}

	return &empty.Empty{}, nil
}

func findAndClickButton(ctx context.Context, buttonAPIMethod string, oobeConn *chrome.Conn, ui *uiauto.Context) error {
	var buttonName string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens."+buttonAPIMethod, &buttonName); err != nil {
		return errors.Wrapf(err, "failed to get button name by calling %s", buttonAPIMethod)
	}

	button := nodewith.Role(role.Button).Name(buttonName)
	if err := uiauto.Combine("Click button with name: "+buttonName,
		ui.WaitUntilExists(button),
		ui.LeftClick(button),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to click button with name: %s", buttonName)
	}
	return nil
}

func enterDemoModeSetupFlow(ctx context.Context, oobeConn *chrome.Conn, ui *uiauto.Context, kb *input.KeyboardEventWriter) error {
	if err := kb.Accel(ctx, "Ctrl+Alt+D"); err != nil {
		return errors.Wrap(err, "failed to enter Demo Setup dialogue with ctrl + alt + D")
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.demoModeConfirmationDialog.isVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the demo confirmation dialog to be visible")
	}
	findAndClickButton(ctx, "WelcomeScreen.getDemoModeOkButtonName()", oobeConn, ui)
	return nil
}

func proceedThroughNetworkScreen(ctx context.Context, oobeConn *chrome.Conn, ui *uiauto.Context) error {
	shouldSkipNetworkScreen := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.NetworkScreen.shouldSkip()", &shouldSkipNetworkScreen); err != nil {
		return errors.Wrap(err, "failed to evaluate whether to skip network screen")
	}
	if shouldSkipNetworkScreen {
		testing.ContextLog(ctx, "NetworkScreen.shouldSkip() is true; skipped")
	} else {
		testing.ContextLog(ctx, "Proceeding through network screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.NetworkScreen.isVisible()"); err != nil {
			return errors.Wrap(err, "failed to wait for the network screen to be visible")
		}
		// TODO(crbug.com/1291153): Switch to focused button.
		nextButton := nodewith.Name("Next").Role(role.Button)
		if err := ui.LeftClickUntil(nextButton, ui.Gone(nextButton))(ctx); err != nil {
			return errors.Wrap(err, "failed to click network page next button")
		}
	}
	return nil
}

func proceedThroughPreferencesScreen(ctx context.Context, oobeConn *chrome.Conn, ui *uiauto.Context, kb *input.KeyboardEventWriter) error {
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.DemoPreferencesScreen.isVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the demo preferences screen to be visible")
	}
	retailerNameInput := nodewith.Role(role.TextField).Name("Retailer Name")
	storeNumberInput := nodewith.Role(role.TextField).Name("Store Number")
	if err := uiauto.Combine("Enter Retailer Name And Store Number",
		ui.LeftClickUntil(retailerNameInput, ui.WaitUntilExists(retailerNameInput.Editable().Focused())),
		kb.TypeAction("Tast Retailer"),
		kb.AccelAction("tab"),
		ui.WaitUntilExists(storeNumberInput.Editable().Focused()),
		kb.TypeAction("1234"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to enter Retailer Name or Store Number")
	}
	findAndClickButton(ctx, "DemoPreferencesScreen.getDemoPreferencesNextButtonName()", oobeConn, ui)
	return nil
}

func proceedThroughTOSAndSetupScreens(ctx context.Context, oobeConn *chrome.Conn, ui *uiauto.Context, tconn *chrome.TestConn) error {
	// Connect to session manager now for post-setup session login, before clicking
	// final accept button and entering non-interactive part of demo setup
	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to session manager")
	}
	sw, err := sm.WatchSessionStateChanged(ctx, "started")
	if err != nil {
		return errors.Wrap(err, "failed to watch for session manager D-Bus signals")
	}
	defer sw.Close(ctx)
	// Click Consolidated Consent accept to finish interactive part of setup
	testing.ContextLog(ctx, "Proceeding through consolidated consent screen")
	if err := oobe.AdvanceThroughConsolidatedConsentIfShown(ctx, oobeConn, tconn); err != nil {
		return errors.Wrap(err, "failed to advance through consolidated consent screen")
	}

	testing.ContextLog(ctx, "Waiting for SessionStateChanged \"started\" D-Bus signal from session_manager")
	select {
	case <-sw.Signals:
		testing.ContextLog(ctx, "Got SessionStateChanged signal. Demo Session has started")
	case <-ctx.Done():
		return errors.Wrap(err, "didn't get SessionStateChanged signal")
	}
	return nil
}
