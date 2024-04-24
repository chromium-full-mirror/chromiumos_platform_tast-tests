// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/gaiaenrollment"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const gaiaEnrollmentSlowTimeout = 10 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         GAIAEnrollmentSlowConnection,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a device on a slow connection 1000kbps without checking policies",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mohamedaomar@google.com",
		},
		BugComponent: "b:1111632",
		Attr:         []string{"group:dpanel-end2end", "group:dmserver-enrollment-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.tape.Service",
			"tast.cros.graphics.ScreenshotService",
		},
		Fixture: fixture.CleanOwnership,
		Timeout: gaiaEnrollmentSlowTimeout,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Enroll devices for production testing (COM_ONB_CUJ6_TASK5_WF1).
			Value: "screenplay-89d51ff3-0264-477a-ac03-754fa6a8eb41",
		}, {
			Key: "feature_id",
			// Enroll devices manually (COM_ONB_CUJ7_TASK5_WF1).
			Value: "screenplay-2de3fdfc-26eb-4fe5-9959-91cbfed05acb",
		}},
		Params: []testing.Param{
			{
				Name: "prod",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerProdURL,
					PoolID:   tape.Enrollment,
				},
			},
			{
				Name: "autopush",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerAlphaURL,
					PoolID:   tape.Enrollment,
				},
			},
			{
				Name: "autopush_new_saml",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerAlphaURL,
					PoolID:   tape.Crosprqa4Com,
				},
			},
			{
				Name: "staging",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerStagingURL,
					PoolID:   tape.Enrollment,
				},
			},
			{
				Name: "staging_new_saml",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerStagingURL,
					PoolID:   tape.Crosprqa4Com,
				},
			},
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

func GAIAEnrollmentSlowConnection(ctx context.Context, s *testing.State) {
	param := s.Param().(gaiaenrollment.TestParams)
	dmServerURL := param.DMServer
	poolID := param.PoolID

	// Shorten deadline to leave time separately for resetting the TPM and for logging and cleanup.
	cleanupCtx, cleanupCancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cleanupCancel()
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

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

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(gaiaEnrollmentSlowTimeout.Seconds())
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
		if err := tapeClient.DeprovisionHelper(cleanupCtx, cl, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	// Throttle the network to 1000kbps.
	// dbus-send --system --print-reply --type=method_call --dest=org.chromium.flimflam / org.chromium.flimflam.Manager.SetNetworkThrottlingStatus boolean:true uint32:1000 uint32:1000
	conn := s.DUT().Conn()
	if err := conn.CommandContext(ctx, "dbus-send", "--system", "--print-reply", "--type=method_call", "--dest=org.chromium.flimflam", "/", "org.chromium.flimflam.Manager.SetNetworkThrottlingStatus", "boolean:true", "uint32:1000", "uint32:1000").Run(); err != nil {
		s.Fatal("Failed to run throttling command on the DUT: ", err)
	}

	defer func(ctx context.Context) {
		// Disable throttling the network.
		// dbus-send --system --print-reply --type=method_call --dest=org.chromium.flimflam / org.chromium.flimflam.Manager.SetNetworkThrottlingStatus boolean:false uint32:0 uint32:0
		if err = conn.CommandContext(ctx, "dbus-send", "--system", "--print-reply", "--type=method_call", "--dest=org.chromium.flimflam", "/", "org.chromium.flimflam.Manager.SetNetworkThrottlingStatus", "boolean:false", "uint32:0", "uint32:0").Run(); err != nil {
			s.Fatal("Failed to disable throttling command on the DUT: ", err)
		}
	}(cleanupCtx)

	if _, err := policyClient.GAIAEnrollUsingChrome(ctx, &pspb.GAIAEnrollUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: dmServerURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(cleanupCtx, &empty.Empty{})
}
