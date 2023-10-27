// Copyright 2021 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GuestPlayStore,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check PlayStore is Off in Guest mode",
		Contacts:     []string{"cros-arc-te@google.com", "arc-core@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInGuest",
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func GuestPlayStore(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	s.Log("Verify None of Default ARC Apps are Installed")
	installedApps, err := ash.ChromeApps(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get installed apps: ", err)
	}
	for _, app := range []apps.App{apps.PlayStore, apps.Duo, apps.PlayBooks, apps.PlayGames, apps.GoogleTV, apps.Clock, apps.Contacts} {
		for _, installedapp := range installedApps {
			if app.ID == installedapp.AppID {
				s.Fatalf("%s (%s) App is installed", app.Name, app.ID)
			}
		}
	}
}
