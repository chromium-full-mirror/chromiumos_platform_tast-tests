// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"

	diagcommon "go.chromium.org/tast-tests/cros/common/network/diag"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/diag"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiagFailLANConnectivity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the LANConnectivity network diagnostic test fails when there is no ethernet",
		Contacts: []string{
			"cros-network-health-team@google.com", // network-health team
			"khegde@chromium.org",                 // test maintainer
			"stevenjb@chromium.org",               // network-health tech lead
		},
		BugComponent: "b:1166446",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		// Use ehide to hide the Ethernet interface.
		Fixture: "networkDiagnosticsShillReset.ehide",
	})
}

// DiagFailLANConnectivity tests that when there is no Ethernet interface,
// the LANConnectivity network diagnostic routine fails.
func DiagFailLANConnectivity(ctx context.Context, s *testing.State) {
	mojo := s.FixtValue().(*diag.MojoAPI)
	// After the property change is emitted, Chrome still needs to process it.
	// Since Chrome does not emit a change, poll to test whether the expected
	// problem occurs.
	const problemNoLanConnectivity uint32 = 0
	expectedResult := &diagcommon.RoutineResult{
		Verdict:  diagcommon.VerdictProblem,
		Problems: []uint32{problemNoLanConnectivity},
	}
	if err := mojo.PollRoutine(ctx, diagcommon.RoutineLanConnectivity, expectedResult); err != nil {
		s.Fatal("Failed to poll routine: ", err)
	}
}
