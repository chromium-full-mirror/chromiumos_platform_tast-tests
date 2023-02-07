// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/optin"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/familylink"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornParentPermission,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if App Install Triggers Parent Permission For Unicorn Account",
		Contacts:     []string{"arc-commercial@google.com", "cros-arc-te@google.com", "cpiao@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline", "informational", "group:arc-functional", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      4 * time.Minute,
		VarDeps:      []string{"arc.parentUser"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Fixture: "familyLinkUnicornArcPolicyLogin",
	})
}

func UnicornParentPermission(ctx context.Context, s *testing.State) {
	const (
		askinMessageButtonText = "Ask in a message"
		askinPersonButtonText  = "Ask in person"
		installButtonText      = "install"
		playStoreSearchText    = "Search for apps & games"
		appPkgName             = "com.instagram.android"
	)
	parentUser := s.RequiredVar("arc.parentUser")
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	st, err := arc.GetState(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get ARC state: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	if st.Provisioned {
		s.Log("ARC is already provisioned. Skipping the Play Store setup")
		if err := apps.Close(ctx, tconn, apps.PlayStore.ID); err != nil {
			s.Fatal("Failed to close the provisioned Play Store: ", err)
		}
	} else {
		// Optin to Play Store.
		s.Log("Opting into Play Store")
		if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
			s.Fatal("Failed to optin to Play Store and Close: ", err)
		}
	}

	// Setup ARC.
	a, err := arc.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.DumpUIHierarchyOnError(cleanupCtx, s.OutDir(), s.HasError)
	defer a.Close(cleanupCtx)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)

	// Start screen recording for easier to debug failures.
	screenRecorder, err := uiauto.NewScreenRecorder(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create ScreenRecorder: ", err)
	}
	if screenRecorder == nil {
		s.Fatal("Screen recorder was not found")
	}
	if err := screenRecorder.Start(ctx, tconn); err != nil {
		s.Fatal("Failed to start screen recorder: ", err)
	}

	defer uiauto.ScreenRecorderStopSaveRelease(cleanupCtx, screenRecorder, filepath.Join(s.OutDir(), "recording.mp4"))

	if err := playstore.OpenAppPage(ctx, a, appPkgName); err != nil {
		s.Fatal("Failed to open the app page in Play Store: ", err)
	}

	installButton := d.Object(ui.ClassName("android.widget.Button"), ui.TextMatches("(?i)"+installButtonText), ui.Enabled(true))
	if err := installButton.WaitForExists(ctx, 30*time.Second); err != nil {
		s.Fatal("Install Button Exisits: ", err)
	}

	askinPersonButton := d.Object(ui.ClassName("android.widget.Button"), ui.Text(askinPersonButtonText), ui.Enabled(true))

	if err := uiauto.Retry(3, func(ctx context.Context) error {
		if err := installButton.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click installButton")
		}

		// Verify Parent Permission Dialog is displayed.
		if err := askinPersonButton.WaitForExists(ctx, 90*time.Second); err != nil {
			return errors.Wrap(err, "Ask in person button doesn't exist")
		}

		return nil
	})(ctx); err != nil {
		s.Fatal("Failed to click installButton and check the existence of Ask in person button: ", err)
	}

	if err := d.Object(ui.TextMatches(askinMessageButtonText)).Exists(ctx); err != nil {
		s.Fatal("Ask in a message button doesn't exist: ", err)
	}

	if err = askinPersonButton.Click(ctx); err != nil {
		s.Fatal("Failed to click  Ask in person: ", err)
	}

	parentPwd := d.Object(ui.ClassName("android.widget.EditText"), ui.Text(parentUser))
	if err := parentPwd.WaitForExists(ctx, 90*time.Second); err != nil {
		s.Fatal("parentPwd doesn't Exists: ", err)
	}

}
