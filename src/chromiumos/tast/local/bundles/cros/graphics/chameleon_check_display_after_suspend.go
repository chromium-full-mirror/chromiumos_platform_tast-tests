// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChameleonCheckDisplayAfterSuspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "To check the UI display mode is preserved before and after suspend",
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

func ChameleonCheckDisplayAfterSuspend(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.ChameleonTest)
	portStr := testOpt.Port
	pixelDiffThreshold := 5000

	cham, err := graphics.ChameleonGetConnection(ctx)

	defer func(ctx context.Context, s *testing.State, cham chameleon.Chameleond) {
		graphics.ChameleonSaveLogsAndOutputOnError(ctx, cham, s.HasError(), s.OutDir())
	}(ctx, s, cham)

	if err != nil {
		s.Fatal("Failed to get the Chameleond instance: ", err)
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

	// Log in to Chrome.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 40*time.Second)
	defer cancel()
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	conn, err := cr.NewConn(ctx, "chrome://gpu")
	if err != nil {
		s.Fatal("Failed to load chrome://gpu: ", err)
	}
	defer conn.Close()

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

	// Check UI Resolution before suspending.
	preSuspendWidth, preSuspendHeight, err := cham.DetectResolution(ctx, port)
	if err != nil {
		s.Fatalf("Failed to detect resolution on port %d: %s ", port, err)
	}
	s.Logf("Before suspension, port %d size = %dx%d pixels", port, preSuspendWidth, preSuspendHeight)

	preSuspendDisplayPath := filepath.Join(s.OutDir(), "preSuspendDisplay.png")
	preSuspendChamPath := filepath.Join(s.OutDir(), "preSuspendCham.png")
	// TODO(b:263163784): Replace CaptureChrome and ChameleonGetScreenshot with screen-util-tools command.
	err = screenshot.CaptureChrome(ctx, cr, preSuspendDisplayPath)
	if err != nil {
		s.Fatal("Failed to screenshot eDP: ", err)
	}
	err = graphics.ChameleonGetScreenshot(ctx, cham, port, preSuspendChamPath)
	if err != nil {
		s.Fatal("Failed to get Chameleon screenshot: ", err)
	}

	s.Log("Start suspending the DUT")
	cmd := testexec.CommandContext(ctx, "powerd_dbus_suspend", "--delay=0", "--timeout=30", "--suspend_for_sec=10")
	err = cmd.Run()
	if err != nil {
		s.Fatal("The powerd_dbus_suspend failed to properly suspend: ", err)
	}
	testing.ContextLog(ctx, "DUT resumes from being suspended")

	s.Log("Reconnecting to Chrome")
	err = cr.Reconnect(ctx)
	if err != nil {
		s.Fatal("Failed to reconnect to Chrome: ", err)
	}
	testing.ContextLog(ctx, "Reconnecting to our browser tab")
	conn, err = cr.NewConnForTarget(ctx, chrome.MatchAllPages())
	if err != nil {
		s.Fatal("Failed to reconnect to browser tab")
	}

	postSuspendWidth, postSuspendHeight, err := cham.DetectResolution(ctx, port)
	if err != nil {
		s.Fatalf("Failed to detect resolution on port %d: %s ", port, err)
	}
	s.Logf("After suspension, port %d size = %dx%d pixels", port, postSuspendWidth, postSuspendHeight)

	postSuspendDisplayPath := filepath.Join(s.OutDir(), "postSuspendDisplay.png")
	postSuspendChamPath := filepath.Join(s.OutDir(), "postSuspendCham.png")
	// TODO(b:263163784): Replace CaptureChrome and ChameleonGetScreenshot with screen-util-tools command.
	err = screenshot.CaptureChrome(ctx, cr, postSuspendDisplayPath)
	if err != nil {
		s.Fatal("Failed to screenshot eDP: ", err)
	}
	err = graphics.ChameleonGetScreenshot(ctx, cham, port, postSuspendChamPath)
	if err != nil {
		s.Fatal("Failed to get Chameleon screenshot: ", err)
	}

	// Check chameleon resolution after suspending.
	if preSuspendWidth != postSuspendWidth || preSuspendHeight != postSuspendHeight {
		s.Errorf("Port %d screen size changed from %dx%d to %dx%d after suspension", port, preSuspendWidth, preSuspendHeight, postSuspendWidth, postSuspendHeight)
	}

	// We assume the UI has kept our about:gpu tab unchanged.
	isDisplaySame, err := graphics.ChameleonPerceptualDiff(ctx, preSuspendDisplayPath, postSuspendDisplayPath, s.OutDir(), pixelDiffThreshold)
	if err != nil {
		s.Fatal("Failed to compare internal display screenshots: ", err)
	}

	isChamSame, err := graphics.ChameleonPerceptualDiff(ctx, preSuspendChamPath, postSuspendChamPath, s.OutDir(), pixelDiffThreshold)
	if err != nil {
		s.Fatal("Failed to compare internal display screenshots: ", err)
	}

	if !isDisplaySame || !isChamSame {
		s.Error("Screenshots before and after suspension are different")
	}
}
