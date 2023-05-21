// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/optin"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/familylink"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/retry"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornParentPermission,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if App Install Triggers Parent Permission For Unicorn Account",
		Contacts:     []string{"arc-commercial@google.com", "cros-arc-te@google.com", "cpiao@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      4 * time.Minute,
		VarDeps:      []string{"arc.parentUser"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"informational"},
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

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Fatalf:      s.Fatalf,
		Logf:        s.Logf}

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {

		st, err := arc.GetState(ctx, tconn)
		if err != nil {
			rl.Exit("get ARC state", err)
		}
		defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

		if st.Provisioned {
			s.Log("ARC is already provisioned. Skipping the Play Store setup")
			if err := apps.Close(ctx, tconn, apps.PlayStore.ID); err != nil {
				rl.Exit("close the provisioned Play Store", err)
			}
		} else {
			s.Log("Opting into Play Store")
			if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
				return rl.Retry("optin to Play Store and Close", err)
			}
		}

		a, err := arc.New(ctx, s.OutDir())
		if err != nil {
			return rl.Retry("start ARC", err)
		}
		defer a.DumpUIHierarchyOnError(cleanupCtx, s.OutDir(), s.HasError)
		defer a.Close(cleanupCtx)

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Retry("initializing UI Automator", err)
		}
		defer d.Close(cleanupCtx)

		// Start screen recording for easier to debug failures.
		screenRecorder, err := uiauto.NewScreenRecorder(ctx, tconn)
		if err != nil || screenRecorder == nil {
			rl.Exit("create ScreenRecorder", err)
		}
		if err := screenRecorder.Start(ctx, tconn); err != nil {
			rl.Exit("start screen recorder", err)
		}

		defer uiauto.ScreenRecorderStopSaveRelease(cleanupCtx, screenRecorder, filepath.Join(s.OutDir(), "recording.mp4"))

		if err := playstore.OpenAppPage(ctx, a, appPkgName); err != nil {
			rl.Exit("open the app page in Play Store", err)
		}

		installButton, err := playstore.FindInstallButton(ctx, d, 15*time.Second)
		if err != nil {
			rl.Exit("find the install button", err)
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
			rl.Exit("click installButton and check the existence of Ask in person button", err)
		}

		if err := d.Object(ui.TextMatches(askinMessageButtonText)).Exists(ctx); err != nil {
			rl.Exit("find Ask in a message button", err)
		}

		if err = askinPersonButton.Click(ctx); err != nil {
			rl.Exit("click Ask in person button", err)
		}

		parentPwd := d.Object(ui.ClassName("android.widget.EditText"), ui.Text(parentUser))
		if err := parentPwd.WaitForExists(ctx, 90*time.Second); err != nil {
			rl.Exit("find the parentPwd element", err)
		}
		return nil
	}, nil); err != nil {
		s.Fatal("Unicorn parent permission test failed: ", err)
	}

}
