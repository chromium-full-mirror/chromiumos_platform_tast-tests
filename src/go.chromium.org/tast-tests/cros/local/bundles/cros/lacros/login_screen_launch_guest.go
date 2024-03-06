// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/mountns"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LoginScreenLaunchGuest,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that Lacros prelaunching does not prevent OS guest mode from loading rootfs Lacros, even if prelaunching mounted Lacros outside the guest namespace",
		Contacts: []string{
			"lacros-tast@google.com",
			"neis@chromium.org",
		},
		BugComponent: "b:1456869",
		SoftwareDeps: []string{"chrome", "lacros"},
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      3 * (chrome.LoginTimeout + time.Minute),
	})
}

func LoginScreenLaunchGuest(ctx context.Context, s *testing.State) {
	// Restart Chrome with Lacros disabled to clean up any Lacros leftovers
	// from the previous test.
	func() {
		cr, err := chrome.New(ctx)
		if err != nil {
			s.Fatal("Failed to restart Chrome: ", err)
		}
		defer cr.Close(ctx)
	}()

	// The first session will start with /run/lacros mounted in the init
	// namespace but not in the session namespace.
	startGuestSessionAndUseLacros(ctx, s)
	// The second session will start with /run/lacros mounted in both.
	startGuestSessionAndUseLacros(ctx, s)
}

func startGuestSessionAndUseLacros(ctx context.Context, s *testing.State) {
	options := []chrome.Option{
		chrome.GuestLogin(),
		// The next two options together cause Lacros to be prelaunched.
		chrome.KeepState(),
		chrome.ExtraArgs("--force-lacros-launch-at-login-screen-for-testing"),
	}
	cr, err := browserfixt.NewChrome(ctx, browser.TypeLacros, lacrosfixt.NewConfig(), options...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	useLacros := func(ctx context.Context) error {
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to test API")
		}
		l, err := lacros.Launch(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to launch Lacros")
		}
		defer l.Close(ctx)
		return nil
	}
	if err := mountns.WithUserSessionMountNS(ctx, useLacros); err != nil {
		s.Fatal("Failed to use Lacros: ", err)
	}
}
