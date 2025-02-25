// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iwa

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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AutostartIWA,
		Desc: "Checks the autostart and prevent close behavior of the IWA Kitchen Sink",
		Contacts: []string{
			"iwa-team@google.com",
			"mohamedaomar@google.com",
		},
		BugComponent: "b:1168200", // Chrome > Isolated Web Apps
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Timeout:      2*chrome.LoginTimeout + 2*time.Minute,
		Fixture:      fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.IsolatedWebAppInstallForceList{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.WebAppSettings{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// AutostartIWA auto launches Kitchen Sink IWA after login and prevents it from closing.
func AutostartIWA(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Login into chrome with fake creds.
	crOpts := []chrome.Option{
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
	}
	cr, err := chrome.New(ctx, crOpts...)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}

	const (
		kitchenSinkIWAUpdateManifestURL = "https://github.com/chromeos/iwa-sink/releases/latest/download/update.json"
		kitchenSinkIWAWebBundleID       = "aiv4bxauvcu3zvbu6r5yynoh4atkzqqaoeof5mwz54b4zfywcrjuoaacai"
		manifestID                      = "isolated-app://aiv4bxauvcu3zvbu6r5yynoh4atkzqqaoeof5mwz54b4zfywcrjuoaacai"
	)

	pb := policy.NewBlob()
	policies := []policy.Policy{
		&policy.IsolatedWebAppInstallForceList{
			Val: []*policy.IsolatedWebAppInstallForceListValue{
				{
					UpdateManifestUrl: kitchenSinkIWAUpdateManifestURL,
					WebBundleId:       kitchenSinkIWAWebBundleID,
					PinnedVersion:     "0.17.0",
				},
			},
		},
		&policy.WebAppSettings{
			Val: []*policy.WebAppSettingsValue{
				{
					ManifestId:                    manifestID,
					PreventCloseAfterRunOnOsLogin: true,
					RunOnOsLogin:                  "run_windowed",
				},
			},
		},
	}
	if err := pb.AddPolicies(policies); err != nil {
		s.Fatal("Failed to add policies for public account setup: ", err)
	}
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}
	// GoBigSleepLint: Wait for policies to reach the DUT before restarting chrome.
	testing.Sleep(ctx, 5*time.Second)

	// Log out and back into chrome again, since it requires fresh OS login to auto launch the IWA.
	// Close the old chrome instance.
	if err := cr.Close(ctx); err != nil {
		s.Fatal("Failed to close chrome: ", err)
	}
	// Reopen a new chrome instance.
	cr, err = chrome.New(ctx, append(crOpts, chrome.KeepState())...)
	if err != nil {
		s.Fatal("Failed to restart chrome: ", err)
	}
	defer cr.Close(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupContext, s.OutDir(), s.HasError, cr, "ui_tree_dump")

	// Setup Chrome Test API Connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn)
	kitchenSinkIWAHeading := nodewith.Name("IWA Kitchen Sink").Role(role.Heading)
	disabledCloseButton := nodewith.Name("Your administrator doesn't allow closing this app").Role(role.Button).ClassName("FrameCaptionButton")

	if err := uiauto.Combine("Wait for IWA Kitchen Sink to launch automatically on start and ensure it doesn't close",
		// Wait till the IWA is launched and the screen capture button appears.
		ui.WithTimeout(30*time.Second).WaitUntilExists(kitchenSinkIWAHeading),
		// Make sure that the close button is disabled and the IWA cannot be closed.
		ui.WaitUntilExists(disabledCloseButton),
		// Attempt to close the IWA via the disabled close button.
		ui.LeftClick(disabledCloseButton),
		// Ensure that the IWA is still open and doesn't get closed.
		ui.EnsureExistsFor(kitchenSinkIWAHeading, 5*time.Second),
		// Attempt to close the IWA via Ctrl+F4.
		kb.AccelAction("Ctrl+F4"),
		// Ensure that the IWA is still open and doesn't get closed.
		ui.EnsureExistsFor(kitchenSinkIWAHeading, 5*time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed to auto launch Kitchen Sink IWA: ", err)
	}
}
