// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlatformAPIAvailableRoutines,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests chrome.os.diagnostics.getAvailableRoutines Chrome Extension API function exposed to Telemetry Extension",
		Contacts:     []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:mainline",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:      "ash",
				Fixture:   fixture.TelemetryExtensionSkipOEMNameCheck,
				ExtraAttr: []string{"informational", "group:criticalstaging"},
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.TelemetryExtensionSkipOEMNameCheckLacros,
			},
		},
	})
}

// PlatformAPIAvailableRoutines tests chrome.os.diagnostics.getAvailableRoutines Chrome Extension API functionality.
func PlatformAPIAvailableRoutines(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	type response struct {
		Routines []string `json:"routines"`
	}

	var resp response
	if err := v.ExtConn.Call(ctx, &resp,
		"tast.promisify(chrome.os.diagnostics.getAvailableRoutines)",
	); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	gotRoutines := make(map[string]struct{})
	for _, got := range resp.Routines {
		gotRoutines[got] = struct{}{}
	}

	wantRoutines := []string{
		"ac_power",
		"cpu_cache",
		"cpu_stress",
		"cpu_floating_point_accuracy",
		"cpu_prime_search",
		"dns_resolution",
		"dns_resolver_present",
		"gateway_can_be_pinged",
		"lan_connectivity",
		"memory",
		"sensitive_sensor",
		"signal_strength",
	}

	for _, want := range wantRoutines {
		if _, exist := gotRoutines[want]; !exist {
			s.Errorf("Wanted %q routine is missing in available routines %v", want, resp.Routines)
		}
	}
}
