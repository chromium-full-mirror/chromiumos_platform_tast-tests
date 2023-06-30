// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/oobe"
	oobeHelper "go.chromium.org/tast-tests/cros/local/oobe"
	pb "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterOobeHidBluetoothServiceServer(srv, &OobeHidBluetoothService{})
		},
	})
}

// OobeHidBluetoothService implements tast.cros.bluetooth.OobeHidBluetoothService.
type OobeHidBluetoothService struct {
	chrome *chrome.Chrome
}

func (svc *OobeHidBluetoothService) NewChrome(ctx context.Context, req *pb.NewChromeRequest) (*empty.Empty, error) {
	if svc.chrome != nil {
		return nil, errors.New("Chrome has already been started")
	}

	var chromeOpts []chrome.Option = []chrome.Option{
		chrome.NoLogin(),
		chrome.EnableHIDScreenOnOOBE(),
		chrome.LoadSigninProfileExtension(req.SigninProfileTestExtension),
		chrome.EnableFeatures(
			"OobeHidDetectionRevamp", "QsRevamp",
		),
	}
	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}
	svc.chrome = cr

	oobeConn, err := svc.chrome.WaitForOOBEConnection(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OOBE connection")
	}
	defer oobeConn.Close()

	tconn, err := svc.chrome.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the signin profile test API connection")
	}

	contextOutDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get the context output directory")
	}

	defer faillog.DumpUITree(ctx, contextOutDir, tconn)

	if err := oobe.IsHidDetectionScreenVisible(ctx, oobeConn); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the HID detection screen to be visible")
	}

	if err := oobe.IsHidDetectionSearchingForMouse(ctx, oobeConn, tconn); err != nil {
		return nil, errors.Wrap(err, "expected mouse device to not be connected")
	}

	if err := oobe.IsHidDetectionSearchingForKeyboard(ctx, oobeConn, tconn); err != nil {
		return nil, errors.Wrap(err, "expected keyboard device to not be connected")
	}

	return &empty.Empty{}, nil
}

func (svc *OobeHidBluetoothService) CloseChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if svc.chrome == nil {
		return nil, errors.New("Chrome must be started before it can be closed")
	}
	err := svc.chrome.Close(ctx)
	svc.chrome = nil
	return &empty.Empty{}, err
}

func (svc *OobeHidBluetoothService) ProgressToWelcomeScreen(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if svc.chrome == nil {
		return nil, errors.New("Chrome must be started before progressing to the welcome screen")
	}

	oobeConn, err := svc.chrome.WaitForOOBEConnection(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OOBE connection")
	}
	defer oobeConn.Close()

	tconn, err := svc.chrome.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the signin profile test API connection")
	}

	// Add a virtual USB keyboard to enable continue button.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a virtual keyboard")
	}

	defer keyboard.Close(ctx)

	// Check that a keyboard is detected.
	if err := oobeHelper.IsHidDetectionSearchingForKeyboard(ctx, oobeConn, tconn); err == nil {
		return nil, errors.Wrap(err, "expected keyboard device to be found")
	}

	if err := oobe.IsHidDetectionContinueButtonEnabled(ctx, oobeConn); err != nil {
		return nil, errors.Wrap(err, "expected continue button to be enabled")
	}

	if err := oobe.ClickHidScreenNextButton(ctx, oobeConn, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to click on hid next button")
	}

	if err := oobe.IsWelcomeScreenVisible(ctx, oobeConn); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the welcome screen to be visible")
	}
	return &empty.Empty{}, nil
}

func (svc *OobeHidBluetoothService) DisableBluetoothFromQuickSettings(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if svc.chrome == nil {
		return nil, errors.New("Chrome must be started before disabling bluetooth from quick settings")
	}

	tconn, err := svc.chrome.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the signin profile test API connection")
	}

	cleanup, err := quicksettings.Init(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init quicksettings")
	}
	defer cleanup()

	if err := quicksettings.Show(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to show quick settings")
	}
	defer quicksettings.Hide(ctx, tconn)

	ui := uiauto.New(tconn)
	if err := ui.LeftClick(quicksettings.FeatureTileBluetoothToggle)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to click the Bluetooth feature tile toggle button")
	}

	// TODO(b/242071154): Add floss implementation.
	if err := bluez.PollForBTDisabled(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for bluetooth adapter to be disabled")
	}

	return &empty.Empty{}, nil
}

func (svc *OobeHidBluetoothService) VerifyBluetoothIsEnabled(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if svc.chrome == nil {
		return nil, errors.New("Chrome must be started before verifying Bluetooth is enabled")
	}

	oobeConn, err := svc.chrome.WaitForOOBEConnection(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OOBE connection")
	}
	defer oobeConn.Close()

	// TODO(b/242071154): Add floss implementation.
	if err := bluez.PollForBTEnabled(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for bluetooth adapter to be enabled")
	}

	return &empty.Empty{}, nil
}
