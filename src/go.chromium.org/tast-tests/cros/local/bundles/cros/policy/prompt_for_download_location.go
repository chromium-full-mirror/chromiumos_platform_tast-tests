// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PromptForDownloadLocation,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of PromptForDownloadLocation policy: checking if a prompt for the download location appears based on the value of the policy",
		Contacts: []string{
			"chrome-downloads@google.com",
			"dtrainor@google.com",
			"qinmin@google.com",
			"alexanderhartl@google.com", // Test author
		},
		BugComponent: "b:47901",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		Data: []string{"prompt_for_download_location.html", "prompt_for_download_location.zip"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PromptForDownloadLocation{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// PromptForDownloadLocation tests the PromptForDownloadLocation policy.
func PromptForDownloadLocation(ctx context.Context, s *testing.State) {
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

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get users Download path: ", err)
	}

	defer func() { // Clean up Downloads directory.
		files, err := ioutil.ReadDir(downloadsPath)
		if err != nil {
			s.Fatal("Failed to get files from Downloads directory: ", err)
		}
		for _, file := range files {
			if err = os.RemoveAll(filepath.Join(downloadsPath, file.Name())); err != nil {
				s.Fatal("Failed to remove file: ", file.Name())
			}
		}
	}()

	for _, param := range []struct {
		name            string
		wantAsk         bool
		wantChecked     checked.Checked
		wantRestriction restriction.Restriction
		value           *policy.PromptForDownloadLocation
	}{
		{
			name:            "unset",
			wantAsk:         false,
			wantChecked:     checked.False,
			wantRestriction: restriction.None,
			value:           &policy.PromptForDownloadLocation{Stat: policy.StatusUnset},
		},
		{
			name:            "disabled",
			wantAsk:         false,
			wantChecked:     checked.False,
			wantRestriction: restriction.Disabled,
			value:           &policy.PromptForDownloadLocation{Val: false},
		},
		{
			name:            "enabled",
			wantAsk:         true,
			wantChecked:     checked.True,
			wantRestriction: restriction.Disabled,
			value:           &policy.PromptForDownloadLocation{Val: true},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			if err := policyutil.SettingsPage(ctx, cr, br, "downloads").
				SelectNode(ctx, nodewith.
					Name("Ask where to save each file before downloading").
					Role(role.ToggleButton)).
				Restriction(param.wantRestriction).
				Checked(param.wantChecked).
				Verify(); err != nil {
				s.Error("Unexpected settings state: ", err)
			}

			// Start a download.
			conn, err := br.NewConn(ctx, server.URL+"/prompt_for_download_location.html")
			if err != nil {
				s.Fatal("Failed to start download: ", err)
			}
			defer conn.Close()

			if err := conn.Eval(ctx, `document.getElementById('dlink').click()`, nil); err != nil {
				s.Fatal("Failed to execute JS expression: ", err)
			}

			// Check whether we get a prompt for the download location or if the file gets downloaded to the default folder.
			ui := uiauto.New(tconn)
			downloadPrompt := nodewith.Name("Save file as").Role(role.Window)
			asked := false
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				// Check if there is a prompt for the download location.
				if err = ui.Exists(downloadPrompt)(ctx); err == nil {
					asked = true
					return nil
				}

				// Check if the file was downloaded.
				if _, err := os.Stat(downloadsPath + "/prompt_for_download_location.zip"); err != nil && !os.IsNotExist(err) {
					return testing.PollBreak(errors.Wrap(err, "finding downloaded file failed"))
				} else if !os.IsNotExist(err) {
					// Remove downloaded file.
					if err := os.Remove(downloadsPath + "/prompt_for_download_location.zip"); err != nil {
						return testing.PollBreak(errors.Wrap(err, "failed to remove prompt_for_download_location.zip"))
					}
					asked = false
					return nil
				}

				return errors.New("found no prompt and no file")

			}, &testing.PollOptions{
				Timeout: 30 * time.Second,
			}); err != nil {
				s.Fatal("Failed to check if asked for download location: ", err)
			}

			if asked != param.wantAsk {
				s.Errorf("Unexpected existence of download location prompt: got %t; want %t", asked, param.wantAsk)
			}
		})
	}
}
