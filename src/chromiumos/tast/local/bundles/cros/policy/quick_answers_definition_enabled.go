// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/event"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/quickanswers"
	"chromiumos/tast/testing"
	"go.chromium.org/tast/core/ctxutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         QuickAnswersDefinitionEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test QuickAnswersDefinitionEnabled policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr:         []string{"group:commercial_limited"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"quick_answers.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.QuickAnswersDefinitionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersDefinitionEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           fixture.LacrosPolicyLoggedIn,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// QuickAnswersDefinitionEnabled tests that Quick Answers definitions can be enabled and disabled via policy.
func QuickAnswersDefinitionEnabled(ctx context.Context, s *testing.State) {
	const (
		annotationID = "46208118" // quick_answers_loader

		// Static local test file containing test string.
		testFileName           = "quick_answers.html"
		textToDefine           = "icosahedron"
		expectedDefinitionText = "twenty plane faces"
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	testFilePath := server.URL + "/" + testFileName
	defer server.Close()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := quickanswers.SetPrefValue(ctx, tconn, "settings.quick_answers.enabled", true); err != nil {
		s.Fatal("Failed to enable Quick Answers: ", err)
	}

	for _, param := range []struct {
		name                  string
		shouldFindAnnotation  bool
		shouldShowContextMenu bool
		policy                *policy.QuickAnswersDefinitionEnabled
	}{
		{
			name:                  "unset",
			shouldFindAnnotation:  true,
			shouldShowContextMenu: true,
			policy:                &policy.QuickAnswersDefinitionEnabled{Stat: policy.StatusUnset},
		},
		{
			name:                  "enabled",
			shouldFindAnnotation:  true,
			shouldShowContextMenu: true,
			policy:                &policy.QuickAnswersDefinitionEnabled{Val: true},
		},
		{
			name:                  "disabled",
			shouldFindAnnotation:  false,
			shouldShowContextMenu: false,
			policy:                &policy.QuickAnswersDefinitionEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			ui := uiauto.New(tconn)

			// Setup a browser.
			bt := s.Param().(browser.Type)
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			// Open page with the query word on it.
			conn, err := br.NewConn(ctx, testFilePath, browser.WithNewWindow())
			if err != nil {
				s.Fatal("Failed to create new Chrome connection: ", err)
			}
			defer conn.Close()
			defer conn.CloseTarget(ctx)

			// Wait for the query word to appear.
			query := nodewith.Name(textToDefine).Role(role.StaticText).First()
			if err := ui.WaitUntilExists(query)(ctx); err != nil {
				s.Fatal("Failed to wait for query to load: ", err)
			}

			// Select the word and setup watcher to wait for text selection event.
			if err := ui.WaitForEvent(nodewith.Root(),
				event.TextSelectionChanged,
				ui.Select(query, 0 /*startOffset*/, query, 2 /*endOffset*/))(ctx); err != nil {
				s.Fatal("Failed to select query: ", err)
			}

			quickAnswers := nodewith.ClassName("QuickAnswersView")
			definitionResult := nodewith.NameContaining(expectedDefinitionText).ClassName("QuickAnswersTextLabel")

			// Right click the selected word and ensure the Quick Answers UI shows the definition.
			if err := uiauto.Combine("Show context menu",
				ui.RightClick(query),
				ui.WaitUntilExists(quickAnswers),
				ui.WaitUntilExists(definitionResult),
			)(ctx); err != nil {
				if param.shouldShowContextMenu {
					s.Fatal("Quick Answers result not showing up: ", err)
				} else if !nodewith.IsNodeNotFoundErr(err) {
					s.Fatal("Failure while trying to show the context menu: ", err)
				} // else the context menu was not found, as expected
			} else if !param.shouldShowContextMenu {
				s.Fatal("Quick Answers result shows when it should be disabled")
			}

			if param.shouldShowContextMenu {
				// Dismiss the context menu and ensure the Quick Answers UI also dismisses.
				if err := uiauto.Combine("Dismiss context menu",
					ui.LeftClick(query),
					ui.WaitUntilGone(quickAnswers),
				)(ctx); err != nil {
					s.Fatal("Quick Answers result not dismissed: ", err)
				}
			}

			// Stop logging and check the logs for quick_answers_loader NetworkTrafficAnnotationTag.
			foundAnnotation, err := annotations.StopLoggingCheckLogs(ctx, cr, br, annotationID)
			if err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}
			if param.shouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Expected: %t. Actual: %t", param.shouldFindAnnotation, foundAnnotation)
			}
		})
	}
}
