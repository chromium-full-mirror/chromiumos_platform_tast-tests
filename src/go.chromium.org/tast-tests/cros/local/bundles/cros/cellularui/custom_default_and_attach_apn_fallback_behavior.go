// Copyright 2024 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CustomDefaultAndAttachApnFallbackBehavior,
		Desc: "Tests the correct UI and connection behavior for a custom default and attach APN",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e", "cellular_carrier_dependent", "cellular_carrier_amarisoft"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularEnforceConnectionAndResetShillProfile",
		Timeout:      9 * time.Minute,
	})
}

func CustomDefaultAndAttachApnFallbackBehavior(ctx context.Context, s *testing.State) {
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
	defer cr.Close(cleanupCtx)

	helper := s.FixtValue().(*cellular.FixtData).Helper

	if err := helper.ClearCustomAPNList(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to clear cellular.CustomAPNList: ", err)
	}

	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	if err := uiauto.Combine("go to APN page of the active cellular network",
		settings.NavigateToMobileNetworkDetailsPage(cr, ossettings.ActiveCellularBtn),
		settings.NavigateToApnPage(cr),
	)(ctx); err != nil {
		s.Fatal("Failed to move to the desired page: ", err)
	}

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	if err != nil {
		s.Fatal("Error getting Service properties: ", err)
	}

	apnConfig := &ossettings.ApnConfig{
		Name:               serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName],
		Username:           serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnUsername],
		Password:           serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnPassword],
		AuthenticationType: ossettings.GetUIStringForAuthenticationType(serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnAuthentication]),
		IPType:             ossettings.GetUIStringForIPType(serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnIPType]),
		ApnType:            ossettings.ApnIsAttach,
	}

	// Enter details for a new APN that is attach only.
	if err := settings.OpenNewAPNDialogAndPopulateFields(ctx, apnConfig); err != nil {
		s.Fatal("Failed to add attach custom APN: ", err)
	}

	ui := uiauto.New(tconn)
	attachApnWarningText := nodewith.NameContaining("A default APN is required").Role(role.StaticText)
	if err := uiauto.Combine("verify the behavior of attach-only APN warning",
		ui.CheckRestriction(nodewith.Name("Add").Role(role.Button), restriction.Disabled),
		settings.WithTimeout(5*time.Second).WaitUntilExists(attachApnWarningText),
		settings.LeftClick(ossettings.DefaultAPNCheckbox),
		settings.WaitUntilGone(attachApnWarningText),
	)(ctx); err != nil {
		s.Fatal("Failed to complete all steps: ", err)
	}

	// The APN type is changed to ApnIsDefault and ApnIsAttach.
	apnConfig.ApnType |= ossettings.ApnIsDefault
	if err := uiauto.Combine("add and verify APN added",
		ui.LeftClick(nodewith.Name("Add").Role(role.Button)),
		ui.WaitUntilExists(nodewith.NameContaining(apnConfig.Name).First()),
		// Settings app should be at APN page at this point.
		settings.VerifyApnStabilized(apnConfig, ossettings.ApnEnabled),
		// Back to network details page to connect to the network.
		settings.DoDefault(ossettings.BackArrowBtn),
		ui.WithTimeout(ossettings.WaitForConnectionTimeout).RetryUntil(
			settings.MaybeConnectToApn(cr),
			settings.WaitUntilExists(ossettings.ConnectedStatus),
		),
		// Navigate to the APN page to verify that the APN page UI reports it's connected to the newly added APN correctly.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, apnConfig.Name, "" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to add custom APN and verify it shows in the APN list: ", err)
	}

	invalidApnConfig := &ossettings.ApnConfig{
		Name:               "invalid_apn",
		Username:           "invalid_user",
		Password:           "invalid_password",
		AuthenticationType: apnConfig.AuthenticationType,
		IPType:             apnConfig.IPType,
		ApnType:            ossettings.ApnIsDefault,
	}

	if err := settings.CreateCustomAPN(ctx, invalidApnConfig); err != nil {
		s.Fatal("Failed to add invalid custom APN: ", err)
	}

	if err := uiauto.Combine("verify invalid apn",
		// Settings app should be at APN page at this point.
		settings.VerifyApnStabilized(invalidApnConfig, ossettings.ApnEnabled),
		// Back to network details page to connect to the network.
		settings.DoDefault(ossettings.BackArrowBtn),
		ui.WithTimeout(ossettings.WaitForConnectionTimeout).RetryUntil(
			settings.MaybeConnectToApn(cr),
			// The network should automatically reconnect to the first network.
			settings.WaitUntilExists(ossettings.ConnectedStatus),
		),
		// Navigate to the APN page to verify that the APN page UI reports it's connected to the newly added APN correctly.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, apnConfig.Name, "" /* source */),
		settings.VerifyApnNotConnected(cr, invalidApnConfig.Name),
	)(ctx); err != nil {
		s.Fatal("Failed to verify invalid APN is not connected: ", err)
	}
}
