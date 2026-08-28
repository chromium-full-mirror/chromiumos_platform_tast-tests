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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CustomDefaultApnBehavior,
		Desc: "Tests the correct connect behavior for a custom default APN",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e", "cellular_carrier_dependent", "cellular_carrier_amarisoft"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularEnforceConnectionAndResetShillProfile",
		Timeout:      2*time.Minute + 5*ossettings.WaitForConnectionTimeout, // This case tries to connect to the network 5 times.
	})
}

func CustomDefaultApnBehavior(ctx context.Context, s *testing.State) {
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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ossettings")

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
		ApnType:            ossettings.ApnIsDefault,
	}
	userFriendlyApnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoUserFriendlyApnName]

	ui := uiauto.New(tconn)
	waitForConnectionState := func(finder *nodewith.Finder) uiauto.Action {
		return ui.WithTimeout(ossettings.WaitForConnectionTimeout).RetryUntil(
			settings.MaybeConnectToApn(cr),
			settings.WaitUntilExists(finder),
		)
	}

	if err := uiauto.Combine("add default-only APN",
		func(ctx context.Context) error { return settings.CreateCustomAPN(ctx, apnConfig) },
		// Settings app should be at APN page at this point.
		settings.VerifyApnStabilized(apnConfig, ossettings.ApnEnabled),
		// Back to network details page to connect to the network.
		settings.DoDefault(ossettings.BackArrowBtn),
		waitForConnectionState(ossettings.ConnectedStatus),
		// Navigate to the APN page to verify that the APN page UI reports it's connected to the newly added APN correctly.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, apnConfig.Name, "" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}

	if err := uiauto.Combine("disable default APN",
		// Settings app should be at APN page at this point.
		settings.ClickMoreActionButtonOfAnAPN(apnConfig, ossettings.ApnConnected),
		settings.DoDefault(ossettings.DisableBtn),
		settings.VerifyApnStabilized(apnConfig, ossettings.ApnDisabled),
		// Back to network details page to ensure the network is connected.
		settings.DoDefault(ossettings.BackArrowBtn),
		waitForConnectionState(ossettings.ConnectedStatus),
		// Navigate to the APN page to check the APN connection state.
		settings.NavigateToApnPage(cr),
		// The network will connect to an automatically detect APN.
		settings.VerifyApnConnected(cr, "", "modb" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to verify APN is connected after disabling default APN: ", err)
	}

	if err := uiauto.Combine("enable default APN",
		// Settings app should be at APN page at this point.
		settings.ClickMoreActionButtonOfAnAPN(apnConfig, ossettings.ApnDisabled),
		settings.DoDefault(ossettings.EnableBtn),
		settings.VerifyApnStabilized(apnConfig, ossettings.ApnEnabled),
		// Back to network details page to ensure the network is connected.
		settings.DoDefault(ossettings.BackArrowBtn),
		waitForConnectionState(ossettings.ConnectedStatus),
		// Navigate to the APN page to verify that the APN page UI reports it's connected to the enabled APN correctly.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, apnConfig.Name, "" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to verify APN is connected after enabling default APN: ", err)
	}

	invalidSuffix := "_invalid"
	invalidApnName := apnConfig.Name + invalidSuffix
	invalidApnConfig := *apnConfig
	invalidApnConfig.Name = invalidApnName

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	if err := uiauto.Combine("edit custom APN in edit APN dialog",
		// Settings app should be at APN page at this point.
		settings.ClickMoreActionButtonOfAnAPN(apnConfig, ossettings.ApnConnected),
		settings.DoDefault(ossettings.EditBtn),
		settings.WaitUntilExists(ossettings.NameOfAPNInput),
		kb.TypeAction(invalidSuffix),
		settings.DoDefault(nodewith.Name("Save").Role(role.Button)),
		settings.WaitUntilExists(nodewith.NameContaining(invalidApnName)),
		// Back to network details page to connect to the network.
		settings.DoDefault(ossettings.BackArrowBtn),
		// The network won't be connected, so wait until the connection attempt is completed.
		waitForConnectionState(ossettings.DisconnectedStatus),
		// Navigate to the APN page to verify that the APN page UI reports the invalid APN is not connected.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnNotConnected(cr, invalidApnName),
	)(ctx); err != nil {
		s.Fatal("Failed to verify network is not connected: ", err)
	}

	if err := uiauto.Combine("remove custom APN",
		// Settings app should be at APN page at this point.
		settings.ClickMoreActionButtonOfAnAPN(&invalidApnConfig, ossettings.ApnEnabled),
		settings.DoDefault(ossettings.RemoveBtn),
		// Back to network details page to ensure the network is connected.
		settings.DoDefault(ossettings.BackArrowBtn),
		waitForConnectionState(ossettings.ConnectedStatus),
		// // Navigate to the APN page to verify that the APN page UI reports it's connected to the APN correctly.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, userFriendlyApnName, "modb" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to verify APN is connected after enabling default APN: ", err)
	}
}
