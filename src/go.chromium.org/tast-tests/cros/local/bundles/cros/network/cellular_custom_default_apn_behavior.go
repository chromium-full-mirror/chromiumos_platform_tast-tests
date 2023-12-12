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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularCustomDefaultApnBehavior,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the correct connect behavior for a custom default APN",
		Contacts: []string{
			"cros-connectivity@google.com",
			"gordonseto@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e", "cellular_carrier_dependent", "cellular_carrier_att"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularResetShillProfileOnPostTest",
		Timeout:      9 * time.Minute,
	})
}

func CellularCustomDefaultApnBehavior(ctx context.Context, s *testing.State) {
	// In case roaming is required for the SIM on the device.
	if err := cellular.SetRoamingPolicy(ctx, true, true); err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

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

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(cleanupCtx)

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

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, userFriendlyApnName, ""); err != nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}

	// Disable default APN.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to disable: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.LeftClick(ossettings.DisableBtn)(ctx); err != nil {
		s.Fatal("Failed to click on disable button of default APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnDisabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to verify Default APN disabled successfully: ", err)
	}

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, userFriendlyApnName, "modb"); err != nil {
		s.Fatal("Failed to verify modb connected UI: ", err)
	}

	// Enable default APN.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnDisabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to enable: ", err)
	}

	if err := ui.LeftClick(ossettings.EnableBtn)(ctx); err != nil {
		s.Fatal("Failed to click on enable button of default APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to verify Default APN enabled successfully: ", err)
	}

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, userFriendlyApnName, ""); err != nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}

	// Edit default APN.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to edit: ", err)
	}

	if err := ui.LeftClick(ossettings.EditBtn)(ctx); err != nil {
		s.Fatal("Failed to click on edit button of default APN: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)
	if err := uiauto.Combine("Edit custom APN in edit APN dialog",
		ui.WaitUntilExists(ossettings.NameOfAPNInput),
		kb.TypeAction("_invalid"),
		ui.LeftClick(nodewith.Name("Save").Role(role.Button)),
		// TODO(b/314192325): Update this to include "_invalid".
		ui.WaitUntilExists(nodewith.NameContaining(apnName)),
	)(ctx); err != nil {
		s.Fatal("Failed to edit custom APN and verify it shows in the APN list: ", err)
	}

	if err := ui.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
		s.Fatal("Failed to navigate back to mobile data subpage from APN subpage: ", err)
	}

	if err := ui.Exists(ossettings.ConnectButton)(ctx); err == nil {
		if err := uiauto.Combine("Connect to network attempt",
			ui.LeftClick(ossettings.ConnectButton),
			ui.WithTimeout(10*time.Second).WaitUntilExists(ossettings.DisconnectedStatus),
		)(ctx); err != nil {
			s.Fatal("Connect attempt didn't result in disconnection: ", err)
		}
	}

	if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn, false /*isFromMobileDataSubpage*/); err != nil {
		s.Fatal("Failed to go to apn subpage: ", err)
	}

	if err := mdp.VerifyAPNSubpageNotConnectedApnUI(ctx, tconn, cr, userFriendlyApnName); err != nil {
		s.Fatal("Failed verify connect failed UI: ", err)
	}

	// Remove custom APN.
	if err := ossettings.ClickAPNMoreActionsButtonOfType(ctx, tconn, apnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to click on more actions button of default APN to remove: ", err)
	}

	if err := ui.LeftClick(ossettings.RemoveBtn)(ctx); err != nil {
		s.Fatal("Failed to click on remove button of default APN: ", err)
	}

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, userFriendlyApnName, "modb"); err != nil {
		s.Fatal("Failed to verify modb connected UI: ", err)
	}
}
