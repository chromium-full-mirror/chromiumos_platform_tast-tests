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
		Func:           MigrateInvalidApn,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Tests the correctness of the UI for an invalid APN that is migrated to the new UI",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_carrier_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularEnforceConnectionLocal",
		Timeout:      5 * time.Minute,
	})
}

func MigrateInvalidApn(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// In case roaming is required for the SIM on the device.
	cleanup, err := cellular.SetRoamingPolicy(ctx, true, true)
	if err != nil {
		s.Fatal("Failed to set roaming policy: ", err)
	}
	defer cleanup(cleanupCtx)

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	defer func(ctx context.Context) {
		if err := helper.ClearCustomAPNList(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clear cellular.CustomAPNList: ", err)
		}
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	invalidAPNToMigrate := "INVALIDAPN"
	func(ctx context.Context) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		cr, err := chrome.New(ctx, chrome.DisableFeatures("ApnRevamp"))
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}

		settings, err := ossettings.LaunchAtMobileData(ctx, tconn, cr)
		if err != nil {
			s.Fatal("Failed to open mobile data subpage: ", err)
		}
		defer settings.Close(cleanupCtx)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "pre_revamp_ossettings")

		if err := settings.NavigateToMobileNetworkDetailsPage(cr, ossettings.ActiveCellularBtn)(ctx); err != nil {
			s.Fatal("Failed to move to the network details page: ", err)
		}

		if err := ossettings.SelectPreRevampOtherAPN(ctx, tconn, "Other"); err != nil {
			s.Fatal("Failed to select custom APN: ", err)
		}

		if err := ossettings.EnterPreRevampOtherAPNDetails(ctx, tconn, invalidAPNToMigrate, "", "", false); err != nil {
			s.Fatal("Failed to enter custom APN: ", err)
		}
	}(ctx)

	cr, err := chrome.New(ctx,
		chrome.EnableFeatures("ApnRevamp"),
		chrome.RemoveNotification(false),
		chrome.KeepState())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	settings, err := ossettings.LaunchAtMobileData(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ossettings")

	if err := uiauto.Combine("go to APN page of the active cellular network",
		settings.NavigateToMobileNetworkDetailsPage(cr, ossettings.ActiveCellularBtn),
		settings.NavigateToApnPage(cr),
	)(ctx); err != nil {
		s.Fatal("Failed to move to the APN page: ", err)
	}

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	if err != nil {
		s.Fatal("Failed to get last good APN: ", err)
	}

	var serviceLastGoodAPNInfoApnName = serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoUserFriendlyApnName]
	if serviceLastGoodAPNInfoApnName == "" {
		serviceLastGoodAPNInfoApnName = serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	}
	serviceLastGoodAPNInfoApnSource := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnSource]

	if err := settings.VerifyApnConnected(cr, serviceLastGoodAPNInfoApnName, serviceLastGoodAPNInfoApnSource)(ctx); err != nil {
		s.Fatal("Failed to verify connected APN UI: ", err)
	}

	// For example, T-Mobile in the US accepts any APN name.
	if serviceLastGoodAPNInfoApnName != invalidAPNToMigrate {
		if err := settings.VerifyApnNotConnected(cr, invalidAPNToMigrate)(ctx); err != nil {
			s.Fatal("Failed to verify not connected APN UI: ", err)
		}
	}
}
