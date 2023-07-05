// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	policyannotations "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/policy_annotations"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrafficAnnotationDomainReliability,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "This test checks the network annotataion for Domain Reliability to make sure we are not sending network traffic when it's off",
		Contacts: []string{
			"chrome-ess-engprod@google.com",
			"meyron@google.com",
			"rzakarian@google.com",
		},
		BugComponent: "b:1152652", // Chrome Operations > BrApp EngProd > ESS > Enterprise Infra
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      4 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.ChromeEnrolledLoggedInShortMetricsInterval,
				Val:     browser.TypeAsh,
			},
		},
		Data: []string{"autofill_address_enabled.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DomainReliabilityAllowed{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.EnableSyncConsent{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SyncDisabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func TrafficAnnotationDomainReliability(ctx context.Context, s *testing.State) {
	const domainReliabilityTestURL = "images.google.com"

	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		s.Fatal("Failed to create fdms temp dir: ", err)
	}
	defer os.RemoveAll(tmpDir)
	altHostPath := filepath.Join(tmpDir, "hosts")
	hostsContent := []byte("127.0.0.1       " + domainReliabilityTestURL)
	// Add line to hosts file
	if err := os.WriteFile(altHostPath, hostsContent, 0644); err != nil {
		s.Fatal("Failed to create alternate hosts file: ", err)
	}

	os.Setenv("HOSTALIASES", altHostPath)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve 10 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	const domainReliabilityNetworkAnnotationID = "108804096"
	for _, param := range []policyannotations.AnnotationTestParams{
		{
			Name:                  "domain_reliability_false",
			AnnotationLogExpected: false,
			Policies: []policy.Policy{
				&policy.DomainReliabilityAllowed{Val: false},
				&policy.SyncDisabled{Val: false},
				&policy.EnableSyncConsent{Val: true},
			},
		},
		{
			Name:                  "domain_reliability_true",
			AnnotationLogExpected: true,
			Policies: []policy.Policy{
				&policy.DomainReliabilityAllowed{Val: true},
				&policy.SyncDisabled{Val: false},
				&policy.EnableSyncConsent{Val: true},
			},
		},
	} {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.Policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br, false); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			// Try to open the website to domain event.
			conn, err := br.NewConn(ctx, "https://"+domainReliabilityTestURL)
			if err != nil {
				s.Log("Failed to open website: ", err)
			}
			defer conn.Close()

			// wait to allow time to write to log (writes every 20 seconds, starting after 1 minute).
			foundAnnotationErr := testing.Poll(ctx, func(ctx context.Context) (err error) {
				// Check the logs for given annotation.
				isFound, err := annotations.CheckLogs(ctx, cr, domainReliabilityNetworkAnnotationID)
				if err != nil {
					return testing.PollBreak(err)
				}

				if isFound {
					return nil
				}
				return errors.New("Annotation ID not found yet")
			}, &testing.PollOptions{
				Timeout:  60 * time.Second,
				Interval: 10 * time.Second,
			})

			// Stop logging.
			if err := annotations.StopLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}

			annotationFound := foundAnnotationErr == nil
			if annotationFound != param.AnnotationLogExpected {
				s.Fatal("Found: ", annotationFound, " But expected: ", param.AnnotationLogExpected)
			}
		})
	}

	// Restore hosts file
	os.Setenv("HOSTALIASES", "")
}
