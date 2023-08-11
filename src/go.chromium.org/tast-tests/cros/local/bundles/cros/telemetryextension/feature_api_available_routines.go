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
		Func:           FeatureAPIAvailableRoutines,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Tests chrome.os.diagnostics.getAvailableRoutines Chrome Extension API function exposed to Telemetry Extension and check availability of all routines",
		Contacts:       []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr:         []string{"group:telemetry_extension_hw"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.TelemetryExtension,
		Params: []testing.Param{
			// Available everywhere.
			{
				Name: "ac_power",
				Val:  "ac_power",
			},
			{
				Name: "cpu_cache",
				Val:  "cpu_cache",
			},
			{
				Name: "cpu_stress",
				Val:  "cpu_stress",
			},
			{
				Name: "cpu_floating_point_accuracy",
				Val:  "cpu_floating_point_accuracy",
			},
			{
				Name: "cpu_prime_search",
				Val:  "cpu_prime_search",
			},
			{
				Name: "dns_resolver_present",
				Val:  "dns_resolver_present",
			},
			{
				Name: "dns_resolution",
				Val:  "dns_resolution",
			},
			{
				Name: "gateway_can_be_pinged",
				Val:  "gateway_can_be_pinged",
			},
			{
				Name: "lan_connectivity",
				Val:  "lan_connectivity",
			},
			{
				Name: "memory",
				Val:  "memory",
			},
			{
				Name: "sensitive_sensor",
				Val:  "sensitive_sensor",
			},
			{
				Name: "signal_strength",
				Val:  "signal_strength",
			},
			// Depend on a battery.
			{
				Name: "battery_health",
				Val:  "battery_health",
			},
			{
				Name: "battery_capacity",
				Val:  "battery_capacity",
			},
			{
				Name: "battery_discharge",
				Val:  "battery_discharge",
			},
			{
				Name: "battery_charge",
				Val:  "battery_charge",
			},
			// Depend on an eMMC disk
			{
				Name: "emmc_lifetime",
				Val:  "emmc_lifetime",
			},
			// Depend on an NVMe capable disk.
			{
				Name: "nvme_wear_level",
				Val:  "nvme_wear_level",
			},
			{
				Name: "nvme_self_test",
				Val:  "nvme_self_test",
			},
			// Depend on SMART support.
			{
				Name: "smart_ctl_check",
				Val:  "smartctl_check",
			},
			{
				Name: "smartctl_check_with_percentage_used",
				Val:  "smartctl_check_with_percentage_used",
			},
			// Depend on FIO support.
			{
				Name: "disk_read",
				Val:  "disk_read",
			},
			// Depend on corresponding sensor
			{
				Name: "fingerprint_alive",
				Val:  "fingerprint_alive",
			},
		},
	})
}

func FeatureAPIAvailableRoutines(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	routineName, ok := s.Param().(string)
	if !ok {
		s.Fatal("Failed to convert params value into string: ", s.Param())
	}

	type response struct {
		Routines []string `json:"routines"`
	}

	var resp response
	if err := v.ExtConn.Call(ctx, &resp,
		"tast.promisify(chrome.os.diagnostics.getAvailableRoutines)",
	); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	contains := func(list []string, want string) bool {
		for _, elem := range list {
			if elem == want {
				return true
			}
		}
		return false
	}

	if !contains(resp.Routines, routineName) {
		s.Errorf(`Unexpected result from "getAvailableRoutines": %q is not present in %v as expected`, routineName, resp.Routines)
	}
}
