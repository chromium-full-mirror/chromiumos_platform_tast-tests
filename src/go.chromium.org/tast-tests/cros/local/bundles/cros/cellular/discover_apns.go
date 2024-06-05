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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiscoverApns,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the correct connect behavior for adding known APNs",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e"},
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

	helper := s.FixtValue().(*cellular.FixtData).Helper

	if err := helper.ClearCustomAPNList(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to clear cellular.CustomAPNList: ", err)
	}

	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	if err != nil {
		s.Fatal("Error getting Service properties: ", err)
	}

	firstAPNName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]

	knownAPNs, err := cellular.GetKnownApns(ctx)
	if err != nil {
		s.Fatal("Error getting known APNs: ", knownAPNs)
	}

	cr, err := chrome.New(ctx, chrome.EnableFeatures("ApnRevamp"))
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ossettings")

	if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn, true /*isFromMobileDataSubpage*/); err != nil {
		s.Fatal("Failed to go to apn subpage: ", err)
	}

	if err := ossettings.OpenDiscoverAPNDialogFromAPNSubpage(ctx, tconn); err != nil {
		s.Fatal("Failed to open discover APN dialog: ", err)
	}

	if err := ossettings.SelectAPNFromDialog(ctx, tconn, firstAPNName); err != nil {
		s.Fatal("Failed to add known APN: ", err)
	}

	if err := navigateFromNetworkMainOrCellularDetailsIfNeeded(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate from network or main cellular details pages: ", err)
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

	ui := uiauto.New(tconn)
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

	if err := navigateFromNetworkMainOrCellularDetailsIfNeeded(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate from network or main cellular details pages: ", err)
	}

	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(nodewith.NameContaining(secondAPNName).NameContaining("connected").Role(role.Button))(ctx); err != nil {
		s.Fatal("Error to show added APN status: ", err)
	}

	if err := ossettings.VerifyOnlyThisAPNEnabled(ctx, tconn, cr, secondAPNName); err != nil {
		s.Fatal("Error to verify there is only one enabled APN: ", err)
	}
}

func navigateFromNetworkMainOrCellularDetailsIfNeeded(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	mobileButton := nodewith.Name("Mobile data").Role(role.Button)
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(mobileButton.Focusable())(ctx); err == nil {
		testing.ContextLog(ctx, "Currently at all network settings")
		if err := ui.LeftClick(mobileButton.Focusable())(ctx); err != nil {
			return errors.Wrap(err, "failed to go to from Network subpage to mobile data subpage")
		}
		if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn /*isFromMobileDataSubpage=*/, true); err != nil {
			return errors.Wrap(err, "failed to go to from Network subpage to active network's APN settings")
		}
	} else if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(ossettings.APNSubpageButton.Focusable())(ctx); err == nil {
		testing.ContextLog(ctx, "Currently at cellular details page")
		if err := ui.LeftClick(ossettings.APNSubpageButton.Focusable())(ctx); err != nil {
			return errors.Wrap(err, "failed to go to from Network subpage to mobile data settings")
		}
	}
	return nil
}
