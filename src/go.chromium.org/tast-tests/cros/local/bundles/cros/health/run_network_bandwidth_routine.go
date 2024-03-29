// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunNetworkBandwidthRoutine,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can run network bandwidth routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"byronlee@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		// This test should not be added to mainline group since the network
		// bandwidth routine depends on real server but not mock server.
		Attr:    []string{"group:healthd", "healthd_perbuild"},
		Fixture: "crosHealthdRunning",
	})
}

func buildNetworkBandwidthRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"network_bandwidth"}, nil
}

// RunNetworkBandwidthRoutine runs the network bandwidth routine.
func RunNetworkBandwidthRoutine(ctx context.Context, s *testing.State) {
	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildNetworkBandwidthRoutineArgs,
		RoutineRunner:  croshealthd.RunDiagV2,
		ResultVerifier: croshealthd.VerifyRoutineV2PassedOrUnsupported,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
