// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"os"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	applauncher "go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppLauncherLaunch,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests launching lacros from the App Launcher",
		Contacts:     []string{"lacros-team@google.com", "chromeos-sw-engprod@google.com"},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"lacros_stable"},
			Fixture:           "lacros",
		}, {
			Name:              "unstable",
			ExtraSoftwareDeps: []string{"lacros_unstable"},
			ExtraAttr:         []string{"informational"},
			Fixture:           "lacros",
		}, {
			Name:              "enrolled",
			ExtraSoftwareDeps: []string{"lacros_stable"},
			ExtraAttr:         []string{"informational"},
			Fixture:           fixture.LacrosEnrolledLoggedIn,
		}, {
			Name:              "enrolled_unstable",
			ExtraSoftwareDeps: []string{"lacros_unstable"},
			ExtraAttr:         []string{"informational"},
			Fixture:           fixture.LacrosEnrolledLoggedIn,
		}},
	})
}

func AppLauncherLaunch(ctx context.Context, s *testing.State) {
	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Clean up user data dir to ensure a clean start.
	os.RemoveAll(lacros.UserDataDir)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)
	if err := applauncher.SearchAndLaunch(tconn, kb, apps.Lacros.Name)(ctx); err != nil {
		s.Fatal("Failed to search and launch Lacros app: ", err)
	}

	s.Log("Checking that Lacros window is visible")
	if err := lacros.WaitForLacrosWindow(ctx, tconn, "New Tab"); err != nil {
		s.Fatal("Failed waiting for Lacros window to be visible: ", err)
	}

	l, err := lacros.Connect(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to connect to lacros-chrome: ", err)
	}
	defer l.Close(ctx)
}
