// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/services/cros/enterpriseconnectors"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const deviceTrustInsessionEnrollmentTimeout = 7 * time.Minute

type userParamInsession struct {
	poolID         string
	expectedIDPURL string
	loginPossible  bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceTrustInsession,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Device Trust is working insession with a fake IdP",
		Contacts: []string{
			"cbe-device-trust-eng@google.com",
			"cros-3pidp@google.com",
			"lmasopust@google.com",
			"rodmartin@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"reboot",
		},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.tape.Service",
			"tast.cros.enterpriseconnectors.DeviceTrustService",
			"tast.cros.graphics.ScreenshotService",
		},
		Attr: []string{
			"group:golden_tier",
			"group:hardware",
		},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			tape.ServiceAccountVar,
		},
		Params: []testing.Param{{
			Name: "host_allowed",
			Val: userParamInsession{
				poolID:         tape.DeviceTrustEnabled,
				expectedIDPURL: "https://staging-idp-dot-cbe-integrationtesting-sandbox.uc.r.appspot.com",
				loginPossible:  true,
			},
		}, {
			Name: "host_not_allowed",
			Val: userParamInsession{
				poolID:         tape.DeviceTrustDisabled,
				expectedIDPURL: "https://www.example.com",
				loginPossible:  false,
			},
		}},
		Fixture:      fixture.CleanOwnership,
		Timeout:      4 * time.Minute,
		BugComponent: "b:1163683",
	})
}

func DeviceTrustInsession(ctx context.Context, s *testing.State) {
	param := s.Param().(userParamInsession)
	poolID := param.poolID
	expectedIDPURL := param.expectedIDPURL
	signinProfileTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(deviceTrustInsessionEnrollmentTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, false /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	captureScreenshotOnError := func(ctx context.Context, hasError func() bool) {
		if !hasError() {
			return
		}

		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "deviceTrustLoginError"})
	}
	defer captureScreenshotOnError(cleanupCtx, s.HasError)

	service := enterpriseconnectors.NewDeviceTrustServiceClient(cl.Conn)

	// Deprovision the DUT at the end of the test. As devices might get
	// provisioned even when the enrollment fails we need to defer the
	// deprovisioning before enrolling.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(ctx, cl, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	s.Log("Enrolling device")
	if _, err = service.Enroll(ctx, &enterpriseconnectors.EnrollRequest{User: acc.Username, Pass: acc.Password, ExpectedIdPURL: expectedIDPURL, SigninProfileTestExtensionManifestKey: signinProfileTestExtensionManifestKey}); err != nil {
		s.Fatal("Remote call Enroll() failed: ", err)
	}
	defer service.StopChrome(cleanupCtx, &empty.Empty{})

	if _, err := service.ConnectToFakeIdP(ctx, &enterpriseconnectors.ConnectToFakeIdPRequest{User: acc.Username, Pass: acc.Password}); err != nil {
		s.Fatal("Remote call ConnectToFakeIdP() failed: ", err)
	}

	if _, err = service.CheckFakeIdPStatus(ctx, &enterpriseconnectors.CheckFakeIdPStatusRequest{Expected: param.loginPossible, IsInSession: true}); err != nil {
		s.Fatal("Remote call CheckFakeIdPStatus() failed: ", err)
	}
}
