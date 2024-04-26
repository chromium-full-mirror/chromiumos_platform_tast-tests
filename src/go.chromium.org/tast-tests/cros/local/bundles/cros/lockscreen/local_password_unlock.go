// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lockscreen

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalPasswordUnlock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that local password unlock works for ChromeOS",
		Contacts: []string{
			"cros-lurs@google.com",
			"emaamari@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		Timeout:      2*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + 2*time.Minute,
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
	})
}

// LocalPasswordUnlock tests if we can unlock the device with a local password.
func LocalPasswordUnlock(ctx context.Context, s *testing.State) {
	const (
		username      = "testuser@gmail.com"
		localPassword = "testpassword"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr, err := login.SetupUserWithLocalPassword(ctx,
		localPassword,
		// We don't need to provide Gaia password, as the local password will be
		// used instead.
		chrome.FakeLogin(chrome.Creds{User: username, Pass: ""}),
	)
	defer userutil.ResetUsers(cleanupCtx)
	if err != nil {
		s.Fatal("Failed to setup user: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Getting test API connection failed: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

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
	defer keyboard.Close(cleanupCtx)

	// Enter and submit the local password to unlock the DUT.
	s.Log("Entering password to unlock")
	if err := lockscreen.EnterPassword(ctx, tconn, username, localPassword, keyboard); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return !st.Locked }, 30*time.Second); err != nil {
		s.Fatalf("Waiting for screen to be unlocked failed: %v (last status %+v)", err, st)
	}
}
