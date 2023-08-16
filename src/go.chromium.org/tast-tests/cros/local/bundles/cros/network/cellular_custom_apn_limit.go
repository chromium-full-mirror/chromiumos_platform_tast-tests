// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularCustomApnLimit,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the UI status when the max custom APN number is hit",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jiajunz@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      5 * time.Minute,
	})
}

func CellularCustomApnLimit(ctx context.Context, s *testing.State) {
	// In case roaming is required for the SIM on the device.
	if err := cellular.SetRoamingPolicy(ctx, true, true); err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}

	cr, err := chrome.New(ctx, chrome.EnableFeatures("ApnRevamp"))
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(ctx)

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	serviceLastGoodAPNInfoApnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoUserFriendlyApnName]
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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

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

	for i := 1; i <= 10; i++ {
		apnName := "custom_apn" + strconv.Itoa(i)
		if err := mdp.CreateCustomAPN(ctx, apnName, "", ""); err != nil {
			s.Fatalf("Failed to add custom APN with name: %s, err: %v", apnName, err)
		}
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Verify New APN button is disabled and show tooltip",
		ui.CheckRestriction(ossettings.NewAPNBtn, restriction.Disabled),
		ui.MouseMoveTo(ossettings.NewAPNBtn, 0),
		ui.WaitUntilExists(ossettings.APNLimitTooltip),
	)(ctx); err != nil {
		s.Fatal("Failed to verify New APN button is disabled and show tooltip: ", err)
	}
}
