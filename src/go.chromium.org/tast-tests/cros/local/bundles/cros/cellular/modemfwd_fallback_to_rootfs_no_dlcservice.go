// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/modemfwd"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModemfwdFallbackToRootfsNoDlcservice,
		Desc:         "Verifies that modemfwd can fallback to the rootfs FW images when dlcservice is not running",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_modem_verification"},
		Fixture:      "cellular",
		SoftwareDeps: []string{"modemfwd"},
		Timeout:      3 * time.Minute,
	})
}

// ModemfwdFallbackToRootfsNoDlcservice Test
func ModemfwdFallbackToRootfsNoDlcservice(ctx context.Context, s *testing.State) {
	if err := upstart.StopJob(ctx, dlc.JobName); err != nil {
		s.Fatalf("Failed to stop %q: %s", dlc.JobName, err)
	}
	s.Log("dlcservice was stopped successfully")
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	// Ensure the test restores the modemfwd state.
	defer func(ctx context.Context) {
		ctx, st := timing.Start(ctx, "cleanUp")
		defer st.End()
		if err := upstart.StartJobAndWaitForDbusService(ctx, dlc.JobName, dlc.ServiceName); err != nil {
			s.Fatal("Failed to start dlcservice: ", err)
		}
		s.Log("dlcservice has started successfully")
	}(cleanupCtx)

	defer func(ctx context.Context) {
		if err := upstart.StopJob(ctx, modemfwd.JobName); err != nil {
			s.Fatalf("Failed to stop %q: %s", modemfwd.JobName, err)
		}
		s.Log("modemfwd has stopped successfully")
	}(ctx)
	// modemfwd is initially stopped in the fixture SetUp
	if err := modemfwd.StartAndWaitForQuiescence(ctx); err != nil {
		s.Fatal("modemfwd failed during initialization: ", err)
	}

}
