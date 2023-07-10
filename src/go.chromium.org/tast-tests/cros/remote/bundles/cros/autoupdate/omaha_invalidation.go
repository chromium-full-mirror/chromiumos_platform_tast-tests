// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/updateutil"
	"go.chromium.org/tast-tests/cros/services/cros/autoupdate"
	"go.chromium.org/tast-tests/cros/services/cros/nebraska"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	prepTimeout              = 2 * time.Minute
	verificationTimeout      = 2 * time.Minute
	rebootTimeout            = 1 * time.Minute
	cleanupTimeout           = 2 * time.Minute
	omahaInvalidationTimeout = prepTimeout + updateutil.UpdateTimeout + verificationTimeout + rebootTimeout + cleanupTimeout
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OmahaInvalidation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the update engine invalidates installed updates if Omaha issues an invalidation",
		Contacts: []string{
			"artyomchen@google.com", // Test author
			"chromeos-commercial-remote-management@google.com",
		},
		Fixture:      fixture.Autoupdate,
		BugComponent: "b:1031231", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Version Control
		Attr:         []string{"group:autoupdate"},
		SoftwareDeps: []string{"reboot", "chrome", "crossystem"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
		},
		Timeout: omahaInvalidationTimeout,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceAutoUpdateDisabled{}, pci.Served),
			pci.SearchFlag(&policy.RebootAfterUpdate{}, pci.Served),
			pci.SearchFlag(&policy.UptimeLimit{}, pci.Served),
		},
	})
}

func OmahaInvalidation(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTimeout)
	defer cancel()

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(cleanupCtx)
	if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer func(ctx context.Context) {
		if cl != nil {
			cl.Close(ctx)
		}
	}(cleanupCtx)

	// Enroll the DUT and set the policy.
	pb := policy.NewBlob()
	// Disable autoupdates and reboot after updates explicitly,
	// so that they do not conflict with the test.
	pb.AddPolicies([]policy.Policy{
		&policy.UptimeLimit{Val: 0},
		&policy.DeviceAutoUpdateDisabled{Val: false},
		&policy.RebootAfterUpdate{Val: false},
	})
	pJSON, err := json.Marshal(pb)
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)
	if _, err := policyClient.EnrollUsingChrome(ctx, &pspb.EnrollUsingChromeRequest{
		PolicyJson: pJSON,
		SkipLogin:  true,
	}); err != nil {
		s.Fatal("Failed to enroll: ", err)
	}
	defer func(ctx context.Context) {
		if cl != nil {
			// Recreating since the client can have a stale RPC connection
			// after the reboot.
			policyClient := pspb.NewPolicyServiceClient(cl.Conn)
			policyClient.StopChromeAndFakeDMS(ctx, &empty.Empty{})
		}
	}(cleanupCtx)

	// Save current root and firmware partition information
	// to verify the invalidation later.
	rootdev, err := s.DUT().Conn().CommandContext(ctx, "rootdev", "-s").Output()
	if err != nil {
		s.Fatal("Failed to get current root partition: ", err)
	}

	mainFWAct, err := updateutil.GetCrossystemFlag(ctx, s.DUT(), updateutil.MainFWActFlag)
	if err != nil {
		s.Fatal("Failed to get current firmware partition: ", err)
	}

	// Read the current builder path from the lsb file
	// for the in-place update.
	lsbContent := map[string]string{
		lsbrelease.BuilderPath: "",
	}
	if err := updateutil.FillFromLSBRelease(ctx, s.DUT(), s.RPCHint(), lsbContent); err != nil {
		s.Fatal("Failed to read from lsb file: ", err)
	}
	// Builder path is used in selecting the update image.
	builderPath := lsbContent[lsbrelease.BuilderPath]
	if builderPath == "" {
		s.Fatal("Builder path is missing")
	}

	// Update the DUT in-place.
	if err := updateutil.UpdateFromGS(ctx, s.DUT(), s.OutDir(), s.RPCHint(), builderPath); err != nil {
		s.Fatal("Failed to update image: ", err)
	}

	if err := updateutil.FakeFirmwareUpdate(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to fake firmware update: ", err)
	}
	defer func(ctx context.Context) {
		if err := updateutil.RevertFakeFirmwareUpdate(ctx, s.DUT()); err != nil {
			s.Error("Failed to reset fake firmware update: ", err)
		}
	}(cleanupCtx)

	// Configure nebraska to invalidate the installed update.
	nebraskaClient := nebraska.NewServiceClient(cl.Conn)
	startResponse, err := nebraskaClient.Start(ctx, &nebraska.StartRequest{})
	if err != nil {
		s.Fatal("Failed to start Nebraska")
	}
	defer func(ctx context.Context) {
		if cl != nil {
			// Recreating since the client can have a stale RPC connection
			// after the reboot.
			nebraskaClient := nebraska.NewServiceClient(cl.Conn)
			nebraskaClient.Stop(ctx, &empty.Empty{})
		}
	}(cleanupCtx)

	if _, err := nebraskaClient.SetInvalidateLastUpdate(ctx, &nebraska.SetInvalidateLastUpdateRequest{InvalidateLastUpdate: true}); err != nil {
		s.Fatal("Failed to configure Nebraska for invalidation")
	}

	// Force an update check to invalidate the update.
	updateClient := autoupdate.NewUpdateServiceClient(cl.Conn)
	updateClient.CheckForUpdate(ctx, &autoupdate.UpdateRequest{
		OmahaUrl: fmt.Sprintf("http://127.0.0.1:%d/update", startResponse.Port),
	})

	// Verify that the update is invalidated.
	if err := updateutil.VerifyInvalidatedUpdate(ctx, s.DUT(), mainFWAct, string(rootdev)); err != nil {
		s.Fatal("Failed to verify the update invalidation: ", err)
	}

	// Reboot and try to ssh into a device
	// to verify that the device still boots properly.
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	// Recreating because the RPC connection is stale after the reboot.
	cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	if err := updateutil.VerifyUpdateInvalidationPostReboot(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to verify that a device is in a valid state after reboot: ", err)
	}
}
