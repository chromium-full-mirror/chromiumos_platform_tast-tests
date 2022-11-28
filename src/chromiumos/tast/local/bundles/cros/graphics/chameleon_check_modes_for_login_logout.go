// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/typecutils"
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
		BugComponent: "TBA",
		Attr: []string{
			"group:graphics",
			"graphics_chameleon_igt",
			"graphics_nightly",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"graphics.chameleon_ip"},
		Timeout:      chrome.LoginTimeout + time.Minute,
	})
}

func ChameleonCheckModesForLoginLogout(ctx context.Context, s *testing.State) {
	// Connect to Chameleon
	chamURL, err := graphics.ChameleonGetURL()
	if err != nil {
		s.Fatal("Failed to get the Chameleon's URL: ", err)
	}
	cham, err := chameleon.NewChameleond(ctx, chamURL)
	if err != nil {
		s.Fatal("Failed to connect to Chameleon: ", err)
	}
	s.Log("Connected to Chameleon")

	supportedPorts, err := cham.GetSupportedPorts(ctx)
	if err != nil {
		s.Fatal("Failed to get supported ports: ", err)
	}

	// TODO(b:260352485): Split iteration over all 4 ports into 4 parameterized tests
	for _, port := range supportedPorts {
		shouldUsePort, err := graphics.ChameleonShouldUsePort(ctx, cham, port)
		if err != nil {
			s.Fatalf("Failed to determine if plug can be used for port %d: %s", port, err)
		}
		if !shouldUsePort {
			continue
		}

		err = cham.Plug(ctx, port)
		if err != nil {
			s.Fatalf("Failed to plug in a physically plugged port %d: %s", port, err)
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
		kb, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to initialize keyboard input: ", err)
		}
		display := typecutils.SwitchWindowToDisplay(ctx, tconn, kb, true)
		err = display(ctx)
		if err != nil {
			s.Fatal("Failed to change display to external display mode: ", err)
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
}
