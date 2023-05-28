// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrimaryLaunch,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Lacros is launchable when a pre-existing user enables LacrosPrimary",
		Contacts: []string{
			"lacros-team@google.com",
			"ythjkt@google.com", // Test author
			"neis@google.com",
			"hidehiko@google.com",
		},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "lacros"},
	})
}

func PrimaryLaunch(ctx context.Context, s *testing.State) {
	err := loginToCreateProfileDirectory(ctx)
	if err != nil {
		s.Fatal("Failed to run Chrome to create a new profile directory: ", err)
	}

	// Pass `KeepState` so that when calling `chrome.New()`, login happens to an existing profile.
	chromeOpts := []chrome.Option{
		chrome.KeepState(),
	}
	opts := []lacrosfixt.Option{
		lacrosfixt.Mode(lacros.LacrosPrimary),
		lacrosfixt.ChromeOptions(chromeOpts...)}
	chromeOpts, err = lacrosfixt.NewConfig(opts...).Opts()
	if err != nil {
		s.Fatal("Failed to get lacrosfixt.NewConfig: ", err)
	}
	// Note that `chrome.New()` will restart session_manager, effectively logging out of the user and then logs back into user session.
	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Failed to launch Ash Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Lacros: ", err)
	}
	l.Close(ctx)
}

func loginToCreateProfileDirectory(ctx context.Context) error {
	cr, err := chrome.New(ctx, chrome.DisableFeatures("LacrosSupport"))

	if err != nil {
		return errors.Wrap(err, "failed to launch Chrome with LacrosSupport disabled")
	}

	return cr.Close(ctx)
}
