// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiscoverApns,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the correct connect behavior for adding known APNs",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e", "cellular_carrier_dependent", "cellular_carrier_att"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularResetShillProfileOnPostTest",
		Timeout:      9 * time.Minute,
	})
}

func DiscoverApns(ctx context.Context, s *testing.State) {
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

	firstAPNName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	if err := ossettings.OpenDiscoverAPNDialogFromAPNSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to open discover APN dialog: ", err)
	}

	if err := ossettings.SelectAPNFromDialog(ctx, tconn, firstAPNName); err != nil {
		s.Fatal("Failed to add known APN: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(nodewith.NameContaining(firstAPNName).Role(role.Button))(ctx); err != nil {
		s.Fatal("Error to show added APN status: ", err)
	}

	if err := ossettings.GoConnectIfNotConnectedThenReturnApnSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure successful connection: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, firstAPNName, "ui"); err != nil {
		s.Fatal("Error to verify APN subpage connected status: ", err)
	}

	if err := ossettings.VerifyOnlyThisAPNEnabled(ctx, tconn, cr, firstAPNName); err != nil {
		s.Fatal("Error to verify there is only one enabled APN: ", err)
	}

	if err := ossettings.OpenDiscoverAPNDialogFromAPNSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to open discover APN dialog: ", err)
	}

	knownAPNs, err := cellular.GetKnownApns(ctx)
	if err != nil {
		s.Fatal("Error getting known APNs: ", knownAPNs)
	}

	secondAPNName := firstAPNName
	for _, knownAPN := range knownAPNs {
		apnName := fmt.Sprintf("%v", knownAPN.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnName])
		// Prefer selecting an APN that is different from the one that was added the first time
		if apnName == firstAPNName {
			continue
		}
		apnSelection := nodewith.NameContaining(apnName).Role(role.StaticText)
		if err := ui.WaitUntilExists(apnSelection)(ctx); err != nil {
			continue
		}
		secondAPNName = apnName
		break
	}

	if err := ossettings.SelectAPNFromDialog(ctx, tconn, secondAPNName); err != nil {
		s.Fatal("Failed to add known APN: ", err)
	}

	if err := ossettings.VerifyOnlyThisAPNEnabled(ctx, tconn, cr, secondAPNName); err != nil {
		s.Fatal("Error to verify there is only one enabled APN: ", err)
	}
}
