// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CustomDefaultAttachApnDisableEnableBehavior,
		Desc: "Tests the correct connect behavior for a custom APN that needs to be enabled as default and attach is created separately",
		Contacts: []string{
			"cros-device-enablement@google.com",
		},
		BugComponent:   "b:1131774", // ChromeOS > Software > Fundamentals > Device Enablement > Connectivity > Cellular
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:cellular", "cellular_sim_active", "cellular_e2e"},
		SoftwareDeps:   []string{"chrome"},
		Fixture:        "cellularEnforceConnectionAndResetShillProfile",
		Timeout:        9 * time.Minute,
	})
}

func CustomDefaultAttachApnDisableEnableBehavior(ctx context.Context, s *testing.State) {
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

	apnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	if apnName == "" {
		// Retrieve the APN name from known APN list if the name in shill profile is blank.
		knownAPNs, err := cellular.GetKnownApns(ctx)
		if err != nil {
			s.Fatal("Failed to get known APNs: ", err)
		}
		if len(knownAPNs) > 0 {
			apnName = knownAPNs[0].APNInfo[shillconst.DevicePropertyCellularAPNInfoApnName].(string)
		}
	}

	defaultApnConfig := &ossettings.ApnConfig{
		Name:               apnName,
		Username:           serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnUsername],
		Password:           serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnPassword],
		AuthenticationType: ossettings.GetUIStringForAuthenticationType(serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnAuthentication]),
		IPType:             ossettings.GetUIStringForIPType(serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnIPType]),
		ApnType:            ossettings.ApnIsDefault,
	}

	// Add default-only APN.
	if err := settings.CreateCustomAPN(ctx, defaultApnConfig); err != nil {
		s.Fatal("Failed to add default custom APN: ", err)
	}

	if err := settings.VerifyApnStabilized(defaultApnConfig, ossettings.ApnEnabled)(ctx); err != nil {
		s.Fatal("Failed to verify Default APN added successfully: ", err)
	}

	// Add attach-only APN.
	attachApnConfig := &ossettings.ApnConfig{
		Name:               apnName,
		Username:           serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnUsername],
		Password:           serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnPassword],
		AuthenticationType: ossettings.GetUIStringForAuthenticationType(serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnAuthentication]),
		IPType:             ossettings.GetUIStringForIPType(serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnIPType]),
		ApnType:            ossettings.ApnIsAttach,
	}
	if err := settings.CreateCustomAPN(ctx, attachApnConfig); err != nil {
		s.Fatal("Failed to add attach custom APN: ", err)
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("add and verify attach APN added",
		// Settings app should be at APN page at this point.
		settings.VerifyApnStabilized(attachApnConfig, ossettings.ApnEnabled),
		// Back to network details page to connect to the network.
		settings.DoDefault(ossettings.BackArrowBtn),
		ui.WithTimeout(ossettings.WaitForConnectionTimeout).RetryUntil(
			settings.MaybeConnectToApn(cr),
			settings.WaitUntilExists(ossettings.ConnectedStatus),
		),
		// Navigate to the APN page to verify that the APN page UI reports it's connected to the newly added APN correctly.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, attachApnConfig.Name, "" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to add custom APN and verify it shows in the APN list: ", err)
	}

	// Attempt to disable default APN that is currently enabled. Should not be possible.
	if err := uiauto.Combine("disable default APN",
		settings.ClickMoreActionButtonOfAnAPN(defaultApnConfig, ossettings.ApnEnabled),
		settings.DoDefault(ossettings.DisableBtn),
		verifyErrorToastMessageIsShowing(cr, settings),
		settings.VerifyApnConnected(cr, defaultApnConfig.Name, "" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to verify APN is connected after disabling default APN: ", err)
	}

	// Attempt to remove default APN that is currently enabled. Should not be possible.
	if err := uiauto.Combine("remove default APN",
		settings.ClickMoreActionButtonOfAnAPN(defaultApnConfig, ossettings.ApnEnabled),
		settings.DoDefault(ossettings.RemoveBtn),
		verifyErrorToastMessageIsShowing(cr, settings),
		settings.VerifyApnConnected(cr, defaultApnConfig.Name, "" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to verify APN is connected after removing default APN: ", err)
	}

	// Disable attach APN and verify disconnection.
	if err := uiauto.Combine("disable attach APN",
		settings.ClickMoreActionButtonOfAnAPN(attachApnConfig, ossettings.ApnEnabled),
		settings.DoDefault(ossettings.DisableBtn),
		settings.VerifyApnStabilized(attachApnConfig, ossettings.ApnDisabled),
		settings.VerifyApnNotConnected(cr, attachApnConfig.Name),
	)(ctx); err != nil {
		s.Fatal("Failed to verify APN no longer connected after disabling attach APN: ", err)
	}

	// Disable and Enable default APN after disabling attach APN.
	if err := uiauto.Combine("disable/enable default APN",
		settings.ClickMoreActionButtonOfAnAPN(defaultApnConfig, ossettings.ApnEnabled),
		settings.DoDefault(ossettings.DisableBtn),
		settings.VerifyApnStabilized(defaultApnConfig, ossettings.ApnDisabled),
		settings.ClickMoreActionButtonOfAnAPN(defaultApnConfig, ossettings.ApnDisabled),
		settings.DoDefault(ossettings.EnableBtn),
		settings.VerifyApnStabilized(defaultApnConfig, ossettings.ApnEnabled),
	)(ctx); err != nil {
		s.Fatal("Failed to verify APN is connected after disabling default APN: ", err)
	}

	// Enable currently disabled attach APN and verify connection.
	if err := uiauto.Combine("enable and verify attach APN",
		// Settings app should be at APN page at this point.
		settings.ClickMoreActionButtonOfAnAPN(attachApnConfig, ossettings.ApnDisabled),
		settings.DoDefault(ossettings.EnableBtn),
		settings.VerifyApnStabilized(attachApnConfig, ossettings.ApnEnabled),
		// Back to network details page to connect to the network.
		settings.DoDefault(ossettings.BackArrowBtn),
		ui.WithTimeout(ossettings.WaitForConnectionTimeout).RetryUntil(
			settings.MaybeConnectToApn(cr),
			settings.WaitUntilExists(ossettings.ConnectedStatus),
		),
		// Navigate to the APN page to verify that the APN page UI reports it's connected to the newly added APN correctly.
		settings.NavigateToApnPage(cr),
		settings.VerifyApnConnected(cr, attachApnConfig.Name, "" /* source */),
	)(ctx); err != nil {
		s.Fatal("Failed to enable attach APN and verify it is connected: ", err)
	}

	if uiauto.Combine("remove both custom APNs",
		// Delete currently enabled attach APN.
		settings.ClickMoreActionButtonOfAnAPN(attachApnConfig, ossettings.ApnEnabled),
		settings.DoDefault(ossettings.RemoveBtn),
		// Delete currently enabled default APN.
		settings.ClickMoreActionButtonOfAnAPN(defaultApnConfig, ossettings.ApnEnabled),
		settings.DoDefault(ossettings.RemoveBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to remove both APNs: ", err)
	}
}

// verifyErrorToastMessageIsShowing will verify that the "Can't disable or remove this APN..." toast is showing
func verifyErrorToastMessageIsShowing(cr *chrome.Chrome, settings *ossettings.OSSettings) uiauto.Action {
	return func(ctx context.Context) error {
		expr := `var node = shadowPiercingQuery(
			'cr-toast#errorToast span#errorToastMessage');
			if (node == undefined) {
				throw new Error("APN name not found");
			}
			node.innerText;
			`

		return testing.Poll(ctx, func(ctx context.Context) error {
			var errorMessage string
			if err := settings.EvalJSWithShadowPiercer(ctx, cr, expr, &errorMessage); err != nil {
				return errors.Wrap(err, "failed to find error message container")
			}

			if !strings.Contains(errorMessage, "Make sure enabled attach APNs are disabled or removed") {
				return testing.PollBreak(errors.Errorf("failed to show error toast; shows %q instead", errorMessage))
			}
			return nil
		}, &testing.PollOptions{
			Timeout:  10 * time.Second,
			Interval: time.Second,
		})
	}
}
