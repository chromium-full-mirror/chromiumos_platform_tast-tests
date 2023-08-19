// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillHotspotPreconditions,
		Desc:         "Verifies that cellular is ready for the hotspot feature. This test is not a complete test, and it will be continuously updated as progress is made in the hotspot project",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com", "aleksandermj@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:      "cellular",
		Timeout:      2 * time.Minute,
	})
}

func ShillHotspotPreconditions(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 6*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		cellular.CheckIfl850VerizonAndFixDefaultAPN(ctx)
		// Restart shill after test to unload the tethering modb.
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	// TODO(b/267804414): Set tethering Allowed is only needed during fishfooding and can be removed later.
	if err := helper.Manager.SetTetheringAllowed(ctx, true); err != nil {
		s.Log("Unable to set Tethering allowed: ", err)
	}

	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to Cellular for hotspot: ", err)
	}

	status, err := helper.Manager.CheckTetheringReadiness(ctx)
	if err != nil {
		s.Fatalf("Failed to check tethering readiness: %s. Status: %q", err, status)
	}
	if status != shillconst.TetheringReadinessReady {
		s.Fatalf("Got TetheringReadiness %q, want %q", status, shillconst.TetheringReadinessReady)
	}

	// Run connectivity test
	ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
	if err != nil {
		s.Fatal("Failed to read network provisioned IP types: ", err)
	}
	s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)

	verifyHostIPConnectivity := func(ctx context.Context) error {
		if err := cellular.VerifyIPConnectivity(ctx, testexec.CommandContext, ipv4, ipv6, "/bin"); err != nil {
			return errors.Wrap(err, "failed connectivity test")
		}
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPConnectivity); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
