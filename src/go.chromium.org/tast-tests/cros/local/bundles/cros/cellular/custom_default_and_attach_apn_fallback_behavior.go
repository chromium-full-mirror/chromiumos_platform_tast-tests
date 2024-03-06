// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CustomDefaultAndAttachApnFallbackBehavior,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the correct UI and connection behavior for a custom default and attach APN",
		Contacts: []string{
			"cros-connectivity@google.com",
			"gordonseto@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e", "cellular_carrier_dependent", "cellular_carrier_att", "cellular_carrier_softbank"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularResetShillProfileOnPostTest",
		Timeout:      9 * time.Minute,
	})
}

func CustomDefaultAndAttachApnFallbackBehavior(ctx context.Context, s *testing.State) {
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

	// Enter details for a new APN that is attach only.
	if err := mdp.OpenNewAPNDialogAndPopulateFields(ctx, &ossettings.ApnConfig{
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

	ui := uiauto.New(tconn)
	if err := ui.CheckRestriction(nodewith.Name("Add").Role(role.Button), restriction.Disabled)(ctx); err != nil {
		s.Fatal("Failed to verify Add button disabled: ", err)
	}

	attachApnWarningText := nodewith.NameContaining("A default APN is required").Role(role.StaticText)
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(attachApnWarningText)(ctx); err != nil {
		s.Fatal("Failed to verify attach-only APN warning exists: ", err)
	}

	if err := ui.LeftClick(ossettings.DefaultAPNCheckbox)(ctx); err != nil {
		s.Fatal("Failed to check default checkbox: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ui.Exists(attachApnWarningText)(ctx); err == nil {
			return errors.Wrap(err, "failed to verify attach-only APN warning doesn't exist")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  2 * time.Second,
		Interval: time.Second,
	}); err != nil {
		s.Fatal("Failed polling for attach-only APN warning: ", err)
	}

	if err := uiauto.Combine("Add and verify APN added",
		ui.LeftClick(nodewith.Name("Add").Role(role.Button)),
		ui.WaitUntilExists(nodewith.NameContaining(apnName).First()),
	)(ctx); err != nil {
		s.Fatal("Failed to add custom APN and verify it shows in the APN list: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, apnName, ossettings.ApnEnabled, true /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to verify Default APN added successfully: ", err)
	}

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, apnName, ""); err != nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}

	invalidApnName := "invalid_apn"
	if err := mdp.CreateCustomAPN(ctx, &ossettings.ApnConfig{
		Name:               invalidApnName,
		Username:           username,
		Password:           password,
		AuthenticationType: authenticationType,
		IPType:             ipType,
		IsAttach:           false,
		IsDefault:          true,
	}); err != nil {
		s.Fatal("Failed to add invalid custom APN: ", err)
	}

	if err := ossettings.VerifyAPNStabilized(ctx, tconn, invalidApnName, ossettings.ApnEnabled, false /*isAttach*/, true /*isDefault*/); err != nil {
		s.Fatal("Failed to verify Default APN added successfully: ", err)
	}

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, apnName, ""); err == nil {
		s.Fatal("Failed to verify connected UI: ", err)
	}
}
