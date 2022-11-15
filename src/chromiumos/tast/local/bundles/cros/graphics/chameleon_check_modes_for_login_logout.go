// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChameleonCheckModesForLoginLogout,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "To check the display mode is preserved after sign out and signin",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		BugComponent: "b:188154", // ChromeOS > Platform > Graphics > Display
		Attr: []string{
			"group:graphics",
			"graphics_chameleon_igt",
			"graphics_nightly",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"graphics.chameleon_ip"},
		Params: []testing.Param{{
			Name: "port0",
			Val: graphics.ChameleonTest{
				Port: 0,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "port1",
			Val: graphics.ChameleonTest{
				Port: 1,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "port2",
			Val: graphics.ChameleonTest{
				Port: 2,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "port3",
			Val: graphics.ChameleonTest{
				Port: 3,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}},
	})
}

func ChameleonCheckModesForLoginLogout(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.ChameleonTest)
	port := testOpt.Port

	cham, err := graphics.ChameleonGetConnection(ctx)
	if err != nil {
		s.Fatal("Failed to get the Chameleond instance: ", err)
	}

	// TODO(b:260352485): Avoid hardcoding the port ids as they may change.
	shouldUsePort, err := graphics.ChameleonShouldUsePort(ctx, cham, port)
	if err != nil {
		s.Fatalf("Failed to determine if plug can be used for port %d: %s", port, err)
	}
	if !shouldUsePort {
		s.Logf("Chameleon is not plugged into port %d", port)
		return
	}

	err = graphics.ChameleonPlug(ctx, cham, port)
	if err != nil {
		s.Fatalf("Failed to get stable video input from a physically plugged port %d: %s", port, err)
	}

	// We explicitly want a Chrome that is waiting on the login screen.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.DeferLogin())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	testing.Sleep(ctx, time.Second)
	preLoginWidth, preLoginHeight, err := cham.DetectResolution(ctx, port)
	if err != nil {
		s.Fatal("Failed to detect prelogin resolution: ", err)
	}
	s.Logf("Before login, port %d size = %dx%d pixels", port, preLoginWidth, preLoginHeight)

	// Log in to Chrome and switch to external display mode.
	err = cr.ContinueLogin(ctx)
	if err != nil {
		s.Fatal("Failed to login to Chrome: ", err)
	}
	_, err = cr.NewConn(ctx, "chrome://gpu")
	if err != nil {
		s.Fatal("Failed to load chrome://gpu: ", err)
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	err = graphics.SwitchDisplayMode(ctx, cr, false)
	if err != nil {
		s.Fatal("Failed to switch to extended mode: ", err)
	}

	postLoginWidth, postLoginHeight, err := cham.DetectResolution(ctx, port)
	if err != nil {
		s.Fatal("Failed to detect post login resolution: ", err)
	}
	s.Logf("After login, port %d size = %dx%d pixels", port, postLoginWidth, postLoginHeight)

	// Check chameleon resolution after login.
	if preLoginWidth != postLoginWidth || preLoginHeight != postLoginHeight {
		s.Fatalf("Port %d screen size changed from %dx%d to %dx%d after login", port, preLoginWidth, preLoginHeight, postLoginWidth, postLoginHeight)
	}

	// Log out of Chrome.
	if err = quicksettings.SignOut(ctx, tconn); err != nil {
		s.Fatal("Failed to logout: ", err)
	}
	testing.Sleep(ctx, time.Second)
	postLogoutWidth, postLogoutHeight, err := cham.DetectResolution(ctx, port)
	if err != nil {
		s.Fatal("Failed to detect resolution: ", err)
	}
	s.Logf("After logout, port %d size = %dx%d pixels", port, postLogoutWidth, postLogoutHeight)

	// Check chameleon resolution after logging out.
	if postLoginWidth != postLogoutWidth || postLoginHeight != postLogoutHeight {
		s.Fatalf("Port %d screen size changed from %dx%d to %dx%d after logout", port, postLoginWidth, postLoginHeight, postLogoutWidth, postLogoutHeight)
	}

	err = cham.Unplug(ctx, port)
	if err != nil {
		s.Fatalf("Failed to unplug a physically plugged port %d: %s ", port, err)
	}
}
