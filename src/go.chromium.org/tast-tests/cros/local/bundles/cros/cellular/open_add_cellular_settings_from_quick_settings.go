// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// addCellularURL is the URL of the Mobile data sub-page within the OS Settings with cellular setup dialog showing.
const addCellularURL = "chrome://os-settings/networks?type=Cellular&showCellularSetup=true"

func init() {
	testing.AddTest(&testing.Test{
		Func:           OpenAddCellularSettingsFromQuickSettings,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests the add cellular button in Quick Settings",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_sim_prod_esim", "cellular_e2e"},
		Fixture:      "cellular",
		Timeout:      9 * time.Minute,
	})
}

// OpenAddCellularSettingsFromQuickSettings tests that a user can successfully
// navigate to the Cellular sub-page with the Add Cellular dialog open within
// OS Settings from the Add Cellular button in the Network detailed view within
// Quick Settings.
func OpenAddCellularSettingsFromQuickSettings(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	defer cancel()

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the network section of Quick Settings: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.LeftClick(quicksettings.AddCellularButton)(ctx); err != nil {
		s.Fatal("Did not click Add cellular button: ", err)
	}

	// Check if the Add cellular sub-page within the OS Settings was opened.
	matcher := chrome.MatchTargetURL(addCellularURL)
	conn, err := cr.NewConnForTarget(ctx, matcher)
	if err != nil {
		s.Fatal("Failed to open the Add Cellular dialog: ", err)
	}

	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(ossettings.DialogEntryHeader)(ctx); err != nil {
		s.Fatal("Failed to find SM-DS discovery page: ", err)
	}
	defer conn.Close()
}
