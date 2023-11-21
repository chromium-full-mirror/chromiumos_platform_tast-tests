// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevicePowerwashAllowed,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test the DevicePowerwashAllowed policy",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"artyomchen@google.com", // Test author
		},
		BugComponent: "b:1111617", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Policy Stack
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		Params: []testing.Param{
			{
				Name: "device_powerwash_allowed_true",
				Val:  true,
			},
			{
				Name: "device_powerwash_allowed_false",
				Val:  false,
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DevicePowerwashAllowed{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// As an admin, I want to disallow a factory reset by device
				// users, so I verify that the affected devices are not able
				// to trigger powerwash via settings or key combination.
				// COM_FOUND_CUJ34_TASK3_WF1
				Value: "screenplay-f24882e3-7c5b-44fa-a55f-9e2e03b8cd9a",
			},
		},
		VarDeps: []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

func DevicePowerwashAllowed(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	policyValue := s.Param().(bool)
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr, err := chrome.New(
		ctx,
		chrome.DMSPolicy(fdms.URL),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.KeepState(),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "add_profile_account_picker")

	policies := []policy.Policy{&policy.DevicePowerwashAllowed{Val: policyValue}}
	if err := policyutil.ServeAndVerifyOnLoginScreen(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)
	if err := kb.Accel(ctx, "Ctrl+Shift+Alt+R"); err != nil {
		s.Fatal("Failed to initiate powerwash via hotkeys: ", err)
	}

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	// TODO(b/312727434): Currently there is no testAPI to verify that
	// the ResetScreen shows up after pressing powerwash hotkeys.
	// Relying on "Browse as Guest" button on the LoginScreen,
	// since the ResetScreen doesn't have this button.
	browseAsGuestNode := nodewith.Name("Browse as Guest").First()
	uiError := ui.WaitUntilExists(browseAsGuestNode)(ctx)
	if policyValue && uiError == nil {
		s.Fatal("Still on login screen after powerwash initiation when powerwash is allowed")
	} else if !policyValue && uiError != nil {
		s.Fatal("Not on login screen after powerwash initiation when powerwash is disallowed: ", uiError)
	}
}
