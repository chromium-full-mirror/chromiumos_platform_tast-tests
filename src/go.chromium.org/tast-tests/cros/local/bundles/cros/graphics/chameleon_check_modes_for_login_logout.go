// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
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
		Fixture:      "gpuWatchHangs",
		Params: []testing.Param{{
			Name: "extended_dp1",
			Val: graphics.ChameleonTest{
				Port:    "dp1",
				Display: graphics.UIExtended,
			},
		}, {
			Name: "extended_dp2",
			Val: graphics.ChameleonTest{
				Port:    "dp2",
				Display: graphics.UIExtended,
			},
		}, {
			Name: "extended_hdmi1",
			Val: graphics.ChameleonTest{
				Port:    "hdmi1",
				Display: graphics.UIExtended,
			},
		}, {
			Name: "extended_hdmi2",
			Val: graphics.ChameleonTest{
				Port:    "hdmi2",
				Display: graphics.UIExtended,
			},
		}, {
			Name: "mirror_dp1",
			Val: graphics.ChameleonTest{
				Port:    "dp1",
				Display: graphics.UIMirror,
			},
		}, {
			Name: "mirror_dp2",
			Val: graphics.ChameleonTest{
				Port:    "dp2",
				Display: graphics.UIMirror,
			},
		}, {
			Name: "mirror_hdmi1",
			Val: graphics.ChameleonTest{
				Port:    "hdmi1",
				Display: graphics.UIMirror,
			},
		}, {
			Name: "mirror_hdmi2",
			Val: graphics.ChameleonTest{
				Port:    "hdmi2",
				Display: graphics.UIMirror,
			},
		}},
		Timeout: chrome.LoginTimeout + time.Minute,
	})
}

func ChameleonCheckModesForLoginLogout(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.ChameleonTest)
	portStr := testOpt.Port

	cham, err := graphics.ChameleonGetConnection(ctx)

	defer graphics.ChameleonSaveLogsAndOutputOnError(ctx, cham, s.HasError, s.OutDir())

	if err != nil {
		s.Fatal("Failed to get the Chameleond instance: ", err)
	}

	connectedPortMap := graphics.ChameleonGetConnectedPortMap(ctx)
	isSupposedConnected := connectedPortMap[graphics.ChameleonGetPortname(portStr)]

	if !isSupposedConnected {
		s.Logf("Port %s (%s) is not connected, won't test", graphics.ChameleonGetPortname(portStr), portStr)
		return
	}

	shouldUsePort, port, err := graphics.ChameleonShouldUsePort(ctx, cham, portStr)
	if err != nil {
		s.Fatalf("Failed to determine if plug can be used for port %s: %s", portStr, err)
	}
	if !shouldUsePort {
		s.Fatalf("Chameleon is not plugged into port %d", port)
	}

	err = graphics.ChameleonPlug(ctx, cham, port)
	if err != nil {
		s.Fatalf("Failed to get stable video input from a physically plugged port %d: %s", port, err)
	}
	defer func(ctx context.Context, port chameleon.PortID) {
		err = cham.Unplug(ctx, port)
		if err != nil {
			s.Fatalf("Failed to unplug a physically plugged port %d: %s ", port, err)
		}
	}(ctx, port)

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

	// Sets Display Mode
	var isMirrorMode bool
	if testOpt.Display == graphics.UIMirror {
		isMirrorMode = true
	} else if testOpt.Display == graphics.UIExtended {
		isMirrorMode = false
	} else {
		s.Fatal("Failed to set either mirror or extended mode")
	}
	err = graphics.SwitchDisplayMode(ctx, cr, isMirrorMode)
	if err != nil {
		s.Fatal("Failed to switch modes: ", err)
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
}
