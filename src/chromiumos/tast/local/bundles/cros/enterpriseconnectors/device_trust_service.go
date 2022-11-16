// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	pb "chromiumos/tast/services/cros/enterpriseconnectors"
	"chromiumos/tast/testing"
)

// defaultUITimeout is the default timeout for UI interactions.
const defaultUITimeout = 20 * time.Second
const sandboxDMServer = "https://crosman-alpha.sandbox.google.com/devicemanagement/data/api"
const deviceTrustFeature = "DeviceTrustConnectorEnabled"

// URL of a fake IdP, which is hosted and maintained by cbe-device-trust-eng@google.com
const fakeIdPURL = "https://cbe-integrationtesting-sandbox.uc.r.appspot.com"

// Expected error message for Device Trust attestation flows, where the host is not allowed.
const errorMessageHostNotAllowed = "Missing X-Device-Trust header in the first request"

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterDeviceTrustServiceServer(srv, &DeviceTrustService{})
		},
	})
}

// DeviceTrustService implements tast.cros.enterpriseconnectors.DeviceTrustService.
type DeviceTrustService struct {
	cr *chrome.Chrome
	ui *uiauto.Context
}

// Enroll the device with the provided account credentials.
func (service *DeviceTrustService) Enroll(ctx context.Context, req *pb.EnrollRequest) (_ *empty.Empty, retErr error) {
	if service.cr != nil {
		return nil, errors.New("DUT for running snapshot is already set up")
	}
	var opts []chrome.Option

	opts = append(opts, chrome.GAIAEnterpriseEnroll(chrome.Creds{User: req.User, Pass: req.Pass}))
	opts = append(opts, chrome.DMSPolicy(sandboxDMServer))
	opts = append(opts, chrome.NoLogin())
	_, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Chrome")
	}

	return &empty.Empty{}, nil
}

// LoginWithFakeIdP uses the fake user credentials to get a SAML redirection to a Fake IdP, where the Device Trust attestation flow is tested.
func (service *DeviceTrustService) LoginWithFakeIdP(ctx context.Context, req *pb.LoginWithFakeIdPRequest) (_ *empty.Empty, retErr error) {
	var fakeCreds chrome.Creds
	fakeCreds.User = "tast-test-device-trust@managedchrome.com"

	cr, err := chrome.New(
		ctx,
		chrome.KeepEnrollment(),
		chrome.DMSPolicy(sandboxDMServer),
		chrome.LoadSigninProfileExtension(req.SigninProfileTestExtensionManifestKey),
		chrome.SAMLLogin(fakeCreds),
		chrome.EnableFeatures(deviceTrustFeature),
	)
	if err != nil {
		return nil, errors.Wrap(err, "Chrome login failed")
	}

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating login test API connection failed")
	}
	ui := uiauto.New(tconn).WithTimeout(defaultUITimeout)

	if err := testFakeIdP(ctx, ui); err != nil {
		return nil, errors.Wrap(err, "Device Trust failed")
	}

	service.cr = cr
	service.ui = ui

	return &empty.Empty{}, nil
}

// ConnectToFakeIdP does a real GAIA login and connects to a Fake IdP inside a session, where the Device Trust inline attestation flow is tested.
func (service *DeviceTrustService) ConnectToFakeIdP(ctx context.Context, req *pb.ConnectToFakeIdPRequest) (_ *empty.Empty, retErr error) {
	cr, err := chrome.New(
		ctx,
		chrome.KeepEnrollment(),
		chrome.DMSPolicy(sandboxDMServer),
		chrome.GAIALogin(chrome.Creds{User: req.User, Pass: req.Pass}),
		chrome.EnableFeatures(deviceTrustFeature),
	)
	if err != nil {
		return nil, errors.Wrap(err, "Chrome login failed")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating test API connection failed")
	}
	ui := uiauto.New(tconn).WithTimeout(defaultUITimeout)

	conn, err := cr.NewConn(ctx, fakeIdPURL)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to URL failed")
	}
	defer conn.Close()

	if err := testFakeIdP(ctx, ui); err != nil {
		return nil, errors.Wrap(err, "Device Trust failed")
	}

	service.cr = cr
	service.ui = ui

	return &empty.Empty{}, nil
}

