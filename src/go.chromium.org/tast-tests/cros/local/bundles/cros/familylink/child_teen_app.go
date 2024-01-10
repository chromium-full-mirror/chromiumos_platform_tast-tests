// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChildTeenApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that clicking the 'Parental controls' link in the Accounts page in Settings launches the Child & Teen version of the Family Link app",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"amberhaynes@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "arc"},
		Params: []testing.Param{
			{
				Name:    "unicorn",
				Fixture: "familyLinkUnicornArcPolicyLogin",
			}, {
				Name:    "geller",
				Fixture: "familyLinkGellerArcPolicyLogin",
			},
		},
		Timeout: 2*time.Minute + apputil.InstallationTimeout,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.Served),
		},
	})
}

// ChildTeenApp verifies that clicking the 'Parental controls' link in the Accounts page in Settings launches the Child & Teen version of the Family Link app.
func ChildTeenApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Set ArcEnabled policy and ensure chrome://policy shows correct ArcEnabled value.
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{&policy.ArcEnabled{Val: true}}); err != nil {
		s.Fatal("Failed to serve and verify ArcEnabled: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to create the instance of ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create the keyboard: ", err)
	}
	defer kb.Close(ctx)

	device, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer device.Close(cleanupCtx)

	app, err := apputil.NewApp(ctx, kb, tconn, a, device, "Family Link child and teen", "com.google.android.apps.kids.familylinkhelper")
	if err != nil {
		s.Fatal("Failed to create the instance of App: ", err)
	}
	defer app.Close(cleanupCtx, cr, s.HasError, s.OutDir())

	if err := app.Install(ctx); err != nil {
		s.Fatalf("Failed to install %s: %v", app.AppName, err)
	}

	// Launch the Settings and navigate to the Accounts page.
	settings, err := ossettings.LaunchAtPage(ctx, tconn, nodewith.Name("Accounts").Role(role.Link))
	if err != nil {
		s.Fatal("Failed to open Accounts page: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Click the 'Parental controls' block in the Accounts page.
	if err := uiauto.New(tconn).LeftClick(nodewith.NameContaining("Parental controls Open").Ancestor(ossettings.WindowFinder))(ctx); err != nil {
		s.Fatal("Failed to click Parental controls: ", err)
	}
	// Defer close app window right after it's launched.
	defer func(ctx context.Context) {
		w, err := ash.GetARCAppWindowInfo(ctx, tconn, app.PkgName)
		if err != nil {
			s.Log("Failed to get ARC UI window info: ", err)
		}
		w.CloseWindow(ctx, tconn)
	}(cleanupCtx)

	// The app, Family Link child and teen, should be launched.
	if err := ash.WaitForVisible(ctx, tconn, app.PkgName); err != nil {
		s.Fatalf("Failed to open %s: %v", app.AppName, err)
	}
}
