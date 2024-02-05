// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/devicetrust"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const deviceTrustByodTimeout = chrome.GAIALoginTimeout + time.Minute
const deviceTrustFeatureByod = "UnmanagedDeviceDeviceTrustConnectorEnabled"

type deviceTrustParams struct {
	poolID         string
	expectedStatus bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceTrustInsessionByod,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Device Trust is working in-session with a fake IdP on an unmanaged device",
		Contacts: []string{
			"cbe-device-trust-eng@google.com",
			"cros-3pidp@google.com",
			"lmasopust@google.com",
			"rodmartin@google.com",
		},
		BugComponent: "b:1163683",
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Vars: []string{"tape.service_account_key"},
		Attr: []string{"group:golden_tier"},
		Params: []testing.Param{{
			Name: "host_allowed",
			Val: deviceTrustParams{
				poolID:         tape.DeviceTrustEnabled,
				expectedStatus: true,
			},
		}, {
			Name: "host_not_allowed",
			Val: deviceTrustParams{
				poolID:         tape.DeviceTrustDisabled,
				expectedStatus: false,
			},
		}},
		Timeout: deviceTrustByodTimeout,
	})
}

func DeviceTrustInsessionByod(ctx context.Context, s *testing.State) {
	param := s.Param().(deviceTrustParams)
	poolID := param.poolID
	expectedStatus := param.expectedStatus

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Create an account manager and lease a test account for the duration of the test.
	timeout := int32(deviceTrustByodTimeout.Seconds())
	accountManager, tapeAccount, err := tape.NewOwnedTestAccountManager(
		ctx,
		[]byte(s.RequiredVar(tape.ServiceAccountVar)),
		false, /*lock*/
		tape.WithTimeout(timeout),
		tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to request owned test account from TAPE: ", err)
	}
	defer accountManager.CleanUp(cleanupCtx)

	// Create a new user session with the leased account.
	cr, err := chrome.New(
		ctx,
		chrome.DMSPolicy(policy.DMServerAlphaURL),
		chrome.GAIALogin(chrome.Creds{User: tapeAccount.Username, Pass: tapeAccount.Password}),
		chrome.EnableFeatures(devicetrust.DeviceTrustFeature),
		chrome.EnableFeatures(deviceTrustFeatureByod),
		chrome.ExtraArgs("--disable-search-engine-choice-screen"),
	)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	// Navigate to the Fake IdP URL.
	conn, err := cr.NewConn(ctx, devicetrust.FakeIdPURL)
	if err != nil {
		s.Fatal("Failed connecting to fake IdP URL: ", err)
	}
	defer conn.Close()

	if err := devicetrust.StartAttestationFlowWithFakeIdP(ctx, ui); err != nil {
		s.Fatal("Failed starting the Device Trust flow: ", err)
	}

	if err := devicetrust.CheckFakeIdPStatus(ctx, cr, ui, true /*isInSession=*/, expectedStatus, false /*isDeviceManaged=*/); err != nil {
		s.Fatal("Failed checking the result of the Device Trust flow: ", err)
	}
}
