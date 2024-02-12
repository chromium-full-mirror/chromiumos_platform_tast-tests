// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	da "go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputCheckRegionalKey,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Input page shows expected regional keyboard layout with different region code",
		// ChromeOS > Software > System Services > Serviceability > Diagnostics
		BugComponent: "b:1131925",
		Contacts: []string{
			"cros-peripherals@google.com",
			"dpad@google.com",
			"jeff.lin@cienet.com",
			"xliu@cienet.com",
			"ashleydp@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
		Params: []testing.Param{{
			Name: "us",
		}, {
			Name: "jp",
		}, {
			Name: "fr",
		}},
	})
}

func InputCheckRegionalKey(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	regionCode := strings.Split(s.TestName(), "InputCheckRegionalKey.")[1]
	cr, err := chrome.New(ctx, chrome.Region(regionCode), chrome.EnableFeatures("DiagnosticsAppNavigation", "EnableInputInDiagnosticsApp"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if err = apps.Launch(ctx, tconn, apps.Diagnostics.ID); err != nil {
		s.Fatal("Failed to launch diagnostics app")
	}
	defer da.Close(cleanupCtx, tconn)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err = ash.WaitForApp(ctx, tconn, apps.Diagnostics.ID, time.Minute); err != nil {
		s.Fatal("Diagnostics app did not appear in shelf after launch")
	}

	ui := uiauto.New(tconn)
	dxRootNode := da.DxRootNodes[regionCode]
	if err = ui.WithTimeout(da.DefaultTimeout).WaitUntilExists(dxRootNode)(ctx); err != nil {
		s.Fatal("Failed to find diagnostics app")
	}

	menuButton := da.DxNarrowMenuButtons[regionCode].Ancestor(dxRootNode)
	if err := uiauto.IfSuccessThen(ui.WithTimeout(da.DefaultTimeout).WaitUntilExists(menuButton),
		ui.WithPollOpts(da.DefaultPolling).LeftClick(menuButton))(ctx); err != nil {
		s.Fatal("Menu click failed")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	internalKeyboardTestButton, ok := da.DxInternalKeyboardTestButtons[regionCode]
	if !ok {
		s.Fatalf("Region code %v has not defined in test button map yet: ", regionCode)
	}
	inputTab, ok := da.DxKeyboardTabButtons[regionCode]
	if !ok {
		s.Fatalf("Region code %v has not defined in input button map yet: ", regionCode)
	}
	inputTab = inputTab.Ancestor(dxRootNode)
	if err := uiauto.Combine("checks regional keys for region "+regionCode+" keyboard layout",
		ui.LeftClick(inputTab),
		ui.LeftClick(internalKeyboardTestButton),
		da.CheckGlyphsbyRegion(ui, regionCode),
	)(ctx); err != nil {
		s.Fatal("Failed to checks regional keys: ", err)
	}
}
