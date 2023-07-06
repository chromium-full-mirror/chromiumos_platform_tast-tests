// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularAutomaticallyDetectedApn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the correctness of the UI for automatically detected APNs",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      9 * time.Minute,
	})
}

func CellularAutomaticallyDetectedApn(ctx context.Context, s *testing.State) {
	// In case roaming is required for the SIM on the device.
	if err := cellular.SetRoamingPolicy(ctx, true, true); err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}

	cr, err := chrome.New(ctx, chrome.EnableFeatures("ApnRevamp"))
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(ctx)

	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	serviceLastGoodAPNInfoApnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	serviceLastGoodAPNInfoApnSource := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnSource]
	if err != nil {
		s.Fatal("Error getting Service properties: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(ctx)

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	if err := mdp.VerifyApnIsVisibleInSubtext(ctx, tconn, cr, serviceLastGoodAPNInfoApnName); err != nil {
		s.Fatal("Failed to go to verify active apn subtext: ", err)
	}

	if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn, false); err != nil {
		s.Fatal("Failed to go to apn subpage: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, serviceLastGoodAPNInfoApnName, serviceLastGoodAPNInfoApnSource); err != nil {
		s.Fatal("Failed to verify connected APN UI: ", err)
	}

	if err := ossettings.VerifyAPNMoreActionsMenuItemsPresent(ctx, tconn, serviceLastGoodAPNInfoApnName, false, false, false); err != nil {
		s.Fatal("Failed verify items in APN more actions menu: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.LeftClick(ossettings.DetailsBtn)(ctx); err != nil {
		s.Fatal("Failed to click details APN menu item: ", err)
	}

	if err := ossettings.CheckAutomaticallyDetectedAPNDetailesDialog(ctx, tconn); err != nil {
		s.Fatal("Failed to verify automatically detected APN details dialog: ", err)
	}
}
