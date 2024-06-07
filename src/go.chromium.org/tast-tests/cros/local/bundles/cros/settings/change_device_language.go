// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChangeDeviceLanguage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Change device language and validate new langauge after restart",
		Contacts: []string{
			"cros-borders-eng@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		BugComponent: "b:1282854",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Timeout:      5 * time.Minute,
	})
}

func ChangeDeviceLanguage(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer func() {
		if cr != nil {
			cr.Close(cleanupCtx)
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	const (
		languageSearchKeyboard   = "chinese"              // keyword used to search for adding language.
		languageUniqueIdentifier = "Chinese (Simplified)" // The unique keyboard in the language full name.
		uiIdentifier             = "关机"                   // Text in local language of "Power off".
	)

	settings, err := ossettings.LaunchAtLanguageSettingsPage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open language page: ", err)
	}
	defer func() {
		defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "ui_dump")
	}()

	if err := settings.ChangeDeviceLanguageAndRestart(ctx, tconn, languageSearchKeyboard, languageUniqueIdentifier); err != nil {
		s.Fatal("Failed to change device language: ", err)
	}

	// Close Chrome to do some cleanup.
	// It will log some errors, as the session is closed already.
	cr.Close(ctx)
	cr = nil

	// Sleep a short time to ensure reboot button is safely clicked.
	// TODO: Improve logic to wait for an event as per GoBigSleepLint
	if err := testing.Sleep(ctx, 1*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	cr, err = chrome.New(ctx, chrome.KeepState(), chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome after changing device language: ", err)
	}

	tconn, err = cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to sign-in profile test api: ", err)
	}

	if err := uiauto.New(tconn).WaitUntilExists(nodewith.Name(uiIdentifier).First())(ctx); err != nil {
		s.Fatal("New language is not used after changing device language: ", err)
	}
}
