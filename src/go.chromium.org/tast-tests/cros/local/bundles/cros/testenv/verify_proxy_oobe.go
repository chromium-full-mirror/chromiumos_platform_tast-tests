// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyProxyOobe,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test that verifies mitmproxy working even not in a user session before login",
		Contacts: []string{
			"cros-ufo-testing@google.com",
			"yanghenry@google.com",
		},
		// ChromeOS > EngProd > Software > Trust & Safety > UFO Testing
		BugComponent: "b:1034522",
		SoftwareDeps: []string{"chrome"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			ui.GaiaPoolDefaultVarName,
		},
	})
}

func VerifyProxyOobe(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	mp, err := proxy.NewMitmProxy(ctx,
		proxy.CustomCA(true),
		proxy.DumpHTTPFlow(true),
	)
	if err != nil {
		s.Fatal("Failed to create new proxy: ", err)
	}
	defer mp.Close(cleanupCtx)

	options := []chrome.Option{
		chrome.DontSkipOOBEAfterLogin(),
		chrome.DeferLogin(),
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.ExtraArgs(fmt.Sprintf("--proxy-server=%s", mp.ProxyAddress())),
	}

	cr, err := chrome.New(ctx, options...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the signin profile test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	focusedButton := nodewith.State(state.Focused, true).Role(role.Button)

	s.Log("Waiting for the welcome screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}

	if err := uiauto.Combine("click next on the welcome screen",
		ui.WaitUntilExists(focusedButton),
		ui.LeftClick(focusedButton),
	)(ctx); err != nil {
		s.Fatal("Failed to click welcome screen next button: ", err)
	}

	if err := oobe.ProceedThroughNetworkScreen(ctx, oobeConn); err != nil {
		s.Fatal("Failed to proceed through network screen: ", err)
	}

	s.Log("Waiting for the user creation screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.UserCreationScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the user creation screen to be visible: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.UserCreationScreen.selectPersonalUser()", nil); err != nil {
		s.Fatal("Failed to select personal account: ", err)
	}

	nextButton := nodewith.Name("Next").Role(role.Button)
	if err := uiauto.Combine("click next on the user creation screen",
		ui.WaitUntilEnabled(nextButton),
		ui.LeftClick(nextButton),
	)(ctx); err != nil {
		s.Fatal("Failed to click user creation screen next button: ", err)
	}

	if err := oobe.ProceedThroughGaiaInfoScreen(ctx, oobeConn); err != nil {
		s.Fatal("Failed to proceed through Gaia Info screen: ", err)
	}

	resp, err := mp.DumpHTTPFlow(ctx, false, true)
	if err != nil {
		s.Fatal("Failed to get dump httpflow from mitmproxy: ", err)
	}

	v := proxy.NewNetworkVerifier(resp)
	if err := v.Verify([]string{"https://www\\.gstatic\\.com/"}, []string{}, []string{".*"}); err != nil {
		s.Fatal("Failed to find www.gstatic.com: ", err)
	}
}
