// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OobeArcAppOpen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launch ARC App post the OOBE Flow Setup Complete",
		Contacts:     []string{"cros-arc-te@google.com", "cros-oac@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:arc", "arc_core", "group:arc-functional"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 35*time.Minute,
		VarDeps: []string{"arc.parentUser", "arc.parentPassword"},
	})
}

func OobeArcAppOpen(ctx context.Context, s *testing.State) {

	const (
		appPkgName  = "com.google.android.apps.books"
		appActivity = ".app.BooksActivity"
	)

	requeiredPkgNames := []string{
		"com.google.android.apps.books",
		"com.google.android.apps.youtube.music.pwa",
		"com.google.android.apps.photos",
		"com.google.android.apps.books",
		"com.google.android.play.games",
		"com.google.android.videos",
	}

	username := s.RequiredVar("arc.parentUser")
	password := s.RequiredVar("arc.parentPassword")

	cr, err := chrome.New(ctx,
		chrome.DontSkipOOBEAfterLogin(),
		chrome.ARCSupported(),
		chrome.GAIALogin(chrome.Creds{User: username, Pass: password}),
		// TODO(b/287862720): Update the test to go through CHOOBE flow and stop disabling the features.
		chrome.ExtraArgs("--disable-features=OobeChoobe,OobeDisplaySize,OobeTouchpadScrollDirection"))

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)
	ui := uiauto.New(tconn)

	if err := oobe.CompleteOnboardingFlow(ctx, ui); err != nil {
		s.Fatal("Failed to go through the oobe flow: ", err)
	}

	// Setup ARC.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	statusArea := nodewith.HasClass("ash/StatusAreaWidgetDelegate")

	s.Log("Waiting for setup complete notification")
	_, err = ash.WaitForNotification(ctx, tconn, 25*time.Minute, ash.WaitTitle("Setup complete"), ash.WaitMessageContains("Installed 6 out of 6 applications"))
	if err != nil {
		s.Log("Haven't found setup complete notification, will try to see if it's in notification UI")
		if err := ui.LeftClick(statusArea)(ctx); err != nil {
			s.Log("Failed to click status area : ", err)
		}
		uda := uidetection.NewDefault(tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)

		setupCompleteNotification := uidetection.TextBlock([]string{"Setup", "complete"})
		if err := uda.WithTimeout(25 * time.Second).WaitUntilExists(setupCompleteNotification)(ctx); err != nil {
			if err := a.WaitForPackages(ctx, requeiredPkgNames); err != nil {
				s.Fatal("Failed waiting for Setup complete notification after checking installed packages: ", err)
			}
		}
	}

	s.Log("Waiting to check if app is installed before launching the app")
	if err := ash.WaitForChromeAppInstalled(ctx, tconn, apps.PlayBooks.ID, 2*time.Minute); err != nil {
		s.Fatal("Failed to wait for app to install: ", err)
	}

	s.Log("Launch the App")
	act, err := arc.NewActivity(a, appPkgName, appActivity)
	if err != nil {
		s.Fatal("Failed to create a new activity: ", err)
	}
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		act.Close(ctx)
		s.Fatal("Failed to start the activity: ", err)
	}

}
