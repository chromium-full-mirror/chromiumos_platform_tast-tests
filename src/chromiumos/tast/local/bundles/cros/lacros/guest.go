// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/mountns"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Guest,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests launching rootfs Lacros in a guest session",
		Contacts: []string{
			"lacros-team@google.com",
			"neis@chromium.org",
		},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Timeout:      3 * time.Minute,
	})
}

func Guest(ctx context.Context, s *testing.State) {
	// Restart Chrome with Lacros disabled to clean up any Lacros leftovers
	// from the previous test.
	func() {
		cr, err := chrome.New(ctx)
		if err != nil {
			s.Fatal("Failed to restart Chrome: ", err)
		}
		defer cr.Close(ctx)
	}()

	// Start guest session with rootfs Lacros.
	cr, err := browserfixt.NewChrome(ctx, browser.TypeLacros, lacrosfixt.NewConfig(), chrome.GuestLogin())
	if err != nil {
		s.Fatal("Failed to restart Chrome: ", err)
	}
	defer cr.Close(ctx)

	// Check that Lacros can be used.
	// We need to run these steps in the guest session mount namespace. See
	// b/244513681.
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
