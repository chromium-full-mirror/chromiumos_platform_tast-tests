// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/devicetrust"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/enterpriseconnectors"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

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
	cr, err := chrome.New(
		ctx,
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: req.User, Pass: req.Pass}),
		chrome.DMSPolicy(policy.DMServerAlphaURL),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(req.SigninProfileTestExtensionManifestKey),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Chrome")
	}
	service.cr = cr
	return &empty.Empty{}, nil
}

// VerifyDeviceTrustPolicy verifies that the Device Trust device policy contains the expected IdP URL.
func (service *DeviceTrustService) VerifyDeviceTrustPolicy(ctx context.Context, req *pb.VerifyDeviceTrustPolicyRequest) (_ *empty.Empty, retErr error) {
	if service.cr == nil {
		return nil, errors.New("Device Trust service is not set up properly")
	}

	tconn, err := service.cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating login test API connection failed")
	}

	// Verify that the policy DeviceLoginScreenContextAwareAccessSignalsAllowlist was applied correctly by the device trust connector.
	testing.ContextLog(ctx, "Verifying policy DeviceLoginScreenContextAwareAccessSignalsAllowlist")
	po := &testing.PollOptions{Timeout: 3 * time.Minute, Interval: 1 * time.Second}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := policyutil.Refresh(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to refresh enterprise policies")
		}

		dps, err := policyutil.PoliciesFromDUT(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to fetch policies from DUT")
		}

		expected := &policy.DeviceLoginScreenContextAwareAccessSignalsAllowlist{Val: []string{req.ExpectedIdPURL}}
		actual, ok := dps.Chrome[expected.Name()]
		if !ok {
			return errors.New("policy DeviceLoginScreenContextAwareAccessSignalsAllowlist was not set on DUT")
		}

		// Compare policy value.
		actualUnmarshaled, err := expected.UnmarshalAs(actual.ValueJSON)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal the policy value")
		}

		if !expected.Equal(actualUnmarshaled) {
			return errors.Errorf("unexpected value for DeviceLoginScreenContextAwareAccessSignalsAllowlist = got %q, want %q", actualUnmarshaled, expected.Val)
		}

		return nil
	}, po); err != nil {
		return nil, errors.Wrap(err, "failed to verify policy DeviceLoginScreenContextAwareAccessSignalsAllowlist")
	}

	return &empty.Empty{}, nil
}

// InitiateSamlLogin uses fake user credentials to get a SAML redirection to a Fake IdP, where the Device Trust attestation flow can be tested.
func (service *DeviceTrustService) InitiateSamlLogin(ctx context.Context, req *pb.InitiateSamlLoginRequest) (_ *empty.Empty, retErr error) {
	var fakeCreds chrome.Creds
	fakeCreds.User = "tast-test-device-trust@managedchrome.com"

	cr, err := chrome.New(
		ctx,
		chrome.KeepEnrollment(),
		chrome.DMSPolicy(policy.DMServerAlphaURL),
		chrome.LoadSigninProfileExtension(req.SigninProfileTestExtensionManifestKey),
		chrome.SAMLLogin(fakeCreds),
		chrome.EnableFeatures(devicetrust.DeviceTrustFeature),
	)
	if err != nil {
		return nil, errors.Wrap(err, "Chrome login failed")
	}

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating login test API connection failed")
	}
	ui := uiauto.New(tconn).WithTimeout(devicetrust.DefaultUITimeout)

	if err := devicetrust.StartAttestationFlowWithFakeIdP(ctx, ui); err != nil {
		return nil, errors.Wrap(err, "Device Trust failed")
	}

	service.cr = cr
	service.ui = ui

	return &empty.Empty{}, nil
}

// GaiaLogin does a real GAIA login, fetches real policy information for the corresponding user and navigates to the URL of the fake IdP.
func (service *DeviceTrustService) GaiaLogin(ctx context.Context, req *pb.GaiaLoginRequest) (_ *empty.Empty, retErr error) {
	cr, err := chrome.New(
		ctx,
		chrome.KeepEnrollment(),
		chrome.DMSPolicy(policy.DMServerAlphaURL),
		chrome.GAIALogin(chrome.Creds{User: req.User, Pass: req.Pass}),
		chrome.EnableFeatures(devicetrust.DeviceTrustFeature),
		chrome.RemoveNotification(false),
	)
	if err != nil {
		return nil, errors.Wrap(err, "Chrome login failed")
	}
	service.cr = cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating test API connection failed")
	}
	ui := uiauto.New(tconn).WithTimeout(devicetrust.DefaultUITimeout)
	service.ui = ui

	_, err = service.cr.NewConn(ctx, devicetrust.FakeIdPURL)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to URL failed")
	}

	return &empty.Empty{}, nil
}

// StartAttestationFlow connects to a Fake IdP, where the Device Trust inline attestation flow can be tested.
func (service *DeviceTrustService) StartAttestationFlow(ctx context.Context, req *empty.Empty) (_ *empty.Empty, retErr error) {
	if service.cr == nil || service.ui == nil {
		return nil, errors.New("Device Trust service is not set up properly")
	}

	if err := devicetrust.StartAttestationFlowWithFakeIdP(ctx, service.ui); err != nil {
		return nil, errors.Wrap(err, "Device Trust failed")
	}
	return &empty.Empty{}, nil
}

// CheckFakeIdPStatus checks if the result of the Device Trust attestation flow is as expected based on the text on the fake IdP.
func (service *DeviceTrustService) CheckFakeIdPStatus(ctx context.Context, req *pb.CheckFakeIdPStatusRequest) (_ *empty.Empty, retErr error) {
	if service.cr == nil || service.ui == nil {
		return nil, errors.New("device Trust service is not set up properly")
	}

	if err := devicetrust.CheckFakeIdPStatus(ctx, service.cr, service.ui, req.IsInSession, req.Expected /*isDeviceManaged=*/, true); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// StopChrome closes the current Chrome instance.
func (service *DeviceTrustService) StopChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if service.cr == nil {
		return nil, errors.New("no active Chrome instance")
	}

	var err error
	if err = service.cr.Close(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close Chrome: ", err)
		err = errors.Wrap(err, "failed to close Chrome")
	}
	service.cr = nil

	return &empty.Empty{}, err
}
