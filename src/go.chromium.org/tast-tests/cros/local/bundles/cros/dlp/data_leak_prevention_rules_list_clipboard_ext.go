// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	policyBlob "go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/clipboard"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListClipboardExt,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with clipboard blocked restriction when accessed by extension",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"ayaelattar@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Attr:         []string{"group:hw_agnostic"},
		Fixture:      "fakeDMS",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policyBlob.LacrosAvailability{}, pci.Served),
		},
		Data: []string{"manifest.json", "background.js", "content.js", "text_1.html", "text_2.html", "editable_text_box.html"},
		Params: []testing.Param{{
			Name:      "ash",
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
			Val:       browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraAttr:         []string{"group:mainline", "informational", "group:criticalstaging"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}
func DataLeakPreventionRulesListClipboardExt(ctx context.Context, s *testing.State) {
	fakeDMS := s.FixtValue().(*fakedms.FakeDMS)
	bt := s.Param().(browser.Type)

	// DLP policy with all clipboard blocked restriction.
	policyDLP := policy.RestrictiveDLPPolicyForClipboard("example.com")
	if bt == browser.TypeLacros {
		policyDLP = append(policyDLP, &policyBlob.LacrosAvailability{Val: "lacros_only"})
	}

	// Update the policy blob.
	pb := policyBlob.NewBlob()
	pb.AddPolicies(policyDLP)
	if err := fakeDMS.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	extDir, err := ioutil.TempDir("", "tast.dlp.Clipboard.")
	if err != nil {
		s.Fatal("Failed to create temp extension dir: ", err)
	}
	defer os.RemoveAll(extDir)

	extID, err := setUpExtension(ctx, s, extDir)
	if err != nil {
		s.Fatal("Failed setup of DLP Clipboard extension: ", err)
	}

	chromeOpts := []chrome.Option{chrome.DMSPolicy(fakeDMS.URL), chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password})}
	if bt == browser.TypeLacros {
		chromeOpts = append(chromeOpts, chrome.LacrosUnpackedExtension(extDir))
	} else {
		chromeOpts = append(chromeOpts, chrome.UnpackedExtension(extDir))
	}
	// Start a Chrome instance that will fetch policies from the FakeDMS.
	// Policies are only updated after Chrome startup.
	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(),
		chromeOpts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)
	defer closeBrowser(ctx)

	bgURL := chrome.ExtensionBackgroundPageURL(extID)
	conn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(bgURL))
	if err != nil {
		s.Fatalf("Failed to connect to background page at %v: %v", bgURL, err)
	}
	defer conn.Close()

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	allowedServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer allowedServer.Close()

	blockedServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer blockedServer.Close()

	destServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer destServer.Close()

	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, policy.PopulateDLPPolicyForClipboard(blockedServer.URL, destServer.URL)); err != nil {
		s.Fatal("Failed to serve and verify the DLP policy: ", err)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	s.Log("Waiting for chrome.clipboard API to become available")
	if err := tconn.WaitForExpr(ctx, "chrome.clipboard"); err != nil {
		s.Fatal("Failed to wait for chrome.clipboard API to become available: ", err)
	}

	info, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		s.Fatal("No display: ", err)
	}

	// Display bounds.
	displayWidth := int(info.Bounds.Width)
	displayHeight := int(info.Bounds.Height)

	// Check extension access with restricted and non-restricted site.
	// See RestrictiveDLPPolicyForClipboard function in policy package for more details.
	for _, param := range []struct {
		name          string
		sourceURL     string
		accessAllowed bool
	}{
		{
			name:          "accessAllowed",
			sourceURL:     allowedServer.URL + "/text_2.html",
			accessAllowed: true,
		},
		{
			name:          "accessDenied",
			sourceURL:     blockedServer.URL + "/text_1.html",
			accessAllowed: false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			conn, err := br.NewConn(ctx, param.sourceURL)
			if err != nil {
				s.Fatalf("Failed to open page %q: %v", param.sourceURL, err)
			}
			defer conn.Close()

			if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
				s.Fatalf("Failed to wait for %q to be loaded and achieve quiescence: %s", param.sourceURL, err)
			}

			ui := uiauto.New(tconn)
			if err := uiauto.Combine("copy all text from source website",
				keyboard.AccelAction("Ctrl+A"),
				keyboard.AccelAction("Ctrl+C"))(ctx); err != nil {
				s.Fatal("Failed to copy text from source browser: ", err)
			}

			destURL := destServer.URL + "/editable_text_box.html"
			destConn, err := br.NewConn(ctx, destURL)
			if err != nil {
				s.Fatal("Failed to open page: ", err)
			}
			defer destConn.Close()

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			if err := webutil.WaitForQuiescence(ctx, destConn, 10*time.Second); err != nil {
				s.Fatalf("Failed to wait for %q to achieve quiescence: %v", destURL, err)
			}

			if err := uiauto.Combine("Select tab and press Ctrl+Z",
				// Select tab for the extension.
				ui.MouseClickAtLocation(0, coords.Point{X: displayWidth / 2, Y: displayHeight / 2}),
				// A custom command to which DLP extension listens and then reads clipboard data.
				keyboard.AccelAction("Ctrl+Z"))(ctx); err != nil {
				s.Fatal("Failed to select tab and press Ctrl+Z: ", err)
			}

			expectedTitle := "Extension Restricted"
			if param.accessAllowed {
				expectedTitle = "Extension Access"
			}
			var actualTitle string

			// This can be too fast, so poll till the extension updates the webpage title.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err := destConn.Eval(ctx, "document.title", &actualTitle); err != nil {
					return errors.Wrap(err, "failed to get the webpage title")
				}

				if expectedTitle != actualTitle {
					return errors.New("Page title not as expected")
				}

				return nil
			}, &testing.PollOptions{
				Timeout:  5 * time.Second,
				Interval: 1 * time.Second,
			}); err != nil {
				s.Fatalf("Found page title %s, expected %s: %s", actualTitle, expectedTitle, err)
			}

			parsedSourceURL, err := url.Parse(blockedServer.URL)
			if err != nil {
				s.Fatalf("Failed to parse blocked server URL %s: %s", blockedServer.URL, err)
			}

			err = clipboard.CheckClipboardBubble(ctx, ui, parsedSourceURL.Hostname())
			// Clipboard DLP bubble is not expected when access allowed.
			if err == nil && param.accessAllowed {
				s.Error("Notification found, expected none")
			}

			// Clipboard DLP bubble is expected when access not allowed.
			if err != nil && !param.accessAllowed {
				s.Error("Notification not found, expected DLP clipboard notification")
			}
		})
	}
}

// setUpExtension moves the extension files into the extension directory and returns extension ID.
func setUpExtension(ctx context.Context, s *testing.State, extDir string) (string, error) {
	for _, name := range []string{"manifest.json", "background.js", "content.js"} {
		if err := fsutil.CopyFile(s.DataPath(name), filepath.Join(extDir, name)); err != nil {
			return "", errors.Wrapf(err, "failed to copy file %s", name)
		}
	}

	extID, err := chrome.ComputeExtensionID(extDir)
	if err != nil {
		s.Fatalf("Failed to compute extension ID for %v: %v", extDir, err)
	}

	return extID, nil
}
