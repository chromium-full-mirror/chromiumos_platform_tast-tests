// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/restrictionlevel"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type printingTestParams struct {
	name                    string
	path                    string
	restriction             restrictionlevel.RestrictionLevel
	browserType             browser.Type
	waitTimeSecNotification time.Duration
	waitTimeSecWarning      time.Duration
}

const (
	dlpPrintingBlockedPath        = "/blocked"
	dlpPrintingAllowedPath        = "/allowed"
	dlpPrintingWarnPath           = "/warn"
	waitTimeSecNotificationAsh    = 10
	waitTimeSecNotificationLacros = 15
	waitTimeSecWarningAsh         = 4
	waitTimeSecWarningLacros      = 5
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListPrinting,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with printing restrictions",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"ayaelattar@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Block users from sharing confidential information (printing): COM_DATPROT_CUJ3_TASK1_WF1.
				Value: "screenplay-d55a401f-c039-49bb-8df5-bf438e151525",
			}, {
				Key: "feature_id",
				// Block users from sharing confidential information within company (printing): COM_DATPROT_CUJ4_TASK1_WF1.
				Value: "screenplay-c44de927-e1a6-4a0d-b788-3944a95a30c2",
			}, {
				Key: "feature_id",
				// Warn users from sharing confidential information (printing): COM_DATPROT_CUJ3_TASK2_WF1.
				Value: "screenplay-0bf7ad3f-d1e9-436a-87b7-b6cd95880727",
			}, {
				Key: "feature_id",
				// Warn users from sharing confidential information within company (printing): COM_DATPROT_CUJ4_TASK2_WF1.
				Value: "screenplay-17b0e03a-7ba7-4c66-a81e-2bfcc470a5a4",
			}},
		Params: []testing.Param{{
			Name:      "ash_blocked",
			ExtraAttr: []string{"group:mainline"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: printingTestParams{
				name:                    "blocked",
				path:                    dlpPrintingBlockedPath,
				restriction:             restrictionlevel.Blocked,
				browserType:             browser.TypeAsh,
				waitTimeSecNotification: waitTimeSecNotificationAsh,
				waitTimeSecWarning:      waitTimeSecWarningAsh,
			},
		}, {
			Name:      "ash_warn_proceeded",
			ExtraAttr: []string{"group:golden_tier"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: printingTestParams{
				name:                    "warn_proceded",
				path:                    dlpPrintingWarnPath,
				restriction:             restrictionlevel.WarnProceeded,
				browserType:             browser.TypeAsh,
				waitTimeSecNotification: waitTimeSecNotificationAsh,
				waitTimeSecWarning:      waitTimeSecWarningAsh,
			},
		}, {
			Name:      "ash_warn_cancelled",
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val: printingTestParams{
				name:                    "warn_cancelled",
				path:                    dlpPrintingWarnPath,
				restriction:             restrictionlevel.WarnCancelled,
				browserType:             browser.TypeAsh,
				waitTimeSecNotification: waitTimeSecNotificationAsh,
				waitTimeSecWarning:      waitTimeSecWarningAsh,
			},
		}, {
			Name:              "lacros_blocked",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: printingTestParams{
				name:                    "blocked",
				path:                    dlpPrintingBlockedPath,
				restriction:             restrictionlevel.Blocked,
				browserType:             browser.TypeLacros,
				waitTimeSecNotification: waitTimeSecNotificationLacros,
				waitTimeSecWarning:      waitTimeSecWarningLacros,
			},
		}, {
			Name:              "lacros_warn_proceeded",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: printingTestParams{
				name:                    "warn_proceeded",
				path:                    dlpPrintingWarnPath,
				restriction:             restrictionlevel.WarnProceeded,
				browserType:             browser.TypeLacros,
				waitTimeSecNotification: waitTimeSecNotificationLacros,
				waitTimeSecWarning:      waitTimeSecWarningLacros,
			},
		}, {
			Name:              "lacros_warn_cancelled",
			ExtraAttr:         []string{"group:golden_tier"},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val: printingTestParams{
				name:                    "warn_cancelled",
				path:                    dlpPrintingWarnPath,
				restriction:             restrictionlevel.WarnCancelled,
				browserType:             browser.TypeLacros,
				waitTimeSecNotification: waitTimeSecNotificationLacros,
				waitTimeSecWarning:      waitTimeSecWarningLacros,
			},
		}},
	})
}

// printingBlockPolicy returns policy that blocks printing based on given URL.
func printingBlockPolicy(url string) []policy.Policy {
	return []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Disable printing of confidential content",
				Description: "User should not be able to print confidential content",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						url,
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "PRINTING",
						Level: "BLOCK",
					},
				},
			},
		},
	},
	}
}

