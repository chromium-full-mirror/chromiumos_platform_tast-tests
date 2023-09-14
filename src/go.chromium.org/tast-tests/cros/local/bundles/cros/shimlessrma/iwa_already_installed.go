// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shimlessrma

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/shimlessrma/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IWAAlreadyInstalled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check if we can open IWA(Isolated Web App) directly if it's already installed",
		Contacts: []string{
			"chromeos-shimless-eng@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Shimless RMA
		BugComponent: "b:1002147",
		// These attributes make the test suite to run on the shimless supported models.
		Attr:         []string{"group:shimless_rma", "shimless_rma_normal"},
		Fixture:      fixture.InstallIWA,
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
	})
}

func IWAAlreadyInstalled(ctx context.Context, s *testing.State) {
	// fixture.InstallIWA already installs the IWA.
	// Now we want to relaunch a new chrome and enter the flow again, to see if we can skip the install flow and load the IWA.
	v := s.FixtValue().(*fixture.Value)
	if err := v.RestartChrome(ctx); err != nil {
		s.Fatal("Failed to new chrome, err: ", err)
	}
	if err := v.Launch3pDiagApp(ctx, false /* withUSB= */); err != nil {
		s.Fatal("Failed to enter install flow, err: ", err)
	}
}
