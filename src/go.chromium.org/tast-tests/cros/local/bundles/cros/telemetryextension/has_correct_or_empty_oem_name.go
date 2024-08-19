// Copyright 2022 The ChromiumOS Authors
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
		Func:           HasCorrectOrEmptyOEMName,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that DUT has correct or empty OEM name",
		Contacts:       []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr:         []string{"group:telemetry_extension_hw"},
		Params: []testing.Param{
			{
				Name:              "asus",
				Val:               "ASUS",
				ExtraHardwareDeps: dep.AsusModels(),
				ExtraAttr:         []string{"group:mainline"},
			},
			{
				Name:              "hp",
				Val:               "HP",
				ExtraHardwareDeps: dep.HPModels(),
				ExtraAttr:         []string{"group:mainline"},
			},
			{
				Name:              "lenovo",
				Val:               "Lenovo",
				ExtraHardwareDeps: dep.LenovoModels(),
				ExtraAttr:         []string{"group:mainline", "informational", "group:criticalstaging"},
			},
		},
	})
}

// HasCorrectOrEmptyOEMName tests that DUT has correct or empty OEM name which
// comes from
//   - CrOSConfig or
//   - /sys/firmware/vpd/ro/oem_name (for unreleased models).
//
// Empty OEM names are allowed because what we want to catach is the wrong OEM
// names. For example,
//   - "TBD" or
//   - "OEM_B" when the actual OEM is A.
func HasCorrectOrEmptyOEMName(ctx context.Context, s *testing.State) {
	oemName, ok := s.Param().(string)
	if !ok {
		s.Fatal("Failed to convert params value into string: ", s.Param())
	}

	if vendor, err := vendorutils.FetchVendor(ctx); err != nil {
		s.Error("Failed to read vendor name: ", err)
	} else if got, want := vendor, oemName; got != "" && got != want {
		s.Errorf("Unexpected vendor name: got %q, want %q", got, want)
	}
}
