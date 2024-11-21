// Copyright 2023 The ChromiumOS Authors
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
		Func: RunFanRoutine,
		Desc: "Checks that cros_healthd can run fan routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"yycheng@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health
		BugComponent: "b:982097",
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
	})
}

func buildFanRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"fan"}, nil
}

func RunFanRoutine(ctx context.Context, s *testing.State) {
	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildFanRoutineArgs,
		RoutineRunner:  croshealthd.RunDiagV2,
		ResultVerifier: croshealthd.VerifyRoutineV2PassedOrUnsupported,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
