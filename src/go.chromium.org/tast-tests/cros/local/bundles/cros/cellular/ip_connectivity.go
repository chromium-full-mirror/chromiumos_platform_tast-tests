// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type ipTestParams struct {
	modbOverrideProto string
	ipv4              bool
	ipv6              bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         IPConnectivity,
		Desc:         "Verifies that traffic can be sent over the Cellular network",
		Contacts:     []string{"chromeos-cellular-team@google.com", "ejcaruso@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Params: []testing.Param{{
			Name:      "ipv4",
			Val:       ipTestParams{"callbox_null_attach_ipv4.pbf", true, false},
			ExtraData: []string{"callbox_null_attach_ipv4.pbf"},
		}, {
			Name:      "ipv6",
			Val:       ipTestParams{"callbox_null_attach_ipv6.pbf", false, true},
			ExtraData: []string{"callbox_null_attach_ipv6.pbf"},
		}, {
			Name:      "ipv4v6",
			Val:       ipTestParams{"callbox_null_attach_ipv4v6.pbf", true, true},
			ExtraData: []string{"callbox_null_attach_ipv4v6.pbf"},
		}, {
			Name:      "ipv4v6_fallback_ipv4",
			Val:       ipTestParams{"callbox_null_attach_ipv4v6_fallback_ipv4.pbf", true, false},
			ExtraData: []string{"callbox_null_attach_ipv4v6_fallback_ipv4.pbf"},
		}, {
			Name:      "ipv4v6_fallback_ipv6",
			Val:       ipTestParams{"callbox_null_attach_ipv4v6_fallback_ipv6.pbf", false, true},
			ExtraData: []string{"callbox_null_attach_ipv4v6_fallback_ipv6.pbf"},
		}},
		Fixture: "cellular",
		Timeout: 2 * time.Minute,
	})
}

func IPConnectivity(ctx context.Context, s *testing.State) {
	params := s.Param().(ipTestParams)
	modbOverrideProto := params.modbOverrideProto
	ipv4Expected := params.ipv4
	ipv6Expected := params.ipv6

	helper := s.FixtValue().(*cellular.FixtData).Helper

	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Failed to disable cellular: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 6*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		// Restart shill after deleting |modbOverrideProto|.
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	cleanup, err := cellular.SetServiceProvidersExclusiveOverride(ctx, s.DataPath(modbOverrideProto))
	if err != nil {
		s.Fatal("Failed to set service providers override: ", err)
	}
	defer cleanup()
	if errs := helper.ResetShill(ctx); errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Unable to connect to service: ", err)
	}

	ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
	if err != nil {
		s.Fatal("Failed to read network provisioned IP types: ", err)
	}
	s.Logf("ipv4: %v, ipv6: %v", ipv4, ipv6)

	if ipv4 != ipv4Expected {
		s.Fatalf("IPv4 connectivity different from expectation: got %v, want %v", ipv4, ipv4Expected)
	}
	if ipv6 != ipv6Expected {
		s.Fatalf("IPv6 connectivity different from expectation: got %v, want %v", ipv6, ipv6Expected)
	}

	verifyHostIPConnectivity := func(ctx context.Context) error {
		if err := cellular.VerifyIPConnectivityUsingCurl(ctx, testexec.CommandContext, ipv4, ipv6, "/usr/bin"); err != nil {
			return errors.Wrap(err, "failed connectivity test")
		}
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPConnectivity); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
