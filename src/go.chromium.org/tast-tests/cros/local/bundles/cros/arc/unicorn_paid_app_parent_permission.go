// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornPaidAppParentPermission,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if paid app installation triggers Parent Permission For Unicorn Account",
		Contacts:     []string{"arc-commercial@google.com", "cros-arc-te@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Vars:         []string{"arc.parentUser"},
		Params: []testing.Param{
			{
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
			},
			{
				Name:              "betty",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
			},
			{
				Name:              "vm",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
			},
			{
				Name:              "x",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
			},
			{
				Name:              "betty_vm",
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
			}},
		Fixture: "familyLinkUnicornArcPolicyLogin",
	})
}

func UnicornPaidAppParentPermission(ctx context.Context, s *testing.State) {
	const (
		provisioningTimeout     = 3 * time.Minute
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

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
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

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		s.Fatal("Failed to wait for provisioning: ", err)
	}

	if err := playstore.OpenAppPage(ctx, a, gamesAppName); err != nil {
		s.Fatal("Failed to open app page: ", err)
	}

	// The buy button shows price only when the app isn't purchased already.
	installButton, err := playstore.FindActionButton(ctx, d, "(Buy for )?\\$[0-9.]+", 30*time.Second)
	if err != nil {
		s.Fatal("Install Button doesn't exist: ", err)
	}

	if err := installButton.Click(ctx); err != nil {
		s.Fatal("Failed to click  installButton: ", err)
	}

	askinPersonButton := d.Object(ui.ClassName("android.widget.TextView"), ui.Text(askYourParentDialogText), ui.Enabled(true))
	if err := askinPersonButton.WaitForExists(ctx, 10*time.Second); err != nil {
		s.Fatal("Ask parent dialog doesn't Exists: ", err)
	}
}
