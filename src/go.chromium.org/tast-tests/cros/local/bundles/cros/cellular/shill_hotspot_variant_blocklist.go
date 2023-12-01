// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/shill"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillHotspotVariantBlocklist,
		Desc:         "Verifies that Hotspot is disabled on devices in which the Hardware, FW or OEM doesn't allow hotspot",
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
	cellularInUpstreamTechCrOS := slices.Contains(technologies, shill.TechnologyCellular)
	variantInBlocklist := (board == "trogdor" || board == "strongbad")
	if variantInBlocklist && cellularInUpstreamTechCrOS {
		s.Fatal("Hotspot should not be allowed on variant")
	}

	// Verify FW version blocklist
	fw, err := helper.GetFirmwareRevisionFromShill(ctx)
	if err != nil {
		s.Fatal("Unable to get firmware revision from shill: ", err)
	}
	inFirmwareBlocklist := false
	blocklistL850 := []string{"18500.5001.00.02.24.09", "18500.5001.00.03.25.18", "18500.5001.00.04.26.01", "18500.5001.00.04.26.06"}
	blocklistFM350 := []string{"81600.0000.00.29.19.16"}
	for _, prefix := range append(blocklistL850, blocklistFM350...) {
		if strings.HasPrefix(fw, prefix) {
			inFirmwareBlocklist = true
			break
		}
	}
	if inFirmwareBlocklist && cellularInUpstreamTechCrOS {
		s.Fatalf("Hotspot should not be allowed on fw: %q", fw)
	}

	if !variantInBlocklist && !inFirmwareBlocklist && !cellularInUpstreamTechCrOS {
		s.Fatalf("Hotspot not supported, but expected to be supported. board : %q firmware : %q", board, fw)
	}
}
