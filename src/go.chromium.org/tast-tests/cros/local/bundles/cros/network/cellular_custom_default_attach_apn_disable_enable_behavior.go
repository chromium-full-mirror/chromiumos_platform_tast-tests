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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularCustomDefaultAttachApnDisableEnableBehavior,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the correct connect behavior for a custom APN that needs to be enabled as default and attach is created separately",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e", "cellular_carrier_dependent", "cellular_carrier_softbank"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularResetShillProfileOnPostTest",
		Timeout:      9 * time.Minute,
	})
}

func CellularCustomDefaultAttachApnDisableEnableBehavior(ctx context.Context, s *testing.State) {
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

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(ctx)

	if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn, true /*isFromMobileDataSubpage*/); err != nil {
		s.Fatal("Failed to go to apn subpage: ", err)
	}

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	if err != nil {
		s.Fatal("Error getting Service properties: ", err)
	}
	apnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	authenticationType := ossettings.GetUIStringForAuthenticationType(serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnAuthentication])
	username := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnUsername]
	password := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnPassword]
	ipType := ossettings.GetUIStringForIPType(serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnIPType])
	userFriendlyApnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoUserFriendlyApnName]

	// Add default-only APN.
	if err := mdp.CreateCustomAPN(ctx, &ossettings.ApnConfig{
		Name:               apnName,
		Username:           username,
		Password:           password,
		AuthenticationType: authenticationType,
		IPType:             ipType,
		IsAttach:           false,
		IsDefault:          true,
	}); err != nil {
		s.Fatal("Failed to add default custom APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to verify Default APN added successfully: ", err)
	}

	// Add attach-only APN.
	if err := mdp.CreateCustomAPN(ctx, &ossettings.ApnConfig{
		Name:               apnName,
		Username:           username,
		Password:           password,
		AuthenticationType: authenticationType,
		IPType:             ipType,
		IsAttach:           true,
		IsDefault:          false,
	}); err != nil {
		s.Fatal("Failed to add attach custom APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnEnabled, true /*isAttach*/, false /*isDefault*/); err != nil {
		s.Fatal("Failed to verify Attach APN added successfully: ", err)
	}

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, userFriendlyApnName, ""); err != nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}

	// Attempt to disable default APN that is currently enabled. Should not be possible.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to disable: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.LeftClick(ossettings.DisableBtn)(ctx); err != nil {
		s.Fatal("Failed to click on disable button of default APN: ", err)
	}

	if err := mdp.VerifyErrorToastMessageIsShowing(ctx, tconn, cr); err != nil {
		s.Fatal("Failed to verify error toast is showing after attempting to disable default APN: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, userFriendlyApnName, ""); err != nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}

	// Attempt to remove default APN that is currently enabled. Should not be possible.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to disable: ", err)
	}

	if err := ui.LeftClick(ossettings.RemoveBtn)(ctx); err != nil {
		s.Fatal("Failed to click on disable button of default APN: ", err)
	}

	if err := mdp.VerifyErrorToastMessageIsShowing(ctx, tconn, cr); err != nil {
		s.Fatal("Failed to verify error toast is showing after attempting to delete default APN: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, userFriendlyApnName, ""); err != nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}

	// Disable attach APN and verify disconnection.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnEnabled, true /*isAttach*/, false /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of attach APN to disable: ", err)
	}

	if err := ui.LeftClick(ossettings.DisableBtn)(ctx); err != nil {
		s.Fatal("Failed to click on disable button of attach APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnDisabled, true /*isAttach*/, false /*isDefault*/); err != nil {
		s.Fatal("Failed to disable default APN: ", err)
	}

	if err := mdp.VerifyAPNSubpageNotConnectedApnUI(ctx, tconn, cr, apnName); err != nil {
		s.Fatal("Failed to verify APN no longer connected: ", err)
	}

	defaultApnState := ossettings.ApnEnabled
	if helper.IsConnected(ctx) != nil {
		defaultApnState = ossettings.ApnConnected
	}
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, defaultApnState, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of attach APN to disable: ", err)
	}

	if err := ui.LeftClick(ossettings.DisableBtn)(ctx); err != nil {
		s.Fatal("Failed to click on disable button of default APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnDisabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to disable default APN: ", err)
	}

	// Enable currently disabled default APN.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnDisabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to enable: ", err)
	}

	if err := ui.LeftClick(ossettings.EnableBtn)(ctx); err != nil {
		s.Fatal("Failed to click on enable button of default APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to enable default APN: ", err)
	}

	// Enable currently enabled attach APN and verify connection.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnDisabled, true /*isAttach*/, false /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of attach APN to enable")
	}

	if err := ui.LeftClick(ossettings.EnableBtn)(ctx); err != nil {
		s.Fatal("Failed to click on enable button of attach APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnEnabled, true /*isAttach*/, false /*isDefault*/); err != nil {
		s.Fatal("Failed to enable attach APN: ", err)
	}

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, userFriendlyApnName, ""); err != nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}

	// Delete currently enabled attach APN.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnEnabled, true /*isAttach*/, false /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to disable: ", err)
	}

	if err := ui.LeftClick(ossettings.RemoveBtn)(ctx); err != nil {
		s.Fatal("Failed to click on disable button of default APN: ", err)
	}

	// Delete currently enabled default APN.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to disable: ", err)
	}

	if err := ui.LeftClick(ossettings.RemoveBtn)(ctx); err != nil {
		s.Fatal("Failed to click on disable button of default APN: ", err)
	}
}