// printingWarnPolicy returns policy that warns in case of printing based on given URL.
func printingWarnPolicy(url string) []policy.Policy {
	return []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Warn before printing confidential content",
				Description: "User should be warned before printing confidential content",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						url,
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "PRINTING",
						Level: "WARN",
					},
				},
			},
		},
	},
	}
}

func DataLeakPreventionRulesListPrinting(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Setup test HTTP server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello DLP client you navigated to ", r.URL.Path)
	}))
	defer server.Close()

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// Construct URL to pass to test and policy.
	testURL, err := url.Parse(server.URL)
	if err != nil {
		s.Fatal("Failed to parse url: ", err)
	}
	testURL.Path = path.Join(testURL.Path, s.Param().(printingTestParams).path)

	// Update the policy blob.
	pb := policy.NewBlob()
	if s.Param().(printingTestParams).restriction == restrictionlevel.Blocked {
		pb.AddPolicies(printingBlockPolicy(testURL.String()))
	} else if s.Param().(printingTestParams).restriction == restrictionlevel.WarnCancelled || s.Param().(printingTestParams).restriction == restrictionlevel.WarnProceeded {
		pb.AddPolicies(printingWarnPolicy(testURL.String()))
	}

	// Update policy.
	if err := policyutil.ServeBlobAndRefresh(ctx, fakeDMS, cr, pb); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(printingTestParams).browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer func(ctx context.Context) {
		if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
			// The Lacros browser is not closed in other places in the test.
			s.Error("The Lacros browser probably crashed: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+s.Param().(printingTestParams).name)

	conn, err := br.NewConn(ctx, testURL.String())
	if err != nil {
		s.Fatal("Failed to open page: ", err)
	}
	defer conn.Close()

	// Type the shortcut.
	if err := keyboard.Accel(ctx, "Ctrl+P"); err != nil {
		s.Fatal("Failed to type printing hotkey: ", err)
	}
	// Waiting for UI to be stable so that we can be sure we find UI components if necessary.
	if err := webutil.WaitForQuiescence(ctx, conn, s.Param().(printingTestParams).waitTimeSecWarning*time.Second); err != nil {
		s.Fatalf("Failed to wait for %q to achieve quiescence: %v", testURL, err)
	}

	// Make a call to print page.
	if err := testPrinting(ctx, tconn, keyboard, s.Param().(printingTestParams).restriction, s.Param().(printingTestParams).waitTimeSecWarning*time.Second); err != nil {
		s.Fatal("Failed to run test body: ", err)
	}

	// Confirm that the notification only appeared if expected.
	if s.Param().(printingTestParams).restriction == restrictionlevel.Blocked {
		if _, err := ash.WaitForNotification(ctx, tconn, s.Param().(printingTestParams).waitTimeSecNotification*time.Second, ash.WaitIDContains("print_dlp_blocked"), ash.WaitTitle("Printing is blocked")); err != nil {
			s.Error("Failed to wait for notification with title 'Printing is blocked': ", err)
		}
	}
}

// testPrinting tests whether printing is possible via hotkey (Ctrl + P).
func testPrinting(ctx context.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, restriction restrictionlevel.RestrictionLevel, waitTime time.Duration) error {
	if restriction == restrictionlevel.WarnProceeded {
		// Hit Enter, which is equivalent to clicking on the "Print anyway" button.
		if err := keyboard.Accel(ctx, "Enter"); err != nil {
			return errors.Wrap(err, "failed to hit Enter")
		}
	}

	if restriction == restrictionlevel.WarnCancelled {
		// Hit Esc, which is equivalent to clicking on the "Cancel" button.
		if err := keyboard.Accel(ctx, "Esc"); err != nil {
			return errors.Wrap(err, "failed to hit Esc")
		}
	}

	// Check that the printing dialog appears if and only if printing the page is allowed.
	ui := uiauto.New(tconn)

	if restriction == restrictionlevel.Allowed || restriction == restrictionlevel.WarnProceeded {
		if err := ui.WithTimeout(waitTime).WaitUntilExists(printpreview.PrintPreviewNode)(ctx); err != nil {
			return errors.Wrap(err, "failed to find the printing window")
		}
	} else {
		if err := ui.EnsureGoneFor(printpreview.PrintPreviewNode, waitTime)(ctx); err != nil {
			return errors.Wrap(err, "should not show the printing window")
		}
	}

	return nil
}
