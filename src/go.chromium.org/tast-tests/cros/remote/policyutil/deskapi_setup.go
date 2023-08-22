// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"time"
)

// DmServerURL is the URL to the autopush DM server.
const dmServerURL = "https://crosman-alpha.sandbox.google.com/devicemanagement/data/api"

var deskAPIAllowlist = []string{"https://continuous-sincere-relation.glitch.me/*", "https://engage.ringcentral.com/*"}

// Interface for calling methods in tape.client.
type tapeClient interface {
	SetPolicy(context.Context, tape.PolicySchema, []string, interface{}, string) error
}

// DeskFixtData holds the data pass from fixture to test body.
type DeskFixtData struct {
	Username string
	Password string
}

// DeskAPISetup sets up the initial setting for desk API fixture.
func DeskAPISetup(ctx context.Context, s *testing.FixtState, isLacros bool) (string, string, *tape.OwnedTestAccountManager) {
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
	defer cl.Close(ctx)

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

	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(1800) /*timeout_in_seconds*/, tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	//e.accManager = accManager

	// Disable Asset ID screen on enrollment.
	if err := disableUpdatingDeviceAttribute(ctx, tapeClient, acc.RequestID); err != nil {
		s.Log("Failed to disable updating device attribute: ", err)
	}

	// Enable Desk API
	if err := enableDeskAPI(ctx, tapeClient, acc.RequestID); err != nil {
		s.Fatal("Failed to enable Desk API policy: ", err)
	}
	if isLacros {
		if err := enableLacros(ctx, tapeClient, acc.RequestID); err != nil {
			s.Fatal("Failed to enable Lacros: ", err)
		}
	} else {
		if err := disableLacros(ctx, tapeClient, acc.RequestID); err != nil {
			s.Fatal("Failed to enable Ash: ", err)
		}
	}

	// Perform enroll without login here, as in local test will need to instantiate a chrome session anyway.
	if _, err := pc.GAIAEnrollUsingChrome(ctx, &pspb.GAIAEnrollUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: dmServerURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}

	return acc.Username, acc.Password, accManager
}

func enableLacros(ctx context.Context, client tapeClient, requestID string) error {
	policy := &tape.LacrosAvailabilityUsers{
		LacrosAvailability: tape.LACROSAVAILABILITYENUM_LACROS_AVAILABILITY_ENUM_LACROS_ONLY,
	}
	if err := client.SetPolicy(ctx, policy, []string{"lacrosAvailability"}, nil, requestID); err != nil {
		return errors.Wrap(err, "failed to set the lacros policy")
	}
	return nil
}

func disableLacros(ctx context.Context, client tapeClient, requestID string) error {
	policy := &tape.LacrosAvailabilityUsers{
		LacrosAvailability: tape.LACROSAVAILABILITYENUM_LACROS_AVAILABILITY_ENUM_LACROS_DISALLOWED,
	}
	if err := client.SetPolicy(ctx, policy, []string{"lacrosAvailability"}, nil, requestID); err != nil {
		return errors.Wrap(err, "failed to set the lacros policy")
	}
	return nil
}

func enableDeskAPI(ctx context.Context, client tapeClient, requestID string) error {
	policy := &tape.DeskApiUsers{
		DeskApiThirdPartyAccessEnabled: true,
		DeskApiThirdPartyAllowlist:     deskAPIAllowlist,
	}
	if err := client.SetPolicy(ctx, policy, []string{"deskApiThirdPartyAccessEnabled", "deskApiThirdPartyAllowlist"}, nil, requestID); err != nil {
		return errors.Wrap(err, "failed to set the Desk API policy")
	}
	return nil
}

func disableUpdatingDeviceAttribute(ctx context.Context, client tapeClient, requestID string) error {
	assetPolicy := &tape.AllowPopulateAssetIdentifierUsers{
		AllowToUpdateDeviceAttribute: false,
	}

	if err := client.SetPolicy(ctx, assetPolicy, []string{"allowToUpdateDeviceAttribute"}, nil, requestID); err != nil {
		return errors.Wrap(err, "failed to disable the AllowToUpdateDeviceAttribute policy")
	}
	return nil
}
