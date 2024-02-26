// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lockscreen

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PinUnlockFail,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that password field is shown after incorrect PIN was entered multiple times",
		Contacts: []string{
			"cros-lurs@google.com",
			"emaamari@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary"},
	})
}

func PinUnlockFail(ctx context.Context, s *testing.State) {
	const (
		Pin             = "1234567890"
		wrongPin        = "0123456789"
		lockoutAttempts = 5
	)

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Getting test API connection failed: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Set up PIN through a connection to the Settings page.
	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Settings app: ", err)
	}
	if err := settings.EnablePINUnlock(cr, chrome.DefaultPass, Pin, false)(ctx); err != nil {
		s.Fatal("Failed to enable PIN unlock: ", err)
	}

	// Lock the screen.
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
		s.Fatalf("Waiting for screen to be locked failed: %v (last status %+v)", err, st)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// Enter the wrong PIN to trigger password field. Here we would
	// try multiple times of the wrong password until pin pad
	// disappears or that we had tried |lockoutAttempts| times already.
	count := 0
	ui := uiauto.New(tconn)
	for count < lockoutAttempts {
		if err := lockscreen.WaitUntilPinPadExists(ctx, tconn); err != nil {
			s.Fatalf("Failed to find PIN pad after %v incorrect attempts: %v", count, err)
		}

		// Enter and submit the PIN to unlock the DUT.
		if err := lockscreen.EnterPIN(ctx, tconn, keyboard, wrongPin); err != nil {
			s.Fatal("Failed to enter PIN: ", err)
		}
		if err := lockscreen.SubmitPINOrPassword(ctx, tconn); err != nil {
			s.Fatal("Failed to submit PIN: ", err)
		}

		// Wait to see the Auth error.
		if count > 0 {
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(lockscreen.ConsecutiveAuthErrorFinder)(ctx); err != nil {
				s.Fatal("Failed to see the Auth error: ", err)
			}
		} else {
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(lockscreen.AuthErrorFinder)(ctx); err != nil {
				s.Fatal("Failed to see the Auth error: ", err)
			}
		}

		count++
	}
	if err := lockscreen.WaitUntilPinPadGone(ctx, tconn); err != nil {
		s.Fatal("Failed to see pin pad hidden: ", err)
	}

	if err := lockscreen.WaitForPasswordField(ctx, tconn, chrome.DefaultUser, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for password field: ", err)
	}
}
