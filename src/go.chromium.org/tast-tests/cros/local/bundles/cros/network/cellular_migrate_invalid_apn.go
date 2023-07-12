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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularMigrateInvalidApn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the correctness of the UI for an invalid APN that is migrated to the new UI",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      5 * time.Minute,
	})
}

func CellularMigrateInvalidApn(ctx context.Context, s *testing.State) {
	// In case roaming is required for the SIM on the device.
	if err := cellular.SetRoamingPolicy(ctx, true, true); err != nil {
		s.Fatal("Failed to set roaming policy: ", err)
	}

	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper (precondition): ", err)
	}

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	serviceLastGoodAPNInfoApnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	serviceLastGoodAPNInfoApnSource := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnSource]

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		if err := helper.ClearCustomAPNList(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clear cellular.CustomAPNList: ", err)
		}
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	invalidAPNToMigrate := "INVALIDAPN"
	cr, err := chrome.New(ctx, chrome.DisableFeatures("ApnRevamp"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if _, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr); err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	if err := ossettings.SelectPreRevampOtherAPN(ctx, tconn, "Other"); err != nil {
		s.Fatal("Failed to select custom APN: ", err)
	}

	if err := ossettings.EnterPreRevampOtherAPNDetails(ctx, tconn, invalidAPNToMigrate, "", "", false); err != nil {
		s.Fatal("Failed to enter custom APN: ", err)
	}

	cr, err = chrome.New(ctx,
		chrome.EnableFeatures("ApnRevamp"),
		chrome.RemoveNotification(false),
		chrome.KeepState())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	defer mdp.Close(ctx)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn, false); err != nil {
		s.Fatal("Failed to go to APN subpage: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, serviceLastGoodAPNInfoApnName, serviceLastGoodAPNInfoApnSource); err != nil {
		s.Fatal("Failed to verify connected APN UI: ", err)
	}

	// For example, T-Mobile in the US accepts any APN name.
	if serviceLastGoodAPNInfoApnName != invalidAPNToMigrate {
		if err := mdp.VerifyAPNSubpageNotConnectedApnUI(ctx, tconn, cr, invalidAPNToMigrate); err != nil {
			s.Fatal("Failed to verify not connected APN UI: ", err)
		}
	}
}
