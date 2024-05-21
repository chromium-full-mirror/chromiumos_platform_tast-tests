// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arc supports interacting with the ARC framework, which is used to run Android applications on Chrome OS.
package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayStoreDefaultHandler,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies Play Store is marked as preferred upon opt-in",
		Contacts: []string{
			"cros-arc-te@google.com",
			"arc-core@google.com",
			"djacobo@google.com",
		},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:arc", "arc_core", "group:arc-functional"},
		SoftwareDeps: []string{"chrome", "android_vm"},
		Fixture:      "arcBootedWithPlayStore",
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 120*time.Second,
	})
}

// PlayStoreDefaultHandler checks that Play store is marked as default handler via App Management on cros settings.
func PlayStoreDefaultHandler(ctx context.Context, s *testing.State) {
	const (
		testAppID       = "cnbgggchhmkkdmeppjobngjoejnihlei"
		testAppName     = "Play Store"
		testPackageName = "com.android.vending"
	)

	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	appHeader := nodewith.Name(testAppName).Role(role.Heading).Ancestor(ossettings.WindowFinder)
	osSettings, err := ossettings.LaunchAtAppMgmtPage(ctx, tconn, cr, testAppID, ui.Exists(appHeader))
	if err != nil {
		s.Fatal("Failed to open OS Settings: ", err)
	}

	defer osSettings.Close(cleanupCtx)

	categoryHeader := nodewith.Name("Opening supported links").Role(role.Link)
	prefButton := nodewith.Name("Open in Play Store app").Role(role.RadioButton)

	if err := uiauto.Combine("check opening supported links selection",
		osSettings.Exists(categoryHeader),
		osSettings.Exists(prefButton),
		osSettings.WaitUntilCheckedState(prefButton, true),
	)(ctx); err != nil {
		s.Fatal("Failed to check pref button: ", err)
	}
}
