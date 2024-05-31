// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apps contains functionality and test cases for ChromeOS essential Apps.
package apps

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/pre"
	"go.chromium.org/tast-tests/cros/local/chrome"
	calc "go.chromium.org/tast-tests/cros/local/chrome/apps/calculator"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CalculatorSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Calculator smoke test app launching and basic calculation",
		Contacts: []string{
			"apps-suite@google.com",
			"zafzal@google.com",
			"jinrongwu@google.com",
		},
		BugComponent: "b:961438",
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		HardwareDeps: hwdep.D(pre.AppsStableModels),
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome", "chrome_internal", "gaia"},
	})
}

func CalculatorSmoke(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.EnableWebAppInstall())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := ash.WaitForChromeAppInstalled(ctx, tconn, apps.Calculator.ID, 5*time.Minute); err != nil {
		s.Fatal("Failed to wait for installed app: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.Calculator.ID); err != nil {
		s.Fatal("Failed to launch Calculator: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Calculator.ID, time.Minute); err != nil {
		s.Fatalf("Fail to wait for %s by app id %s: %v", apps.Calculator.Name, apps.Calculator.ID, err)
	}

	appConn, err := calc.UIConn(ctx, cr)
	if err != nil {
		s.Fatal("Failed to connect to Calculator web page: ", err)
	}
	defer appConn.Close()

	// perform 1+2=3.
	if err := uiauto.Combine("perform basic calculation",
		calc.TapKey(appConn, "1"),
		calc.TapKey(appConn, "plus"),
		calc.TapKey(appConn, "2"),
		calc.TapKey(appConn, "equals"),
		calc.WaitForCalculateResult(appConn, "3"),
	)(ctx); err != nil {
		s.Fatal("Failed to perform calculation: ", err)
	}
}
