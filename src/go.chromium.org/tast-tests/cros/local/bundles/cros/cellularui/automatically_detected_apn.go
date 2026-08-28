// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           AutomaticallyDetectedApn,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Tests the correctness of the UI for automatically detected APNs",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_carrier_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularEnforceConnectionLocal",
		Timeout:      9 * time.Minute,
	})
}

func AutomaticallyDetectedApn(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// In case roaming is required for the SIM on the device.
	cleanup, err := cellular.SetRoamingPolicy(ctx, true, true)
	if err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}
	defer cleanup(cleanupCtx)

	cr, err := chrome.New(ctx, chrome.EnableFeatures("ApnRevamp"))
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(ctx)

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	defer func(ctx context.Context) {
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	serviceLastGoodAPNInfoApnUserFriendlyName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoUserFriendlyApnName]
	if serviceLastGoodAPNInfoApnUserFriendlyName == "" {
		serviceLastGoodAPNInfoApnUserFriendlyName = serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	}
	if serviceLastGoodAPNInfoApnUserFriendlyName == "" {
		// If there is no APN in the database, it will display "Modem APN". For more information see b/375010291.
		serviceLastGoodAPNInfoApnUserFriendlyName = "Modem APN"
	}

	serviceLastGoodAPNInfoApnSource := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnSource]
	if err != nil {
		s.Fatal("Error getting Service properties: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	settings, err := ossettings.LaunchAtMobileData(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ossettings")

	if err := settings.NavigateToMobileNetworkDetailsPage(cr, ossettings.ActiveCellularBtn)(ctx); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	if err := settings.VerifyApnIsVisibleInSubtext(ctx, tconn, cr, serviceLastGoodAPNInfoApnUserFriendlyName); err != nil {
		s.Fatal("Failed to go to verify active apn subtext: ", err)
	}

	if err := uiauto.Combine("verify the more actions items present correctly",
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, serviceLastGoodAPNInfoApnUserFriendlyName, serviceLastGoodAPNInfoApnSource),
		settings.ClickMoreActionsButtonWithAPNName(serviceLastGoodAPNInfoApnUserFriendlyName),
		settings.VerifyAPNMoreActionsMenuItemsPresent(false /* hasEnable */, false /* hasDisable */, false /* hasRemove*/),
	)(ctx); err != nil {
		s.Fatal("Failed to verify more action items: ", err)
	}

	if err := settings.CheckAutomaticallyDetectedAPNDetailesDialog(ctx); err != nil {
		s.Fatal("Failed to verify automatically detected APN details dialog: ", err)
	}
}
