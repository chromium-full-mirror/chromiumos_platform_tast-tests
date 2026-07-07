// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ConnectedStatus,
		Desc: "Checks that active primary SIM is displayed correctly",
		Contacts: []string{
			"cros-device-enablement@google.com",
		},
		BugComponent:   "b:1131774", // ChromeOS > Software > Fundamentals > Device Enablement > Connectivity > Cellular
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:cellular", "cellular_sim_dual_active"},
		SoftwareDeps:   []string{"chrome"},
		Fixture:        "cellularEnforceConnectionLocal",
		Timeout:        5 * time.Minute,
	})
}

func ConnectedStatus(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "os_settings")

	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}
	firstIccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, firstIccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
	}

	if err := mdp.WaitUntilExists(ossettings.NotActiveCellularBtn)(ctx); err != nil {
		s.Fatal("Failed to find not connected network(s): ", err)
	}
	if err := mdp.LeftClick(ossettings.NotActiveCellularRows.First())(ctx); err != nil {
		s.Fatal("Failed to click into not active cellular row: ", err)
	}
	if err := ossettings.WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	// Ensure mobile data page is open as it may have navigated away.
	_, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	secondIccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}
	if firstIccid == secondIccid {
		s.Fatal("Failed to connect to a different cellular network")
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, secondIccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
	}
}
