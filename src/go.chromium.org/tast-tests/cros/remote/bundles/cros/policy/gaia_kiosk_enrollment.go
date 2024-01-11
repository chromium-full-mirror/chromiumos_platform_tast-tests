// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/gaiaenrollment"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	kspb "go.chromium.org/tast-tests/cros/services/cros/kiosk"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	// gaiaKioskEnrollmentTimeout is the timeout for enrollment.
	gaiaKioskEnrollmentTimeout = 5 * time.Minute
	// gaiaKioskEnrollmentLaunchTimeout is the timeout for launching Kiosk. The value should be based
	// on `kiosk_mode.LaunchDuration`.
	gaiaKioskEnrollmentLaunchTimeout  = 5 * time.Minute
	gaiaKioskEnrollmentCleanupTimeout = 3 * time.Minute
	gaiaKioskEnrollmentTestTimeout    = gaiaKioskEnrollmentLaunchTimeout + gaiaKioskEnrollmentTimeout + gaiaKioskEnrollmentCleanupTimeout
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GAIAKioskEnrollment,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a kiosk device and make sure kiosk app started",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"edmanp@google.com",
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr:         []string{"group:dpanel-end2end", "group:dmserver-enrollment-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.kiosk.KioskService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service", "tast.cros.graphics.ScreenshotService"},
		Timeout:      gaiaKioskEnrollmentTestTimeout,
		Params: []testing.Param{
			{
				Name: "autopush",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerAlphaURL,
					PoolID:   tape.EnrollmentKiosk,
				},
			},
			{
				Name: "staging",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerStagingURL,
					PoolID:   tape.EnrollmentKiosk,
				},
			},
		},
		Vars: []string{tape.ServiceAccountVar},
	})
}

func GAIAKioskEnrollment(ctx context.Context, s *testing.State) {
	param := s.Param().(gaiaenrollment.TestParams)
	dmServerURL := param.DMServer
	poolID := param.PoolID

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(ctx)

	// Shorten deadline to leave time separately for resetting the TPM and for logging and cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, gaiaKioskEnrollmentCleanupTimeout)
	defer cancel()

	if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	captureScreenshotOnError := func(ctx context.Context, hasError func() bool) {
		if !hasError() {
			return
		}

		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "enrollmentError"})
	}
	defer captureScreenshotOnError(cleanupCtx, s.HasError)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(gaiaKioskEnrollmentTestTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, false /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	// Deprovision the DUT at the end of the test. As devices might get
	// provisioned even when the enrollment fails we need to defer the
	// deprovisioning before enrolling.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(cleanupCtx, cl, acc.CustomerID, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	// Init a syslog reader in KioskService before signing in.
	kc := kspb.NewKioskServiceClient(cl.Conn)
	if _, err := kc.InitSyslogReader(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to initialize syslog reader for Kiosk: ", err)
	}

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	if _, err := policyClient.GAIAEnrollUsingChrome(ctx, &pspb.GAIAEnrollUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: dmServerURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(cleanupCtx, &empty.Empty{})

	launchCtx, cancelLaunchCtx := context.WithTimeout(ctx, gaiaKioskEnrollmentLaunchTimeout)
	defer cancelLaunchCtx()
	if _, err := kc.ConfirmKioskStartedWithReader(launchCtx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to confirm Kiosk started: ", err)
	}
}
