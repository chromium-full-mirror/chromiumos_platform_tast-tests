// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnableArc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify PlayStore can be turned On from Settings ",
		Contacts:     []string{"cros-arc-te@google.com", "arc-core@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:arc", "arc_core", "group:arc-functional", "arc_chromeos_vm"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "familyLinkParentArcLogin",
		}, {
			Name:              "unicorn",
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "familyLinkUnicornArcLogin",
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "familyLinkParentArcLogin",
		}, {
			Name:              "unicorn_vm",
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "familyLinkUnicornArcLogin",
		}},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 120*time.Second,
	})
}

func EnableArc(ctx context.Context, s *testing.State) {

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	if err := optin.SetPlayStoreEnabled(ctx, tconn, false); err != nil {
		s.Fatal("Failed to set Play Store off prior to test: ", err)
	}

	s.Log("Turn On Play Store from Settings")
	if err := turnOnPlayStore(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to Turn On Play Store: ", err)
	}

	s.Log("Verify Play Store is Enabled")
	playStoreState, err := optin.GetPlayStoreState(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to check GooglePlayStore State: ", err)
	}
	if playStoreState["enabled"] == false {
		s.Fatal("Playstore Disabled ")
	}

}

func turnOnPlayStore(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {

	ui := uiauto.New(tconn)
	playStoreButton := nodewith.Name("Google Play Store").Role(role.Button)
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "apps", ui.Exists(playStoreButton)); err != nil {
		return errors.Wrap(err, "failed to launch apps settings page")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return uiauto.Combine("enable Play Store",
			ui.FocusAndWait(playStoreButton),
			ui.LeftClick(playStoreButton),
			ui.LeftClick(nodewith.Name("More").Role(role.Button)),
			ui.LeftClick(nodewith.Name("Accept").Role(role.Button)),
		)(ctx)
	}, &testing.PollOptions{Timeout: 90 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to enable Play Store")
	}

	if err := optin.WaitForPlayStoreReady(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait for Play Store to be ready")
	}

	return nil

}
