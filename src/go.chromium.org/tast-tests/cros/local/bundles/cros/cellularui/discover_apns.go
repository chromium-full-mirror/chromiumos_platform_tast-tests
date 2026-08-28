// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
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
		Func: DiscoverApns,
		Desc: "Tests the correct connect behavior for adding known APNs",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_carrier_agnostic", "cellular_e2e"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularEnforceConnectionAndResetShillProfile",
		Timeout:      9 * time.Minute,
	})
}

func DiscoverApns(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// In case roaming is required for the SIM on the device.
	cleanup, err := cellular.SetRoamingPolicy(ctx, true, true)
	if err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}
	defer cleanup(cleanupCtx)

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
		s.Fatal("Error getting known APNs: ", err)
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
		s.Fatal("Failed to go to apn subpage: ", err)
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("select and verify the first APN",
		// Settings app should be at APN page at this point.
		settings.OpenDiscoverAPNDialogFromAPNSubpage(),
		settings.SelectAPNFromDialog(firstAPNName),
		// Back to network details page to connect to the network.
		settings.DoDefault(ossettings.BackArrowBtn),
		ui.WithTimeout(ossettings.WaitForConnectionTimeout).RetryUntil(
			settings.MaybeConnectToApn(cr),
			settings.WaitUntilExists(ossettings.ConnectedStatus),
		),
		// Navigate to the APN page to verify that the APN page UI reports it's connected to the newly added APN correctly.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, firstAPNName, "ui"),
		verifyOnlyThisAPNEnabled(settings, firstAPNName),
	)(ctx); err != nil {
		s.Fatal("Failed to verify the first APN is the only APN: ", err)
	}

	if err := settings.OpenDiscoverAPNDialogFromAPNSubpage()(ctx); err != nil {
		s.Fatal("Failed to open discover APN dialog: ", err)
	}

	secondAPNName := firstAPNName
	for _, knownAPN := range knownAPNs {
		apnName := fmt.Sprintf("%v", knownAPN.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnName])
		// Prefer selecting an APN that is different from the one that was added the first time
		if apnName == firstAPNName {
			continue
		}
		apnSelection := nodewith.NameContaining(apnName).Role(role.StaticText)
		if err := settings.WaitUntilExists(apnSelection)(ctx); err != nil {
			continue
		}
		secondAPNName = apnName
		break
	}

	if err := uiauto.Combine("select and verify the second APN",
		settings.SelectAPNFromDialog(secondAPNName),
		verifyOnlyThisAPNEnabled(settings, secondAPNName),
	)(ctx); err != nil {
		s.Fatal("Failed to verify the second APN is the only APN: ", err)
	}
}

// verifyOnlyThisAPNEnabled verifies that only the specified |apn| is enabled.
func verifyOnlyThisAPNEnabled(settings *ossettings.OSSettings, apn string) uiauto.Action {
	return func(ctx context.Context) error {
		moreActionsButtonOfAPNFinder := nodewith.NameRegex(regexp.MustCompile("APN is (connected|enabled)")).Role(role.Button).HasClass("icon-more-vert")
		// Ensure the page is loaded.
		if err := settings.WaitUntilExists(moreActionsButtonOfAPNFinder.First())(ctx); err != nil {
			return errors.Wrap(err, "failed to wait until node exits")
		}
		moreOptionsButtons, err := settings.NodesInfo(ctx, moreActionsButtonOfAPNFinder)
		if err != nil {
			return errors.Wrap(err, "failed to find more options button")
		}

		if len(moreOptionsButtons) == 0 {
			return errors.Wrap(err, "failed to find an enabled or connected APN")
		}
		if len(moreOptionsButtons) > 1 {
			return errors.Wrap(err, "failed to find exactly one enabled or connected APN")
		}

		// Check if the only enabled APN is the expected APN.
		// We force both strings to be lower-cased since the casing may be different even though the name is the same.
		if !strings.Contains(strings.ToLower(moreOptionsButtons[0].Name), strings.ToLower(apn)) {
			return errors.Errorf("unexpected enabled APN, got: %q; want: %q", moreOptionsButtons[0].Name, apn)
		}

		return nil
	}
}
