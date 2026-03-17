// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

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
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LaunchIWA,
		Desc: "Checks installing and auto launching the Kitchen Sink IWA in Kiosk mode and sending messages via direct sockets",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"sbykov@google.com",
			"mohamedaomar@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial > Kiosk
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Attr: []string{
			"group:golden_tier_secondary",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
			"group:release-health",
			"release-health_enterprise",
		},
		Timeout: 2*chrome.LoginTimeout + 2*time.Minute,
		Fixture: fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceLocalAccounts{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// LaunchIWA launches Kitchen Sink IWA in Kiosk mode and sends messages via direct sockets.
func LaunchIWA(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	var updateManifestURL string = "https://github.com/chromeos/iwa-sink/releases/latest/download/update.json"
	var webBundleID string = "aiv4bxauvcu3zvbu6r5yynoh4atkzqqaoeof5mwz54b4zfywcrjuoaacai"
	iwaKioskAccountType := policy.AccountTypeKioskIWA

	testing.ContextLog(ctx, "Starting Chrome in Kiosk mode")
	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		kioskmode.CustomLocalAccounts(
			&policy.DeviceLocalAccounts{
				Val: []policy.DeviceLocalAccountInfo{
					{
						AccountID:   &kioskmode.KioskAppAccountID,
						AccountType: &iwaKioskAccountType,
						IsolatedWebAppKioskInfo: &policy.IsolatedWebAppKioskInfo{
							WebBundleId: &webBundleID,
							ManifestUrl: &updateManifestURL,
						},
					},
				},
			},
		),
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupContext)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupContext, s.OutDir(), s.HasError, cr, "ui_tree_dump")

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Setup Chrome Test API Connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	ui := uiauto.New(tconn)
	createSocketConnButton := nodewith.Name("Create new socket connection").Role(role.Button)
	sendMessageTextField := nodewith.Name("Send a message").Role(role.TextField)
	sendButton := nodewith.Name("Send").Role(role.Button)

	if err := uiauto.Combine("Interact with Kitchen Sink IWA UI",
		// Create a new socket connection.
		ui.LeftClick(createSocketConnButton),
		ui.WaitUntilExists(sendMessageTextField.Nth(1)),
		// Send messages to the TCP Server.
		ui.LeftClickUntil(sendMessageTextField.First(), ui.Exists(sendMessageTextField.Focused())),
		kb.TypeAction("Na?"),
		ui.LeftClick(sendButton.First()),
		ui.WaitUntilExists(nodewith.NameContaining("Na?").First()),

		// Send a message from the TCP Server.
		ui.LeftClickUntil(sendMessageTextField.Nth(1), ui.Exists(sendMessageTextField.Focused())),
		kb.TypeAction("na ja!"),
		ui.LeftClick(sendButton.Nth(1)),
		ui.WaitUntilExists(nodewith.NameContaining("na ja!").First()),
	)(ctx); err != nil {
		s.Fatal("Failed to interact with the Kitchen Sink IWA: ", err)
	}
}