// CheckFakeIdPStatus checks if the result of the Device Trust attestation flow is as expected based on the text on the fake IdP.
func (service *DeviceTrustService) CheckFakeIdPStatus(ctx context.Context, req *pb.CheckFakeIdPStatusRequest) (_ *empty.Empty, retErr error) {
	if service.cr == nil || service.ui == nil {
		return nil, errors.New("Device Trust service is not set up properly")
	}
	defer service.cr.Close(ctx)

	deviceTrustSuccessful, err := wasDeviceTrustAttestationSuccessful(ctx, service.ui)
	if err != nil {
		return nil, errors.Wrap(err, " failed to check if Device Trust succeeded")
	}

	conn, err := service.cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix(fakeIdPURL+"/idp/login"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to open existing connection")
	}

	if deviceTrustSuccessful {
		if req.Expected != deviceTrustSuccessful {
			return nil, errors.New("Device Trust succeeded unexpectedly")
		}

		if err := checkSignals(ctx, conn); err != nil {
			return nil, errors.Wrap(err, "checking signals failed")
		}
	} else {
		errorMessage, err := getErrorMessage(ctx, conn)
		if err != nil {
			return nil, errors.Wrap(err, "checking error message failed")
		}

		if req.Expected != deviceTrustSuccessful {
			return nil, errors.New("Device trust failed with error: " + errorMessage)
		}

		if errorMessage != errorMessageHostNotAllowed {
			return nil, errors.Errorf("unexpected value for errorMessage: got %q, want %q", errorMessage, errorMessageHostNotAllowed)
		}
	}

	return &empty.Empty{}, nil
}

// getErrorMessage returns in case of an unsuccessful Device Trust attestation flow the error message, which should be displayed by the fake IdP.
func getErrorMessage(ctx context.Context, conn *chrome.Conn) (string, error) {
	var errorMessage string
	if err := conn.Call(ctx, &errorMessage, "() => { return document.getElementById('errorMessage').innerText; }"); err != nil {
		return "", err
	}

	errorMessage = strings.ReplaceAll(errorMessage, "\n", "")

	return errorMessage, nil
}

// checkSignals checks in case of a successful Device Trust attestation flow, if transmitted client and server signals are non-empty.
func checkSignals(ctx context.Context, conn *chrome.Conn) error {
	var serverSignals string
	if err := conn.Call(ctx, &serverSignals, "() => { return document.getElementById('serverSignals').innerText; }"); err != nil {
		return errors.Wrap(err, "failed reading server signals")
	}
	if serverSignals == "" {
		return errors.New("Server signals were empty")
	}

	var clientSignals string
	if err := conn.Call(ctx, &clientSignals, "() => { return document.getElementById('clientSignals').innerText; }"); err != nil {
		return errors.Wrap(err, "failed reading client signals")
	}
	if clientSignals == "" {
		return errors.New("Client signals were empty")
	}

	return nil
}

// wasDeviceTrustAttestationSuccessful analyzes the current content on the fake IdP site to decide whether the Device Trust attestation flow was successful or not.
func wasDeviceTrustAttestationSuccessful(ctx context.Context, ui *uiauto.Context) (bool, error) {
	root := nodewith.Name("Sample Login page").Role(role.RootWebArea)
	signalText := nodewith.Name("Server Signals:").Role(role.StaticText).Ancestor(root)
	errorMessage := nodewith.Name("Device Trust failed with error:").Role(role.StaticText).Ancestor(root)

	result := false
	err := testing.Poll(ctx, func(ctx context.Context) error {
		err := ui.Exists(signalText)(ctx)
		if err == nil {
			result = true
			return nil
		}
		err = ui.Exists(errorMessage)(ctx)
		if err == nil {
			result = false
			return nil
		}
		return errors.Wrap(err, " found neither the signal list nor the error message")
	}, &testing.PollOptions{Interval: 300 * time.Millisecond,
		Timeout: defaultUITimeout})

	if err != nil {
		return false, err
	}

	return result, nil
}

func testFakeIdP(ctx context.Context, ui *uiauto.Context) error {
	root := nodewith.Name("Device Trust IdP").Role(role.RootWebArea)

	startButton := nodewith.Name("Start Device Trust Attestation(using VAv2)").Role(role.Link).Ancestor(root).Focusable()
	if err := uiauto.Combine("Click on start button and proceed",
		ui.WaitUntilExists(startButton),
		ui.LeftClick(startButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to start the Device Trust attestation. Fake IdP not loaded correctly")
	}

	return nil
}
