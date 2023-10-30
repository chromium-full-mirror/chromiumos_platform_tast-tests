// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testParams struct {
	boxNames    []string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromePolicyPageStatusTimestamps,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests timestamps in status boxes on chrome://policy page",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"sergiyb@google.com", // Test author
			"chromeos-commercial-remote-management@google.com",
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			// TODO(b/308448107): Test unmanaged user on enrolled device in Ash and Lacros.
			// TODO(b/308448107): Test managed/unmanaged secondary profile in enrolled/unenrolled Lacros.
			{
				// User is managed, but device is not.
				Name:      "ash_managed",
				Fixture:   fixture.ChromePolicyLoggedIn,
				ExtraAttr: []string{"group:golden_tier"},
				Val: testParams{
					boxNames:    []string{"User policies"},
					browserType: browser.TypeAsh,
				},
			},
			{
				// Both user and device are managed.
				Name:      "ash_enrolled",
				Fixture:   fixture.ChromeEnrolledLoggedIn,
				ExtraAttr: []string{"group:golden_tier"},
				Val: testParams{
					boxNames:    []string{"User policies", "Device policies"},
					browserType: browser.TypeAsh,
				},
			},
			{
				// User is managed, but device is not.
				Name:              "lacros_managed",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosPolicyLoggedIn,
				// TODO(b/307688738): Enable test.
				// ExtraAttr: []string{"group:golden_tier"},
				Val: testParams{
					boxNames:    []string{"User policies"},
					browserType: browser.TypeLacros,
				},
			},
			{
				// Both user and device are managed.
				Name:              "lacros_enrolled",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosEnrolledLoggedIn,
				ExtraAttr:         []string{"group:golden_tier"},
				Val: testParams{
					boxNames: []string{
						"User policies",
						// TODO(b/308417012): Enable testing device policies.
						// "Device policies",
					},
					browserType: browser.TypeLacros,
				},
			},
		},
	})
}

// reloadPolicies clicks the "Reload policies" button on the chrome://policy
// page to reload policies. Although we could use `policyutil.Refresh`, we
// prefer clicking the button to ensure that it works as expected.
func reloadPolicies(ctx context.Context, conn *browser.Conn, s *testing.State, isLacros bool) {
	if err := conn.Eval(ctx, `document.getElementById('reload-policies').click()`, nil); err != nil {
		s.Fatal("Failed to click Reload policies button: ", err)
	}

	// Wait until reload button becomes enabled again, i.e. policies reloaded.
	if err := conn.WaitForExpr(ctx, `!document.getElementById('reload-policies').disabled`); err != nil {
		s.Fatal("Failed while waiting for Reload policies button to become enabled again: ", err)
	}

	if isLacros {
		// GoBigSleepLint: TODO(crbug/1326565): Wait for policies to be reloaded.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed while waiting for policies to be reloaded: ", err)
		}
		if err := conn.Navigate(ctx, "chrome://policy"); err != nil {
			s.Fatal("Failed to refresh chrome://policy after reloading policies: ", err)
		}
	}
}

// Content from status boxes on chrome://policy keyed by the box name, e.g.
// "User policies". Each value is also a map from a field class name, e.g.
// "time-since-last-fetch-attempt", to a field value, e.g. "0 secs ago".
type statusBoxesMap map[string]map[string]string

func readStatusBoxes(ctx context.Context, conn *browser.Conn, s *testing.State) statusBoxesMap {
	if err := conn.WaitForExpr(ctx, `!document.getElementById('status-section').hidden`); err != nil {
		s.Fatal("Failed while waiting for status box to appear: ", err)
	}

	var boxes statusBoxesMap
	if err := conn.Eval(ctx, `(async() => {
		const statusSection = document.getElementById('status-section');
		const policies = getPolicyFieldsets();
		const statuses = {};
		for (let i = 0; i < policies.length; ++i) {
			const statusHeading = policies[i]
				.querySelector('.status-box-heading').textContent;
			const entries = {};
			const rows = policies[i]
				.querySelectorAll('.status-entry div:nth-child(2)');
			for (let j = 0; j < rows.length; ++j) {
				entries[rows[j].className.split(' ')[0]] = rows[j].textContent.trim();
			}
			statuses[statusHeading.trim()] = entries;
		}

		return statuses;
	})()`, &boxes); err != nil {
		s.Fatal("Failed to read status boxes: ", err)
	}
	return boxes
}

func checkTime(boxes statusBoxesMap, boxNames []string, key string, matcher *regexp.Regexp, s *testing.State) {
	for _, boxName := range boxNames {
		status, ok := boxes[boxName]
		if !ok {
			s.Errorf("No status box named %s", boxName)
			return
		}

		if didMatch := matcher.Match([]byte(status[key])); !didMatch {
			s.Errorf("%s is invalid: %s (does not match `%s`)", key, status[key], matcher)
		}
	}
}

func ChromePolicyPageStatusTimestamps(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	params := s.Param().(testParams)

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Setup browser based on the chrome type.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, params.browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Run actual test. Start by opening chrome://policy page.
	conn, err := br.NewConn(ctx, "chrome://policy")
	if err != nil {
		s.Fatal("Failed to load chrome://policy: ", err)
	}
	defer conn.Close()

	// This time regexp allows for up to 30 seconds more time to pass than
	// its name suggests. This helps reduce flakiness on slow devices.
	var zeroSecsAgoRE = regexp.MustCompile(`([0-9]|1[0-9]|2[0-9]|30) secs? ago`)
	var oneMinAgoRE = regexp.MustCompile(`1 min ago`)

	// Reload policies and immediately check that timestamps are at 0 secs ago.
	reloadPolicies(ctx, conn, s, params.browserType == browser.TypeLacros)
	newBoxes := readStatusBoxes(ctx, conn, s)
	checkTime(newBoxes, params.boxNames, "time-since-last-refresh", zeroSecsAgoRE, s)
	checkTime(newBoxes, params.boxNames, "time-since-last-fetch-attempt", zeroSecsAgoRE, s)

	// GoBigSleepLint: Sleep for 1 minute, refresh page, check that timestamps are updated.
	if err = testing.Sleep(ctx, time.Minute); err != nil {
		s.Fatal("Failed to sleep for 1 minute: ", err)
	}
	if err = conn.Navigate(ctx, "chrome://policy"); err != nil {
		s.Fatal("Failed to reload chrome://policy: ", err)
	}
	sleepyBoxes := readStatusBoxes(ctx, conn, s)
	checkTime(sleepyBoxes, params.boxNames, "time-since-last-refresh", oneMinAgoRE, s)
	checkTime(sleepyBoxes, params.boxNames, "time-since-last-fetch-attempt", oneMinAgoRE, s)

	// Simulate 500 error on the server while reloading policies, check that fetch
	// timestamp is at 0 secs ago whilst policy timestamp is still at 1 min ago.
	pb := policy.NewBlob()
	pb.RequestErrors["policy"] = 500
	if err = policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to simulate error 500 on the policy server: ", err)
	}
	reloadPolicies(ctx, conn, s, params.browserType == browser.TypeLacros)
	mixedBoxes := readStatusBoxes(ctx, conn, s)
	checkTime(mixedBoxes, params.boxNames, "time-since-last-refresh", oneMinAgoRE, s)
	checkTime(mixedBoxes, params.boxNames, "time-since-last-fetch-attempt", zeroSecsAgoRE, s)
}
