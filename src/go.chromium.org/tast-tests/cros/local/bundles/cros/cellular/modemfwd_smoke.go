// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/modemfwd"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModemfwdSmoke,
		Desc:         "Verifies that modemfwd initializes without errors",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active"},
		Fixture:      "cellular",
		SoftwareDeps: []string{"modemfwd"},
		Timeout:      7 * time.Minute,
		Requirements: []string{"cell-fwDrv-0004-v01"},
	})
}

// ModemfwdSmoke Test
func ModemfwdSmoke(ctx context.Context, s *testing.State) {

	if modemfwd.GetAutoUpdatePrefValue(ctx) {
		os.Remove(modemfwd.DisableAutoUpdatePref)
		s.Fatalf("%q file found. This file was not properly removed in another test. Deleting it now", modemfwd.DisableAutoUpdatePref)
	}

	// modemfwd is initially stopped in the fixture SetUp
	if err := modemfwd.StartAndWaitForQuiescence(ctx); err != nil {
		s.Fatal("modemfwd failed during initialization: ", err)
	}
	s.Log("modemfwd has started successfully")
	if err := upstart.StopJob(ctx, modemfwd.JobName); err != nil {
		s.Fatalf("Failed to stop %q: %s", modemfwd.JobName, err)
	}
	s.Log("modemfwd has stopped successfully")
}
