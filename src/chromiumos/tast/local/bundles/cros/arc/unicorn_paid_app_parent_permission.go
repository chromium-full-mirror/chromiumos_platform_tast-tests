// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/optin"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/familylink"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornPaidAppParentPermission,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if paid app installation triggers Parent Permission For Unicorn Account",
		Contacts:     []string{"arc-commercial@google.com", "cros-arc-te@google.com", "cpiao@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline", "informational", "group:arc-functional", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      4 * time.Minute,
		Vars:         []string{"arc.parentUser"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Fixture: "familyLinkUnicornArcPolicyLogin",
	})
}

func UnicornPaidAppParentPermission(ctx context.Context, s *testing.State) {
	const (
		askYourParentDialogText = "Ask your parent"
		gamesAppName            = "org.twisevictory.apps"
	)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	st, err := arc.GetState(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get ARC state: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	if st.Provisioned {
		s.Log("ARC is already provisioned. Skipping the Play Store setup")
		if err := optin.ClosePlayStore(ctx, tconn); err != nil {
			s.Fatal("Failed to close the provisioned Play Store: ", err)
		}
	} else {
		// Optin to Play Store.
		s.Log("Opting into Play Store")
		if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
			s.Fatal("Failed to optin to Play Store and Close: ", err)
		}
	}

	a, err := arc.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)
	defer a.DumpUIHierarchyOnError(cleanupCtx, s.OutDir(), s.HasError)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(ctx)

	if err := playstore.OpenAppPage(ctx, a, gamesAppName); err != nil {
		s.Fatal("Failed to open app page: ", err)
	}

	searchResult := d.Object(ui.ClassName("android.view.View"), ui.DescriptionContains("$"), ui.Index(1))
	if err := searchResult.WaitForExists(ctx, 30*time.Second); err != nil {
		s.Log("Search Result doesn't exist: ", err)
	} else if err := searchResult.Click(ctx); err != nil {
		s.Fatal("Failed to click on Search Result: ", err)
	}

	installButton := d.Object(ui.ClassName("android.widget.Button"), ui.TextContains("$"), ui.Enabled(true))
	if err := installButton.WaitForExists(ctx, 10*time.Second); err != nil {
		s.Fatal("Install Button doesn't exisit: ", err)
	}
	if err := installButton.Click(ctx); err != nil {
		s.Fatal("Failed to click  installButton: ", err)
	}

	askinPersonButton := d.Object(ui.ClassName("android.widget.TextView"), ui.Text(askYourParentDialogText), ui.Enabled(true))
	if err := askinPersonButton.WaitForExists(ctx, 10*time.Second); err != nil {
		s.Fatal("Ask parent dialog doesn't Exists: ", err)
	}
}
