// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/local/stork"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StorkProfile,
		Desc: "Verifies that the Stork API can be invoked by a device",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
			"khorimoto@google.com",
			"pholla@google.com",
		},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_test_esim"},
		Timeout:      5 * time.Minute,
	})
}

func StorkProfile(ctx context.Context, s *testing.State) {
	ctxForCleanup := ctx

	ctx, cancel := ctxutil.Shorten(ctx, stork.CleanupProfileTime)
	defer cancel()

	activationCode, cleanupFunc, err := stork.FetchStorkProfile(ctx)
	if err != nil {
		s.Fatal("Failed to fetch Stork profile: ", err)
	}

	defer cleanupFunc(ctxForCleanup)
	s.Log("Fetched Stork profile with activation code: ", activationCode)
}
