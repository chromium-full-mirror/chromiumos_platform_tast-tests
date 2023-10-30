// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefaultAppLaunchWhenArcIsOff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify Default App Icons Launch Opt In Flow When PlayStore is Off ",
		Contacts:     []string{"cros-arc-te@google.com", "arc-core@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:arc", "arc_core", "group:arc-functional"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 120*time.Second,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

func DefaultAppLaunchWhenArcIsOff(ctx context.Context, s *testing.State) {
	const (
		defaultTimeout = 20 * time.Second
	)

	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Launch Play Books App.
	if err := launcher.SearchAndWaitForAppOpen(tconn, kb, apps.PlayBooks)(ctx); err != nil {
		s.Log("Failed to Launch the Play Books: ", err)
	}

	ui := uiauto.New(tconn)
	more := nodewith.Name("More").Role(role.StaticText)
	accept := nodewith.Name("Accept").Role(role.Button)

	if err := uiauto.Combine("verify optin flow launch",
		ui.WaitUntilExists(more),
		ui.LeftClick(more),
		ui.WaitUntilExists(accept),
		ui.LeftClick(accept),
	)(ctx); err != nil {
		s.Fatal("Failed to launch optin flow: ", err)
	}

	// Verify Play Store is Enabled.
	if err = optin.WaitForPlayStoreReady(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for Play Store to be ready: ", err)
	}

}
