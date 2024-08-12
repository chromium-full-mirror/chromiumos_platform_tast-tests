// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const freTestTimeout = 30 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         ForcedReEnrollment,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test forced re-enrollment after powerwash",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"sergiyb@google.com",
		},
		BugComponent: "b:1111632",
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.tape.Service",
			"tast.cros.graphics.ScreenshotService",
		},
		Attr:         []string{"group:powerwash-daily"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      freTestTimeout,
		Vars: []string{
			"ui.signinProfileTestExtensionManifestKey",
			tape.ServiceAccountVar,
		},
		Fixture: fixture.CleanOwnership,
		Params: []testing.Param{
			{
				Name: "automated_prod",
				Val: freTestParams{
					DMServer:    policy.DMServerProdURL,
					PoolID:      tape.FREAutomated,
					OrgUnitPath: "AutoRE",
				},
				ExtraSoftwareDeps: []string{"non_flex_device"},
			},
			{
				Name: "automated_alpha",
				Val: freTestParams{
					DMServer:    policy.DMServerAlphaURL,
					PoolID:      tape.FREAutomated,
					OrgUnitPath: "AutoRE",
				},
				ExtraSoftwareDeps: []string{"non_flex_device"},
			},
			// TODO(b/359219375): Add manual FRE variants.
		},
	})
}

type freTestParams struct {
	DMServer    string // device management server url
	PoolID      string // poolID for the used test account
	OrgUnitPath string // path to the OU into which device must be moved
}

func ForcedReEnrollment(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time separately for logging and cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	// Capture screenshot if test unexpectedly fails.
	captureScreenshotOnError := func(ctx context.Context, filename string) {
		cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the device to take screenshot: ", err)
		}
		defer cl.Close(ctx)
		screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: filename})
	}
	defer func() {
		if !s.HasError() {
			return
		}
		captureScreenshotOnError(cleanupCtx, "test-failure")
	}()

	// Deprovision the device after the test.
	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}
	defer func(ctx context.Context) {
		cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the device before deprovisioning: ", err)
		}
		defer cl.Close(ctx)
		if err := tapeClient.DeprovisionHelper(ctx, cl, ""); err != nil {
			s.Error("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	// Create an account manager and lease a test account for the duration of the test.
	param := s.Param().(freTestParams)
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(
		ctx, tapeClient,
		/*lock=*/ false, tape.WithTimeout(int32(freTestTimeout.Seconds())),
		tape.WithPoolID(param.PoolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	// Manually enroll into target organization.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect the DUT before manual enrollment: ", err)
	}
	defer cl.Close(ctx)
	policyClient := ps.NewPolicyServiceClient(cl.Conn)
	if _, err := policyClient.GAIAEnrollUsingChrome(ctx, &ps.GAIAEnrollUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: param.DMServer,
	}); err != nil {
		s.Fatal("Failed to manually enroll device before testing FRE: ", err)
	}

	// Move the device to the target OU with FRE enabled.
	if err := tapeClient.MoveDeviceToOU(ctx, cl, param.OrgUnitPath); err != nil {
		s.Fatal("Failed to move device to the target OU: ", err)
	}

	// Powerwash.
	if err := policyutil.Powerwash(ctx, s.CloudStorage(), s.DUT(), s.PushedFilesToDUT(""), nil); err != nil {
		s.Fatal("Failed to powerwash: ", err)
	}

	// When device has not been enrolled into any domain shortly prior to the
	// enrollment above, it may have been missing in the PSM and FRE will likely
	// fail because it can take up to 20 minutes before it is added to PSM. This
	// is why we want to wait at least 2 minutes between attempts so that device
	// has a chance of being added to PSM and do not unnecessarily overload
	// DMServer with repeated state determination requests.
	attempt := 0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Give FRE attempt 5 minutes to succeed, then retry.
		oobeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		// Reconnect to the device because cleaning the TPM restarts Chrome.
		attempt++
		cl, err = rpc.Dial(oobeCtx, s.DUT(), s.RPCHint())
		if err != nil {
			return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
		}
		defer cl.Close(ctx)

		policyClient := ps.NewPolicyServiceClient(cl.Conn)
		if param.PoolID == tape.FREAutomated {
			// Wait until device automatically enrolls into same org.
			_, err = policyClient.AutoReEnrollUsingChrome(oobeCtx, &ps.AutoReEnrollUsingChromeRequest{
				DmserverURL: param.DMServer,
				ManifestKey: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
			})
		} else if param.PoolID == tape.FREManual {
			// TODO(b/359219375): Implement manual FRE verification logic.
			err = nil
		} else {
			s.Fatal("Unexpected PoolID: ", param.PoolID)
		}
		if err != nil {
			s.Logf("Failed to re-enroll on attempt %d with error: %s", attempt, err)
			captureScreenshotOnError(ctx, fmt.Sprintf("attempt-%d-failure", attempt))

			if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
				return errors.Wrap(err, "failed to reset the TPM after failed FRE attempt")
			}
			return err
		}
		return nil
	}, &testing.PollOptions{Interval: 2 * time.Minute}); err != nil {
		s.Fatal("Failed to re-enroll: ", err)
	}
}
