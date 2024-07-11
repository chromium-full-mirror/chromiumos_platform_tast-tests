// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListPrivacyScreen,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with privacy screen blocked restriction",
		Contacts: []string{
			"chromeos-dlp@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.PrivacyScreen()),
		Params: []testing.Param{{
			Name:      "ash_blocked",
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
			Fixture:   fixture.ChromePolicyLoggedIn,
			Val:       browser.TypeAsh,
		}, {
			Name: "lacros_blocked",
			ExtraAttr: []string{
				"group:golden_tier",
				"group:medium_low_tier",
				"group:hardware",
				"group:complementary",
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func DataLeakPreventionRulesListPrivacyScreen(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello DLP client you navigated to ", r.URL.Path)
	}))
	defer server.Close()

	// DLP policy with privacy screen blocked restriction.
	policyDLP := []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Enable privacy screen for confidential content in restricted source",
				Description: "Privacy screen should be enabled when on restricted site",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						server.URL + "/blocked",
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "PRIVACY_SCREEN",
						Level: "BLOCK",
					},
				},
			},
		},
	},
	}

	// Update the policy blob.
	pb := policy.NewBlob()
	pb.AddPolicies(policyDLP)

	// Update policy.
	if err := policyutil.ServeBlobAndRefresh(ctx, fakeDMS, cr, pb); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer func(ctx context.Context) {
		if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
			// The Lacros browser is not closed in other places in the test.
			s.Error("The Lacros browser probably crashed: ", err)
		}
	}(cleanupCtx)

	ui := uiauto.New(tconn)

	conn, err := br.NewConn(ctx, server.URL+"/blocked")
	if err != nil {
		s.Fatal("Failed to open page: ", err)
	}
	defer conn.Close()

	if err := checkPrivacyScreenOnBubble(ctx, ui); err != nil {
		s.Error("Couldn't check for notification: ", err)
	}

	value, err := privacyScreenValue(ctx)
	if err != nil {
		s.Fatal("Couldn't check value for privacy screen prop: ", err)
	}

	if !value {
		s.Errorf("Privacy screen prop value: got %v; want true", value)
	}

	// Verify that by opening the allowed page, the privacy screen will get disabled.
	destURL := server.URL + "/allowed"
	if _, err := br.NewConn(ctx, destURL); err != nil {
		s.Error("Failed to open page: ", err)
	}

	// GoBigSleepLint - Wait for privacy screen to be disabled.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	value, err = privacyScreenValue(ctx)
	// Privacy screen should be disabled.
	if value {
		s.Errorf("Privacy screen prop value: got %v; want false", value)
	}
}

func checkPrivacyScreenOnBubble(ctx context.Context, ui *uiauto.Context) error {
	// Message name - IDS_ASH_STATUS_TRAY_PRIVACY_SCREEN_TOAST_ACCESSIBILITY_TEXT
	bubbleMessage := nodewith.NameContaining("Privacy screen is on. Enforced by your administrator").First()

	if err := ui.WaitUntilExists(bubbleMessage)(ctx); err != nil {
		return errors.Wrap(err, "failed to check for privacy screen on bubble")
	}

	return nil
}

// privacyScreenValue retrieves value of privacy screen prop.
func privacyScreenValue(ctx context.Context) (bool, error) {
	// modetest -c get list of connectors
	output, err := testexec.CommandContext(ctx, "modetest", "-c").Output()
	if err != nil {
		return false, err
	}

	// Get privacy-screen connector.
	outputSlice := strings.Split(string(output), "privacy-screen:")

	if len(outputSlice) <= 1 {
		return false, errors.New("failed to find privacy screen prop")
	}

	for _, line := range strings.Split(outputSlice[1], "\n") {
		// Check for prop value.
		matches := strings.Contains(line, "value:")
		if matches {
			if found := strings.Contains(line, "1"); found {
				return true, nil
			}

			if found := strings.Contains(line, "0"); found {
				return false, nil
			}

			// Need to check for prop value only once.
			return false, errors.New("failed to find value for privacy screen prop")
		}
	}

	return false, nil
}
