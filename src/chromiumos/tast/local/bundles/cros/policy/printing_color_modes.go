// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/printpreview"
	"chromiumos/tast/local/chrome/uiauto/restriction"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/strcmp"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintingColorModes,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify behaviour of PrintingAllowedColorModes and PrintingColorDefault policies",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
			"project-bolton@google.com",
			"nedol@google.com", // Test author
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
			"informational",
		},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.Printers{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PrintingColorDefault{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PrintingAllowedColorModes{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Test PrintingAllowedColorModes and PrintingColorDefault policies (COM_FOUND_CUJ9_TASK3_WF1).
				Value: "screenplay-af2592d2-c335-4a0b-8330-a8f494423e58",
			},
		},
	})
}

func PrintingColorModes(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	printerName := "Water Cooler Printer"
	printersPolicy := &policy.Printers{Val: []string{
		fmt.Sprintf(`{
			"display_name": "%s",
			"description": "The printer next to the water cooler.",
			"manufacturer": "Printer Manufacturer",
			"model": "Color Laser 2004",
			"uri": "lpd://localhost:9100",
			"uuid": "1c395fdb-5d93-4904-b246-b2c046e79d12",
			"ppd_resource": {
				"effective_model": "generic pcl 6/pcl xl printer pxlcolor",
				"autoconf": false
			}
		}`, printerName)}}

	// Find a keyboard input source.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Error("Failed to get the keyboard: ", err)
	}
	defer kb.Close()

	for _, param := range []struct {
		name                    string
		expectedDefaultColor    string
		expectedAvailableColors []string
		policies                []policy.Policy
	}{
		{
			name:                    "default_color_unset_all_colors_allowed",
			expectedDefaultColor:    "Color",
			expectedAvailableColors: []string{"Color", "Black and white"},
			policies: []policy.Policy{
				printersPolicy,
				&policy.PrintingColorDefault{Stat: policy.StatusUnset},
				&policy.PrintingAllowedColorModes{Val: "any"},
			},
		},
		{
			name:                    "default_color_monochrome_allowed_colors_unset",
			expectedDefaultColor:    "Black and white",
			expectedAvailableColors: []string{"Color", "Black and white"},
			policies: []policy.Policy{
				printersPolicy,
				&policy.PrintingColorDefault{Val: "monochrome"},
				&policy.PrintingAllowedColorModes{Stat: policy.StatusUnset},
			},
		},
		{
			name:                    "default_color_color_all_colors_allowed",
			expectedDefaultColor:    "Color",
			expectedAvailableColors: []string{"Color", "Black and white"},
			policies: []policy.Policy{
				printersPolicy,
				&policy.PrintingColorDefault{Val: "color"},
				&policy.PrintingAllowedColorModes{Val: "any"},
			},
		},
		{
			name:                    "default_color_monochrome_is_allowed",
			expectedDefaultColor:    "Black and white",
			expectedAvailableColors: []string{"Black and white"},
			policies: []policy.Policy{
				printersPolicy,
				&policy.PrintingColorDefault{Val: "monochrome"},
				&policy.PrintingAllowedColorModes{Val: "monochrome"},
			},
		},
		{
			name:                    "default_color_monochrome_is_not_allowed",
			expectedDefaultColor:    "Color",
			expectedAvailableColors: []string{"Color"},
			policies: []policy.Policy{
				printersPolicy,
				&policy.PrintingColorDefault{Val: "monochrome"},
				&policy.PrintingAllowedColorModes{Val: "color"},
			},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Reserve ten seconds for cleanup.
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Error("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.policies); err != nil {
				s.Error("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)
			// The UI tree must be dumped before closing the browser.
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open a new tab. The print dialog fails to open when invoking CTRL+P
			// directly after calling `browserfixt.SetUp`, likely because the page
			// isn't fully loaded yet. It also fails to open on about:blank pages, but
			// works fine on chrome://newtab; see crbug.com/1290797.
			conn, err := br.NewConn(ctx, chrome.NewTabURL)
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}
			defer conn.Close()

			// Connect to Test API to use it with the UI library.
			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Error("Failed to create Test API connection: ", err)
			}

			ui := uiauto.New(tconn)
			if err := uiauto.Combine("open Print Preview with a shortcut",
				kb.AccelAction("Ctrl+P"),
				printpreview.WaitForPrintPreview(tconn))(ctx); err != nil {
				s.Error("Failed to open the Print Preview: ", err)
			}

			if err := printpreview.SelectPrinter(ctx, tconn, printerName); err != nil {
				s.Error("Failed to select a printer: ", err)
			}

			colorSelection := nodewith.Role(role.ComboBoxSelect).Name("Color")
			if err := ui.WaitUntilExists(colorSelection)(ctx); err != nil {
				s.Error("Failed to find the color selection box: ", err)
			}

			nodeInfo, err := ui.Info(ctx, colorSelection)
			if err != nil {
				s.Error("Failed to check the state of 'Color' selection: ", err)
			}

			if param.expectedDefaultColor != nodeInfo.Value {
				s.Errorf("Unexpected default value of the 'Color' selection: got %s; want %s", nodeInfo.Value, param.expectedDefaultColor)
			}

			availableColors := make([]string, 0)

			if nodeInfo.Restriction == restriction.Disabled {
				// Color selection box is disabled, thus the only mode available is the one selected.
				availableColors = append(availableColors, nodeInfo.Value)
			} else if nodeInfo.Restriction == restriction.None {
				// Click on the color selection box and wait for it to expand.
				if err := uiauto.Combine("open color selection preview",
					ui.DoDefault(colorSelection),
					ui.WaitUntilExists(colorSelection.State("expanded", true)))(ctx); err != nil {
					s.Error("Failed to expand color selection: ", err)
				}

				// Search for all available color mode nodes.
				availableColorModeNodes, err := ui.NodesInfo(ctx, nodewith.Ancestor(colorSelection).Role(role.ListBoxOption))
				if err != nil {
					s.Error("Failed to fetch available color modes: ", err)
				}
				for _, availableColorModeNode := range availableColorModeNodes {
					availableColors = append(availableColors, availableColorModeNode.Name)
				}
			} else {
				s.Error("Unknown restriction value")
			}

			// Compare actual and expected sets of available color modes.
			if diff := strcmp.SameList(param.expectedAvailableColors, availableColors); diff != "" {
				s.Error("Unexpected available color modes (-want +got) ", diff)
			}

			// We should manually close the print preview, as the "chrome://print" page
			// doesn't exist, but it's stored in the context for some reason
			defer func() {
				if err := uiauto.Combine("close print preview",
					ui.DoDefault(nodewith.Role(role.Button).NameStartingWith("Cancel")))(ctx); err != nil {
					s.Error("Failed to close print preview: ", err)
				}
			}()
		})
	}
}
