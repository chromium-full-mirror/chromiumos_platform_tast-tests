// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/login/identitycuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	sessionCookiesCopyTimeout = 5 * time.Second
	waitForLoginScreenTimeout = 30 * time.Second
	// docsTab name in Google docs.
	docsTab = "Google Docs"
	// slidesTab name in Google docs.
	slidesTab = "Google Slides"
	// Text to be written to a Google doc in the |docsTab|.
	docText = "Text to write to Google docs"
	// Text to be written to a Google slides in the |slidesTab|.
	slidesTitleText = "Text to write to the title field in Google slides"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SSO,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies in-session sso with Google services for managed accounts on unmanaged device",
		Contacts: []string{
			"cros-3pidp@google.com",
			"mslus@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1027806", // ChromeOS > Software > Commercial (Enterprise) > Identity
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// CUJ: access data and services from my private or non-corp device.
			Value: "screenplay-ef8f5d9b-43a8-46c4-88f9-20fe96d53bc5",
		}, {
			Key: "feature_id",
			// CUJ: require the same authentication across services.
			Value: "screenplay-64702801-83b5-4994-8dad-08afda61e58c",
		}},
		Vars: []string{
			tape.ServiceAccountVar,
		},
		Timeout: 3 * time.Minute,
	})
}

func SSO(ctx context.Context, s *testing.State) {

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Create a managed device account.
	acc, err := identitycuj.CreateManagedDeviceAccount(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}

	// chrome.NoLogin() and chrome.KeepState() to show the login screen with
	// a user pod (instead of the OOBE login screen).
	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctxForCleanup)

	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	s.AttachErrorHandlers(nil /*OnErrorHandler*/, func(errMsg string) {
		faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tLoginConn)
	})

	// Wait for the login screen to be ready for password entry.
	if st, err := lockscreen.WaitState(ctx, tLoginConn, func(st lockscreen.State) bool { return st.ReadyForPassword }, waitForLoginScreenTimeout); err != nil {
		s.Fatalf("Failed waiting for the login screen to be ready for password entry: %v, last state: %+v", err, st)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctxForCleanup)

	// Enter password to log in.
	if err := lockscreen.EnterPassword(ctx, tLoginConn, acc.Username, acc.Password, kb); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if err := lockscreen.WaitForLoggedIn(ctx, tLoginConn, chrome.LoginTimeout); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	// TODO(b/297183686) This should be improved by waiting on an actual event.
	// GoBigSleepLint: waiting 5 seconds to allow auth cookies to be copied to the new session.
	if err := testing.Sleep(ctx, sessionCookiesCopyTimeout); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	createNewGoogleDocs(ctx, s, cr)
}

func createNewGoogleDocs(ctx context.Context, s *testing.State, cr *chrome.Chrome) {
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctxForCleanup)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Open a Google doc in a new browser tab.
	_, err = identitycuj.OpenNewURL(ctx, cr, cuj.NewGoogleDocsURL)
	if err != nil {
		s.Fatal("Failed to open a new Google Docs URL: ", err)
	}
	docWebArea := nodewith.NameContaining(docsTab).Role(role.RootWebArea).First()
	canvas := nodewith.Role(role.Canvas).Ancestor(docWebArea).First()
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("type text to Google docs",
		ui.WaitUntilExists(canvas),
		kb.TypeAction(docText))(ctx); err != nil {
		s.Fatal("Failed to write to a Google doc: ", err)
	}

	uiHdl, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create clamshell action handler: ", err)
	}

	// Open Google slides in a new browser tab.
	_, err = identitycuj.OpenNewURL(ctx, cr, cuj.NewGoogleSlidesURL)
	if err != nil {
		s.Fatal("Failed to open a new Google Slides URL: ", err)
	}
	slidesWebArea := nodewith.NameContaining(slidesTab).Role(role.RootWebArea)
	title := nodewith.Name("title").Role(role.StaticText).Ancestor(slidesWebArea)
	if err := uiauto.Combine("open a new presentation",
		uiHdl.Click(title),
		kb.TypeAction(slidesTitleText))(ctx); err != nil {
		s.Fatal("Failed to write to a Google slides: ", err)
	}
}
