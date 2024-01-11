// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/gaiaenrollment"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const gaiaBytebotEnrollmentTimeout = 7 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         GAIABytebotEnrollment,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a device to a Bytebot domain and check PluginVmUserId policy",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
		},
		BugComponent: "b:1111632",
		Attr:         []string{"group:dmserver-enrollment-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.tape.Service", "tast.cros.graphics.ScreenshotService"},
		Timeout:      gaiaBytebotEnrollmentTimeout,
		Params: []testing.Param{
			{
				Name: "autopush",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerAlphaURL,
					PoolID:   tape.ChromeosbytebotCom,
				},
			},
			{
				Name: "staging",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerStagingURL,
					PoolID:   tape.ChromeosbytebotCom,
				},
			},
			{
				Name:      "live",
				ExtraAttr: []string{"group:dmserver-enrollment-live"},
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerProdURL,
					PoolID:   tape.ChromeosbytebotCom,
				},
			},
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PluginVmUserId{}, pci.VerifiedValue),
		},
	})
}

func GAIABytebotEnrollment(ctx context.Context, s *testing.State) {
	param := s.Param().(gaiaenrollment.TestParams)
	dmServerURL := param.DMServer
	poolID := param.PoolID

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(ctx)

	// Shorten deadline to leave time separately for resetting the TPM and for logging and cleanup.
	cleanupCtx, cleanupCancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cleanupCancel()
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
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

		screenshotService.CaptureScreenshot(cleanupCtx, &graphics.CaptureScreenshotRequest{FilePrefix: "enrollmentError"})
	}
	defer captureScreenshotOnError(ctx, s.HasError)

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(gaiaBytebotEnrollmentTimeout.Seconds())
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

	if _, err := policyClient.GAIAEnrollAndLoginUsingChrome(ctx, &pspb.GAIAEnrollAndLoginUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: dmServerURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(cleanupCtx, &empty.Empty{})

	pJSON, err := policy.MarshalList([]policy.Policy{
		&policy.PluginVmUserId{Stat: policy.StatusSet, Val: "********"},
	})
	if err != nil {
		s.Fatal("Error while marshalling policies to JSON: ", err)
	}

	if _, err := policyClient.VerifyPolicyStatus(ctx, &pspb.VerifyPolicyStatusRequest{
		Policies: pJSON,
	}); err != nil {
		s.Fatal("Failed to verify policy: ", err)
	}
}
