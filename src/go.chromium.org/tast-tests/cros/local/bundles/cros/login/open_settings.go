// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Open OS Settings and access them with a password",
		Contacts: []string{
			"cros-lurs@google.com",
			"emaamari@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{"group:mainline", "informational", "group:hw_agnostic"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 2*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + 2*time.Minute,
	})
}

func OpenSettings(ctx context.Context, s *testing.State) {
	const (
		username = "testuser@gmail.com"
		password = "testpassword"
	)

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	defer userutil.ResetUsers(cleanupContext)

	cr, err := login.SetupUserWithLocalPassword(ctx,
		password,
		// Use a local password so that we don't need Gaia.
		chrome.FakeLogin(chrome.Creds{User: username, Pass: ""}),
		chrome.ExtraArgs("--disable-first-run-ui"),
	)
	if err != nil {
		s.Fatal("Failed to setup user: ", err)
	}
	defer cr.Close(cleanupContext)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Getting test API connection failed: ", err)
	}

	// Open OS Settings > lock screen.
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osPrivacy/lockScreen", func(context.Context) error { return nil })
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupContext)
	defer faillog.DumpUITreeOnError(cleanupContext, s.OutDir(), s.HasError, tconn)

	// The page is password protected, confirm that we can access it with a password.
	if err := ossettings.ConfirmPassword(ctx, cr, password); err != nil {
		s.Fatal("Failed to confirm password: ", err)
	}
}
