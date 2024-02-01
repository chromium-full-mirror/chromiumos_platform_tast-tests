// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           MigrateDefaultCustomApn,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests the correctness of the UI for a valid custom APN that is migrated to the new UI",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_amari_callbox"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularResetShillProfileOnPostTest",
		Timeout:      10 * time.Minute,
	})
}

func MigrateDefaultCustomApn(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper
	if err := helper.ClearCustomAPNList(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to clear cellular.CustomAPNList: ", err)
	}
	if errs := helper.ResetShill(ctx); errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}
	knownAPNs, err := cellular.GetKnownApns(ctx)
	if err != nil {
		s.Fatal("Error getting known APNs: ", knownAPNs)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		if err := helper.ClearCustomAPNList(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clear cellular.CustomAPNList: ", err)
		}
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	apn := knownAPNs[0]
	validAPNToMigrate := fmt.Sprintf("%v", apn.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnName])
	testing.ContextLog(ctx, "Custom APN to migrate: ", validAPNToMigrate)

	func() {
		cr, err := chrome.New(ctx, chrome.DisableFeatures("ApnRevamp"))
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}

		defer cr.Close(ctx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}

		if _, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr); err != nil {
			s.Fatal("Failed to open mobile data subpage: ", err)
		}

		if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
			s.Fatal("Failed to go to active cellular network detail page view: ", err)
		}

		if err := ossettings.SelectPreRevampOtherAPN(ctx, tconn, "Other"); err != nil {
			s.Fatal("Failed to select custom APN: ", err)
		}

		isAttach := false
		username := ""
		password := ""

		for _, apnType := range apn.APNTypes {
			if apnType == shillconst.DevicePropertyCellularAPNInfoApnTypeIA {
				isAttach = true
			}
		}

		if savedUsername := apn.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnUsername]; savedUsername != nil {
			username = fmt.Sprintf("%v", savedUsername)
		}

		if savedPassword := apn.APNInfo[shillconst.DevicePropertyCellularAPNInfoApnPassword]; savedPassword != nil {
			password = fmt.Sprintf("%v", savedPassword)
		}

		if err := ossettings.EnterPreRevampOtherAPNDetails(ctx, tconn, validAPNToMigrate, username, password, isAttach); err != nil {
			s.Fatal("Failed to enter custom APN: ", err)
		}

		ui := uiauto.New(tconn)

		if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err != nil {
			s.Fatal("Failed to verify Connected: ", err)
		}

		if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.DisconnectButton.Focusable())(ctx); err != nil {
			s.Fatal("Failed to verify Disconnect button appears after APN details changed: ", err)
		}

		// Disconnect from the network and attempt a connect to ensure the custom APN is used.
		if err := ui.EnsureExistsFor(ossettings.DisconnectButton.Focusable(), 5*time.Second)(ctx); err != nil {
			if err := uiauto.Combine("Disconnect and re-connect",
				ui.ScrollToVisible(ossettings.DisconnectButton.Focusable()),
				ui.LeftClickUntil(ossettings.DisconnectButton.Focusable(),
					ui.EnsureGoneFor(ossettings.DisconnectButton.Focusable(), 5*time.Second)),
				ui.WithTimeout(10*time.Second).WaitUntilExists(ossettings.ConnectButton.Focusable()),
				ui.EnsureExistsFor(ossettings.ConnectButton.Focusable(), 5*time.Second),
				ui.LeftClick(ossettings.ConnectButton),
				ui.WithTimeout(10*time.Second).WaitUntilExists(ossettings.ConnectedStatus),
				ui.EnsureExistsFor(ossettings.DisconnectButton.Focusable(), 5*time.Second),
			)(ctx); err != nil {
				s.Fatal("Failed to disconnect and re-connect: ", err)
			}
		}
	}()

	cr, err := chrome.New(ctx,
		chrome.EnableFeatures("ApnRevamp"),
		chrome.RemoveNotification(false),
		chrome.KeepState())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	defer mdp.Close(ctx)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn, false); err != nil {
		s.Fatal("Failed to go to APN subpage: ", err)
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, validAPNToMigrate, ""); err != nil {
		s.Fatal("Failed to verify connected APN UI: ", err)
	}
}
