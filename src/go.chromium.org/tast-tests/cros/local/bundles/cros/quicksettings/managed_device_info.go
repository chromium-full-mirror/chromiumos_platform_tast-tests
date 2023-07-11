// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// managedDeviceInfoTestParam is the parameter for ManagedDeviceInfo tests.
type managedDeviceInfoTestParam struct {
	// Which type of browser to start (ash or lacros).
	browserType browser.Type
	// Whether feature QsRevamp is enabled.
	qsRevamp bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedDeviceInfo,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that the Quick Settings managed device info is displayed correctly",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"leandre@chromium.org",
			"amehfooz@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "qs_revamp_disabled",
			Fixture: fixture.FakeDMS,
			Val: managedDeviceInfoTestParam{
				browserType: browser.TypeAsh,
				qsRevamp:    false,
			},
		}, {
			Name:    "qs_revamp_enabled",
			Fixture: fixture.FakeDMS,
			Val: managedDeviceInfoTestParam{
				browserType: browser.TypeAsh,
				qsRevamp:    true,
			},
		}, {
			Name:              "lacros_qs_revamp_disabled",
			Fixture:           fixture.PersistentLacros,
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val: managedDeviceInfoTestParam{
				browserType: browser.TypeLacros,
				qsRevamp:    false,
			},
		}, {
			Name:              "lacros_qs_revamp_enabled",
			Fixture:           fixture.PersistentLacros,
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val: managedDeviceInfoTestParam{
				browserType: browser.TypeLacros,
				qsRevamp:    true,
			},
		}},
	})
}

// ManagedDeviceInfo tests that the Quick Settings managed device info is displayed correctly.
func ManagedDeviceInfo(ctx context.Context, s *testing.State) {
	const uiTimeout = 10 * time.Second

	// Reserve some time for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Start a Browser instance that will fetch policies from the FakeDMS.
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(managedDeviceInfoTestParam)
	bt := param.browserType
	opts := []chrome.Option{
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EnableFeatures("ManagedDeviceUIRedesign"),
	}
	if param.qsRevamp {
		opts = append(opts, chrome.EnableFeatures("QsRevamp"))
	}
	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(), opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show Quick Settings: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)

	// Check if management information is shown.
	ui := uiauto.New(tconn)
	managedBtn := quicksettings.ManagedInfoView
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(managedBtn)(ctx); err != nil {
		s.Fatal("Failed to find managed info button: ", err)
	}

	// Check if the information contains the managed domain name or indication that the device is "enterprise managed" (depending on test account configuration).
	info, err := ui.Info(ctx, managedBtn)
	if err != nil {
		s.Fatal("Failed to get management information button info: ", err)
	}
	if !strings.Contains(info.Name, "managedchrome.com") && !strings.Contains(info.Name, "enterprise managed") {
		s.Fatalf("Managed info string: %q, expected containing management domain name or enterprise managed indication", info.Name)
	}

	if err := ui.LeftClick(managedBtn)(ctx); err != nil {
		s.Fatal("Failed to click management information button: ", err)
	}

	// Check if management page is open after clicking the button.
	if _, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL("chrome://management/")); err != nil {
		s.Fatal("Management page did not open: ", err)
	}
}
