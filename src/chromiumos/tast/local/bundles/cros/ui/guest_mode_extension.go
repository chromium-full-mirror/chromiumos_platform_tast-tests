// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/mountns"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GuestModeExtension,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check Tast extension can be loaded in Guest mode",
		Contacts:     []string{"benreich@chromium.org", "chromeos-engprod-syd@google.com"},
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			Fixture: "chromeLoggedInGuest",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           "lacrosGuest",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

func GuestModeExtension(ctx context.Context, s *testing.State) {
	// In order for the "guest_lacros" case to work correctly, we need to
	// run the test body in the user mount namespace. See b/244513681.
	if err := mountns.WithUserSessionMountNS(ctx, func(ctx context.Context) error {
		guestModeExtension(ctx, s)
		return nil
	}); err != nil {
		s.Fatal("Failed to run test in correct mount namespace: ", err)
	}
}

func guestModeExtension(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	if _, err := cr.TestAPIConn(ctx); err != nil {
		s.Fatal("Failed to create Test API Connection: ", err)
	}
}
