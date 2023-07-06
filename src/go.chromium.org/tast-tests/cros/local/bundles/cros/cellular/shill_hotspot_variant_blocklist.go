// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"golang.org/x/exp/slices"

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
		Attr:         []string{"group:cellular", "cellular_sim_active"},
		Fixture:      "cellular",
		Timeout:      2 * time.Minute,
	})
}

func ShillHotspotVariantBlocklist(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	if err := helper.Manager.SetExperimentalTetheringFunctionality(ctx, false); err != nil {
		s.Fatal("Unable to set ExperimentalTetheringFunctionality: ", err)
	}

	technologies, err := helper.Manager.GetTetheringCapabilityUpstreamTechnologies(ctx)
	if err != nil {
		s.Fatal("Failed to get Upstream Technologies property: ", err)
	}

	board, err := cellular.GetBoard(ctx)
	if err != nil {
		s.Fatalf("Failed to get board: %s", err)
	}
	shouldSupportHotspot := !(board == "trogdor" || board == "strongbad")
	cellularInUpstreamTechCrOS := slices.Contains(technologies, shill.TechnologyCellular)
	if shouldSupportHotspot != cellularInUpstreamTechCrOS {
		s.Fatalf("Hotspot variant support mismatch, got: %t want: %t", cellularInUpstreamTechCrOS, shouldSupportHotspot)
	}
}
