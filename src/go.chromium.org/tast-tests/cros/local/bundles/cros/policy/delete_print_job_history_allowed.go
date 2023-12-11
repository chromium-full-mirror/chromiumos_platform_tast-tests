// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printmanagementapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeletePrintJobHistoryAllowed,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Behavior of DeletePrintJobHistoryAllowed policy, checking the corresponding button state after setting the policy",
		Contacts: []string{
			"cros-peripherals@google.com",
			"gavinwill@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Printing
		BugComponent: "b:1131981",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Fixture: fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeletePrintJobHistoryAllowed{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.Printers{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// DeletePrintJobHistoryAllowed tests the DeletePrintJobHistoryAllowed policy.
func DeletePrintJobHistoryAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	uia := uiauto.New(tconn)

	// Get clipboard to print.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(ctx)

	for _, param := range []struct {
		name                string
		expectedRestriction restriction.Restriction              // specifies whether the button to clear history should be restricted.
		value               *policy.DeletePrintJobHistoryAllowed // value is the value of the policy.
	}{
		{
			name:                "deny",
			expectedRestriction: restriction.Disabled,
			value:               &policy.DeletePrintJobHistoryAllowed{Val: false},
		},
		{
			name:                "allow",
			expectedRestriction: restriction.None,
			value:               &policy.DeletePrintJobHistoryAllowed{Val: true},
		},
		{
			name:                "unset",
			expectedRestriction: restriction.None,
			value:               &policy.DeletePrintJobHistoryAllowed{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Configure printer policy.
			printerName := "Water Cooler Printer"
			printersPolicy := &policy.Printers{Val: []string{
				fmt.Sprintf(`{
			  "display_name": "%s",
			  "description": "The printer next to the water cooler.",
			  "manufacturer": "Printer Manufacturer",
			  "model": "Color Laser 2004",
			  "uri": "lpd://localhost:9101",
			  "uuid": "1c395fdb-5d93-4904-b246-b2c046e79d12",
			  "ppd_resource": {
    			"effective_model": "generic pcl 6/pcl xl printer pxlcolor",
		  	"autoconf": false
			  }
		  }`, printerName)}}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value, printersPolicy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Launch Print Management app.
			_, err := printmanagementapp.Launch(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to launch Print Management app: ", err)
			}
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			if err := uiauto.Combine("open Print Preview with a shortcut",
				kb.AccelAction("Ctrl+P"),
				printpreview.WaitForPrintPreview(tconn))(ctx); err != nil {
				s.Fatal("Failed to open the Print Preview: ", err)
			}

			if err := printpreview.SelectPrinter(ctx, tconn, printerName); err != nil {
				s.Fatal("Failed to select printer: ", err)
			}

			if err := printpreview.Print(ctx, tconn); err != nil {
				s.Fatal("Failed to print: ", err)
			}

			// Cancel the print job.
			if err := uiauto.Combine("Cancel the print job",
				uia.FocusAndWait(nodewith.Role(role.Button).Ancestor(nodewith.NameContaining("Press enter to cancel the print job").First())),
				kb.AccelAction("Enter"),
			)(ctx); err != nil {
				s.Fatal("Failed to cancel the print job: ", err)
			}

			if err := uia.CheckRestriction(printmanagementapp.PrintManagementDeleteHistoryButton, param.expectedRestriction)(ctx); err != nil {
				s.Fatal("Failed to check that the clear history button is in an expected restriction state: ", err)
			}
		})
	}
}
