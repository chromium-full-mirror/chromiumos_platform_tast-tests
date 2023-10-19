// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyDefaultApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies Default arc apps are installed",
		Contacts:     []string{"cros-arc-te@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:arc", "arc_core", "group:arc-functional"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 2*time.Minute,
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithPlayStore",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

// VerifyDefaultApps checks that default ARC apps are available after boot.
// Some of these apps are installed through PAI and will only have "promise
// icon" stubs available on first boot. This PAI integration is tested
// separately by arc.PlayAutoInstall.
func VerifyDefaultApps(ctx context.Context, s *testing.State) {

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Lookup for ARC++ default apps
	apps := []apps.App{
		apps.PlayStore,
		apps.PlayBooks,
		apps.PlayGames,
		apps.GoogleTV,
		apps.Photos,
		apps.Clock,
		apps.Contacts,
	}

	for _, app := range apps {
		if err := ash.WaitForChromeAppInstalled(ctx, tconn, app.ID, ctxutil.MaxTimeout); err != nil {
			s.Errorf("Failed to wait for %s (%s) to be installed: %v", app.Name, app.ID, err)
		}
	}
}
