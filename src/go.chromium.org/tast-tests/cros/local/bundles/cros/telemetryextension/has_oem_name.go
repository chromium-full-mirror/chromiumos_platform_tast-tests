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
		Func:           HasOEMName,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that DUT has correct OEM name",
		Contacts:       []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr:         []string{"group:telemetry_extension_hw"},
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
		},
	})
}

// HasOEMName tests that DUT has correct OEM name which comes from
//   - /sys/devices/virtual/dmi/id/sys_vendor (old approach) or
//   - /sys/firmware/vpd/ro/oem_name (new approach for unreleased models) or
//   - CrOSConfig (new approach).
func HasOEMName(ctx context.Context, s *testing.State) {
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
