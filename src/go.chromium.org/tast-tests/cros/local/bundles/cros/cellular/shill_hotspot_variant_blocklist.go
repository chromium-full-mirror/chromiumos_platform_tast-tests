// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/shill"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillHotspotVariantBlocklist,
		Desc:         "Verifies that Hotspot is disabled on devices in which the Hardware or OEM doesn't allow hotspot",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:      "cellular",
		Timeout:      2 * time.Minute,
	})
}

func ShillHotspotVariantBlocklist(ctx context.Context, s *testing.State) {
	helper, _, err := cellular.NewHelperWithSim(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper (precondition): ", err)
	}

	technologies, err := helper.Manager.GetTetheringCapabilityUpstreamTechnologies(ctx)
	if err != nil {
		s.Fatal("Failed to get Upstream Technologies property: ", err)
	}

	if !contains(technologies, shill.TechnologyCellular) {
		s.Fatal("Variant does not support hotspot over cellular")
	}
}

// contains returns whether the slice "list" contains "s".
// This can be replaced with slices.Contains() once Go releases the slices
// package: https://pkg.go.dev/golang.org/x/exp/slices#Contains
func contains(list []shill.Technology, s shill.Technology) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
