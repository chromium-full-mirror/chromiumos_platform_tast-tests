// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

var deskAPIAllowlist = []string{"https://continuous-sincere-relation.glitch.me/*", "https://engage.ringcentral.com/*"}

// DeskFixtData holds the data pass from fixture to test body.
type DeskFixtData struct {
	Username string
	Password string
}

// DeskFixt defines shared logic for desk API fixture.
type deskFixt struct {
	isLacros bool
	acc      *tape.OwnedTestAccount
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.DeskAPILacros,
		Desc:            "Fixture providing Desk API feature access for lacros browser",
		Contacts:        []string{"chromeos-commercial-remote-management@google.com", "cros-commercial-productivity-eng@google.com", "aprilzhou@google.com"},
		BugComponent:    "b:1020793", // ChromeOS Server Projects > Enterprise Management > Commercial Productivity
		Impl:            NewDeskAPILacrosFixt(),
		SetUpTimeout:    15 * time.Minute,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.graphics.ScreenshotService",
			"tast.cros.tape.Service",
			"tast.cros.browser.ChromeService",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.DeskAPIAsh,
		Desc:            "Fixture providing Desk API feature access for ash browser",
		Contacts:        []string{"chromeos-commercial-remote-management@google.com", "cros-commercial-productivity-eng@google.com", "aprilzhou@google.com"},
		BugComponent:    "b:1020793", // ChromeOS Server Projects > Enterprise Management > Commercial Productivity
		Impl:            NewDeskAPIAshFixt(),
		SetUpTimeout:    15 * time.Minute,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.graphics.ScreenshotService",
			"tast.cros.tape.Service",
			"tast.cros.browser.ChromeService",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

// NewDeskAPIAshFixt returns a fixture to run desk API test in ash.
func NewDeskAPIAshFixt() testing.FixtureImpl {
	return &deskFixt{isLacros: false}
}

// NewDeskAPILacrosFixt returns a fixture to run desk API test in lacros.
func NewDeskAPILacrosFixt() testing.FixtureImpl {
	return &deskFixt{isLacros: true}
}

func (e *deskFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Make sure the DUT is connected at the beginning.
	// TODO(b/239013478): Clean up the connection checks when the issue is resolved.
	if err := s.DUT().Health(ctx); err != nil {
		s.Log("Failed DUT connection check at the beginning: ", err)

		// Try to reconnect to the DUT.
		waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if err := s.DUT().WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to the DUT at the beginning: ", err)
		}
	}

	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	defer func(ctx context.Context, hasError func() bool) {
		if !hasError() {
			return
		}
		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "deskApiError"})
	}(ctx, s.HasError)

	pc := pspb.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	// Create an account manager and lease a test account for the duration of the test.
	_, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(1800) /*timeout_in_seconds*/, tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	e.acc = acc

	// Disable Asset ID screen on enrollment.
	assetPolicy := &tape.AllowPopulateAssetIdentifierUsers{
		AllowToUpdateDeviceAttribute: false,
	}

	if err := tapeClient.SetPolicy(ctx, assetPolicy, []string{"allowToUpdateDeviceAttribute"}, nil, acc.RequestID); err != nil {
		s.Log("Failed to disable updating device attribute: ", err)
	}

	// Enable Desk API.
	policies := &tape.DeskApiUsers{
		DeskApiThirdPartyAccessEnabled: true,
		DeskApiThirdPartyAllowlist:     deskAPIAllowlist,
	}
	if err := tapeClient.SetPolicy(ctx, policies, []string{"deskApiThirdPartyAccessEnabled", "deskApiThirdPartyAllowlist"}, nil, acc.RequestID); err != nil {
		s.Fatal("Failed to set the Desk API policy: ", err)
	}

	// Set Lacros policy.
	if e.isLacros {
		policies := &tape.LacrosAvailabilityUsers{
			LacrosAvailability: tape.LACROSAVAILABILITYENUM_LACROS_AVAILABILITY_ENUM_LACROS_ONLY,
		}
		if err := tapeClient.SetPolicy(ctx, policies, []string{"lacrosAvailability"}, nil, acc.RequestID); err != nil {
			s.Fatal("Failed to enable lacros: ", err)
		}
	} else {
		policies := &tape.LacrosAvailabilityUsers{
			LacrosAvailability: tape.LACROSAVAILABILITYENUM_LACROS_AVAILABILITY_ENUM_LACROS_DISALLOWED,
		}
		if err := tapeClient.SetPolicy(ctx, policies, []string{"lacrosAvailability"}, nil, acc.RequestID); err != nil {
			s.Fatal("Failed to disable lacros: ", err)
		}
	}

	// Perform enroll without login here, as in local test will need to instantiate a chrome session anyway.
	if _, err := pc.GAIAEnrollUsingChrome(ctx, &pspb.GAIAEnrollUsingChromeRequest{
		Username:    e.acc.Username,
		Password:    e.acc.Password,
		DmserverURL: policy.DMServerAlphaURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}

	return DeskFixtData{Username: e.acc.Username, Password: e.acc.Password}
}

func (e *deskFixt) TearDown(ctx context.Context, s *testing.FixtState) {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC Service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}
	if err := tapeClient.DeprovisionHelper(ctx, cl, e.acc.OrgUnitPath); err != nil {
		s.Fatal("Failed to deprovision device: ", err)
	}
	tapeClient.ReleaseOwnedTestAccount(ctx, e.acc)

	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}
}

func (*deskFixt) Reset(ctx context.Context) error                        { return nil }
func (*deskFixt) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (*deskFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {}
