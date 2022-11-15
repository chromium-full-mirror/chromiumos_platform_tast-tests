// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
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
const fakeIdPURL = "https://cbe-integrationtesting-sandbox.uc.r.appspot.com/"

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
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Chrome")
	}

	service.cr = cr
	return &empty.Empty{}, nil
}

// LoginWithFakeIdP uses the fake user credentials to get a SAML redirection to a Fake IdP, where the Device Trust attestation flow is tested.
func (service *DeviceTrustService) LoginWithFakeIdP(ctx context.Context, req *pb.LoginWithFakeIdPRequest) (res *pb.FakeIdPResponse, retErr error) {
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
	defer cr.Close(ctx)

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating login test API connection failed")
	}

	loginPossible, err := testFakeIdP(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "Device Trust failed")
	}

	return &pb.FakeIdPResponse{Succesful: loginPossible}, nil
}

// ConnectToFakeIdP does a real GAIA login and connects to a Fake IdP inside a session, where the Device Trust inline attestation flow is tested.
func (service *DeviceTrustService) ConnectToFakeIdP(ctx context.Context, req *pb.ConnectToFakeIdPRequest) (res *pb.FakeIdPResponse, retErr error) {
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
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating test API connection failed")
	}

	conn, err := cr.NewConn(ctx, fakeIdPURL)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to URL failed")
	}
	defer conn.Close()

	loginPossible, err := testFakeIdP(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "Device Trust failed")
	}

	return &pb.FakeIdPResponse{Succesful: loginPossible}, nil
}

func loginPossible(ctx context.Context, ui *uiauto.Context) (bool, error) {
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

func testFakeIdP(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	ui := uiauto.New(tconn).WithTimeout(defaultUITimeout)
	root := nodewith.Name("Device Trust IdP").Role(role.RootWebArea)

	startButton := nodewith.Name("Start Device Trust Attestation(using VAv2)").Role(role.Link).Ancestor(root).Focusable()
	if err := uiauto.Combine("Click on start button and proceed",
		ui.WaitUntilExists(startButton),
		ui.LeftClick(startButton),
	)(ctx); err != nil {
		return false, errors.Wrap(err, "failed to start the Device Trust attestation. Fake IdP not loaded correctly")
	}

	loginPossible, err := loginPossible(ctx, ui)
	if err != nil {
		return false, errors.Wrap(err, " failed to load results")
	}

	return loginPossible, nil
}
