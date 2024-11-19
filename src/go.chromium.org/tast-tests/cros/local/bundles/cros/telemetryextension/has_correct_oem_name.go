// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/dep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/vendorutils"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           HasCorrectOEMName,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that DUT has correct OEM name",
		Contacts:       []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health > OEM Services
		BugComponent: "b:1256717",
		Attr:         []string{"group:telemetry_extension_hw", "group:mainline", "informational", "group:healthd", "healthd_perbuild"},
		Params: []testing.Param{
			{
				Name:              "asus",
				Val:               "ASUS",
				ExtraHardwareDeps: dep.AsusModels(),
			},
			{
				Name:              "hp",
				Val:               "HP",
				ExtraHardwareDeps: dep.HPModels(),
			},
			{
				Name:              "lenovo",
				Val:               "Lenovo",
				ExtraHardwareDeps: dep.LenovoModels(),
			},
			{
				Name:              "acer",
				Val:               "Acer",
				ExtraHardwareDeps: dep.AcerModels(),
			},
		},
	})
}

// HasCorrectOEMName tests that DUT has correct OEM name which comes from
//   - CrOSConfig or
//   - /sys/firmware/vpd/ro/oem_name (for unreleased models).
//
// This test should not be promoted to critical as we don't plan to updata all
// configs. It is used to know which models do not have the correct OEM name.
func HasCorrectOEMName(ctx context.Context, s *testing.State) {
	oemName, ok := s.Param().(string)
	if !ok {
		s.Fatal("Failed to convert params value into string: ", s.Param())
	}

	if vendor, err := vendorutils.FetchVendor(ctx); err != nil {
		s.Error("Failed to read vendor name: ", err)
	} else if got, want := vendor, oemName; got != want {
		s.Errorf("Unexpected vendor name: got %q, want %q", got, want)
	}
}
