// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/policyutil"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type annotationTestParams struct {
	name                  string          // name is the subtest name.
	browserType           browser.Type    // browser type used in the subtest.
	policies              []policy.Policy // policies to check
	annotationLogExpected bool
}

const testURL = "https://www.google.com"

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrafficAnnotationURLKeyedDataCollection,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "This test checks the network annotataion for UKM policy to make sure we are not sending network traffic when it's off",
		Contacts: []string{
			"chrome-ess-engprod@google.com",
			"meyron@google.com",
			"rzakarian@google.com",
		},
		BugComponent: "b:1152652", // Chrome Operations > BrApp EngProd > ESS > Enterprise Infra
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      8 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.ChromeEnrolledLoggedInShortMetricsInterval,
				Val:     browser.TypeAsh,
			},
		},
		Data: []string{"autofill_address_enabled.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UrlKeyedAnonymizedDataCollectionEnabled{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.EnableSyncConsent{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SyncDisabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func TrafficAnnotationURLKeyedDataCollection(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve 10 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	const ukmNetworkAnnotationID = "727478"
	for _, param := range []annotationTestParams{
		{
			name:                  "ukm_true_msbb_true",
			annotationLogExpected: true,
			policies: []policy.Policy{
				&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: true},
				&policy.SyncDisabled{Val: false},
				&policy.EnableSyncConsent{Val: true},
			},
		},
		{
			name:                  "ukm_false_msbb_false",
			annotationLogExpected: false,
			policies: []policy.Policy{
				&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: false},
				&policy.SyncDisabled{Val: true},
				&policy.EnableSyncConsent{Val: false},
			},
		},
		{
			name:                  "ukm_false_msbb_true",
			annotationLogExpected: false,
			policies: []policy.Policy{
				&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: false},
				&policy.SyncDisabled{Val: false},
				&policy.EnableSyncConsent{Val: true},
			},
		},
		{
			name:                  "ukm_true_msbb_false",
			annotationLogExpected: true,
			policies: []policy.Policy{
				&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: true},
				&policy.SyncDisabled{Val: true},
				&policy.EnableSyncConsent{Val: false},
			},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			ukmAppConn, err := navigateToPageAndLogElement(ctx, br,
				"chrome://ukm", `document.getElementsByClassName("ukm-collection-status")[0]`)
			if err != nil {
				s.Fatal("Failed to open website: ", err)
			}
			defer ukmAppConn.Close()

			// Open the website to log to ukm.
			conn, err := br.NewConn(ctx, testURL)
			if err != nil {
				s.Fatal("Failed to open website: ", err)
			}
			defer conn.Close()

			// verify logs on ukm app.
			if err := verifyOnUkmApp(ctx, ukmAppConn, param); err != nil {
				s.Fatal("Failed verify log on ukm app: ", err)
			}

			// wait to allow ukm time to write to log (writes every 20 seconds, starting after 1 minute).
			foundAnnotationErr := testing.Poll(ctx, func(ctx context.Context) (err error) {
				// Check the logs for given annotation.
				isFound, err := annotations.CheckLogs(ctx, cr, ukmNetworkAnnotationID)
				if err != nil {
					return testing.PollBreak(err)
				}

				if isFound {
					return nil
				}
				return errors.New("Annotation ID not found yet")
			}, &testing.PollOptions{
				Timeout:  80 * time.Second,
				Interval: 10 * time.Second,
			})

			// Stop logging.
			if err := annotations.StopLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}

			annotationFound := foundAnnotationErr == nil
			if annotationFound != param.annotationLogExpected {
				s.Fatal("Found: ", annotationFound, " But expected: ", param.annotationLogExpected)
			}
		})
	}
}

// verifyOnUkmApp - verifying that the test url is logged the ukm app.
func verifyOnUkmApp(ctx context.Context, ukmAppConn *chrome.Conn, param annotationTestParams) error {
	if param.annotationLogExpected {
		refreshBtn := `document.getElementById("refresh").click()`
		if err := ukmAppConn.Eval(ctx, refreshBtn, nil); err != nil {
			return errors.Wrap(err, "failed to refresh")
		}

		// Find a row in UKM.
		urlxPath := `//td[contains(@class, 'url') and normalize-space(text()) = '` + testURL + `/']`
		xpathFinder := `document.evaluate("` + urlxPath +
			`", document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue`
		if err := ukmAppConn.WaitForExprWithTimeout(ctx, xpathFinder, 3*time.Second); err != nil {
			return errors.Wrap(err, "failed to find url in the ukm app")
		}
	}

	return nil
}

// navigateToPageAndLogElement - used to navigate to a page and log the contents of an element on it.
func navigateToPageAndLogElement(ctx context.Context, br *browser.Browser, url, element string) (newConn *chrome.Conn, err error) {
	conn, err := br.NewConn(ctx, url)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open url app "+url)
	}

	if err := conn.WaitForExpr(ctx, element); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the element "+element)
	}
	var content string
	if err := conn.Eval(ctx, element+".innerText", &content); err != nil {
		return nil, errors.Wrap(err, "failed to get element "+element)
	}
	testing.ContextLog(ctx, content)

	return conn, nil
}
