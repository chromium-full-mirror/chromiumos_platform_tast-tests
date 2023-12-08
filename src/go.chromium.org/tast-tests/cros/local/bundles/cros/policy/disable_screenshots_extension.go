// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
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
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const disableScreenshotsExtensionHTML = "disable_screenshots_extension.html"

var extensionFiles = []string{
	"screen_shooter_extension/background.js",
	"screen_shooter_extension/content.js",
	"screen_shooter_extension/manifest.json",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisableScreenshotsExtension,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of the DisableScreenshots policy, check whether screenshot can be taken by chrome.tabs.captureVisibleTab extensions API",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"poromov@google.com", // Policy owner
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.FakeDMS,
		Params: []testing.Param{{
			Val: browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
		Data: append(extensionFiles, disableScreenshotsExtensionHTML),
		// 2 minutes is the default local test timeout. Check localTestTimeout constant in tast/src/go.chromium.org/tast-tests/cros/internal/bundle/local.go.
		Timeout: chrome.ManagedUserLoginTimeout + 2*time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DisableScreenshots{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.Served),
		},
	})
}

func DisableScreenshotsExtension(ctx context.Context, s *testing.State) {
	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	fdms := s.FixtValue().(*fakedms.FakeDMS)
	bt := s.Param().(browser.Type)

	extDir, err := ioutil.TempDir("", "screen_shooter_extension")
	if err != nil {
		s.Fatal("Failed to create temp dir: ", err)
	}
	defer os.RemoveAll(extDir)

	if err := os.Chown(extDir, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		s.Fatalf("Failed to chown %q dir: %v", extDir, err)
	}

	for _, file := range extensionFiles {
		dst := filepath.Join(extDir, filepath.Base(file))
		if err := fsutil.CopyFile(s.DataPath(file), dst); err != nil {
			s.Fatalf("Failed to copy %q file to %q: %v", file, extDir, err)
		}
	}

	// Setting Lacros policy if needed
	pb := policy.NewBlob()
	if bt == browser.TypeLacros {
		pb.AddPolicies([]policy.Policy{&policy.LacrosAvailability{Val: "lacros_only"}})
	}
	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Setup browser based on the chrome type.
	chromeOpts := []chrome.Option{
		chrome.DMSPolicy(fdms.URL), chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
	}
	if bt == browser.TypeLacros {
		chromeOpts = append(chromeOpts, chrome.LacrosUnpackedExtension(extDir))
	} else {
		chromeOpts = append(chromeOpts, chrome.UnpackedExtension(extDir))
	}
	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(), chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)
	defer closeBrowser(ctx)

	for _, tc := range []struct {
		name      string
		value     []policy.Policy
		wantTitle string
	}{
		{
			name:      "true",
			value:     []policy.Policy{&policy.DisableScreenshots{Val: true}},
			wantTitle: "Taking screenshots has been disabled",
		},
		{
			name:      "false",
			value:     []policy.Policy{&policy.DisableScreenshots{Val: false}},
			wantTitle: "Screen capture allowed",
		},
		{
			name:      "unset",
			value:     []policy.Policy{},
			wantTitle: "Screen capture allowed",
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			// This is needed only for debugging purpose to understand why the test is flaky.
			// TODO(crbug:1159824): Remove once test will be stable.
			defer func(ctx context.Context) {
				if s.HasError() {
					if err := screenshot.Capture(ctx, filepath.Join(s.OutDir(), fmt.Sprintf("screenshot_%s.png", tc.name))); err != nil {
						s.Error("Failed to capture screenshot: ", err)
					}
				}
			}(cleanupCtx)

			// GoBigSleepLint: Minimum interval between captureVisibleTab requests
			// is 1 second, so we must sleep for 1 seconds to be able to take
			// screenshot, otherwise API will return an error.
			// Please check MAX_CAPTURE_VISIBLE_TAB_CALLS_PER_SECOND constant in
			// chrome/common/extensions/api/tabs.json
			if err := testing.Sleep(ctx, time.Second); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}

			policies := tc.value
			if bt == browser.TypeLacros {
				policies = append(policies, &policy.LacrosAvailability{Val: "lacros_only"})
			}
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, policies); err != nil {
				s.Fatal("Failed to serve and verify: ", err)
			}

			conn, err := br.NewConn(ctx, server.URL+"/"+disableScreenshotsExtensionHTML)
			if err != nil {
				s.Fatal("Failed to create a tab: ", err)
			}
			defer conn.Close()

			// Wait until page has desired title to avoid race conditions.
			if err := conn.WaitForExpr(ctx, `document.title === "Page Title"`); err != nil {
				s.Fatal("Failed to execute JS in extension: ", err)
			}

			// Here we check only `chrome.tabs.captureVisibleTab` extension API.
			if err := conn.Eval(ctx, `document.title = "captureVisibleTab"`, nil); err != nil {
				s.Fatal("Failed to execute JS in extension: ", err)
			}

			if err := keyboard.Accel(ctx, "Ctrl+Shift+Y"); err != nil {
				s.Fatal("Failed to press Ctrl+Shift+Y: ", err)
			}

			if err := conn.WaitForExpr(ctx, `document.title != "captureVisibleTab"`); err != nil {
				s.Fatal("Failed to execute JS in extension: ", err)
			}

			var title string
			if err := conn.Eval(ctx, `document.title`, &title); err != nil {
				s.Fatal("Failed to execute JS in extension: ", err)
			}
			if title != tc.wantTitle {
				s.Errorf("Unexpected title: get %q; want %q", title, tc.wantTitle)
			}
		})
	}
}
