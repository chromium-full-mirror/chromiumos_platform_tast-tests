// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExternalMouseSpeedAndPrimaryButton,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify external mouse movement speed, scroll, and primary button functionality switching",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService"},
		Vars:         []string{"servo", "USBID"},
		Data:         []string{"detect_mouse_speed.html", "detect_mouse_wheel.html"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name:    "clamshell_mode",
			Fixture: "enableServoAndDisableTabletMode",
		}, {
			Name:    "tablet_mode",
			Fixture: "enableServoAndTabletMode",
		}},
	})
}

func ExternalMouseSpeedAndPrimaryButton(ctx context.Context, s *testing.State) {
	/*
		Testing steps:
		1. Boot and loging to ChromeOS.
		2. Connect a external mouse to the Chromebook
		3. Go to settings page and change the "Mouse speed"
		(appears under "Touchpad speed" then change sensitivity bar to the left and to the right)
		4. Go to settings and change the "Mouse speed" as lowest then unplug/ replug the USB mouse,
		5. Sign out and sign in as Guest then click on "Learn more about Guest browsing
		6. Move mouse middle button very slowly to scroll the page.
		7. Go to settings page under 'Touchpad and mouse settings' (button to click), apply "swap primary mouse button" check-box.
		8. Convert the device into Tablet Mode and repeat the 2 ~ 7 steps in tablet mode
	*/
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize the fixture: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	USBID := s.RequiredVar("USBID")
	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to plug in the external mouse: ", err)
	}
	if err := utils.InitSimulator(ctx); err != nil {
		s.Fatal("Failed to initialize the simulator: ", err)
	}
	defer utils.ReleaseSimulator(cleanupCtx)
	fs := dutfs.NewClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	detectMouseSpeed, err := os.ReadFile(s.DataPath("detect_mouse_speed.html"))
	if err != nil {
		s.Fatal("Failed to read detect_mouse_speed.html: ", err)
	}
	if err := verifyMouseSpeed(ctx, appsSvc, uiautoSvc, fs, detectMouseSpeed); err != nil {
		s.Fatal("Failed to verify mouse speed: ", err)
	}
	if err := utils.ControlFixture(ctx, USBID, "off"); err != nil {
		s.Fatal("Failed to unplug the external mouse: ", err)
	}
	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to replug the external mouse: ", err)
	}
	if err := verifyMouseSpeed(ctx, appsSvc, uiautoSvc, fs, detectMouseSpeed); err != nil {
		s.Fatal("Failed to verify mouse speed after replug: ", err)
	}
	detectMouseWheel, err := os.ReadFile(s.DataPath("detect_mouse_wheel.html"))
	if err != nil {
		s.Fatal("Failed to read detect_mouse_wheel.html: ", err)
	}
	if err := verifyMouseWheel(ctx, appsSvc, uiautoSvc, fs, detectMouseWheel); err != nil {
		s.Fatal("Failed to verify mouse whell: ", err)
	}
	// Sign in as guest.
	if _, err := cs.Close(cleanupCtx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close chrome: ", err)
	}
	loginReq = &ui.NewRequest{LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to sign in as guest: ", err)
	}
	if err := utils.VerifyGuestLearnMore(ctx, appsSvc, uiautoSvc); err != nil {
		s.Fatal("Failed to verify guest learn more: ", err)
	}
	if err := verifyMousePrimary(ctx, appsSvc, uiautoSvc); err != nil {
		s.Fatal("Failed to verify primary functionality: ", err)
	}
}

// verifyMouseSpeed verifies whether the mouse movement speed changes due to settings,
// using HTML + JavaScript to detect the distance of mouse movement.
func verifyMouseSpeed(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, fs *dutfs.Client, detectMouseSpeedHTML []byte) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	htmlPath := path.Join(utils.MyFilesPath, "Downloads", "ms.html")
	if err := fs.WriteFile(ctx, htmlPath, detectMouseSpeedHTML, 0644); err != nil {
		return errors.Wrap(err, "failed to write html to file")
	}
	defer fs.Remove(cleanupCtx, htmlPath)
	defer appsSvc.CloseApp(cleanupCtx, &pb.CloseAppRequest{AppName: "Settings"})
	defer appsSvc.CloseApp(cleanupCtx, &pb.CloseAppRequest{AppName: "Chrome"})
	// Reset mouse.
	defer utils.MouseMove(cleanupCtx, 0, 0)
	if err := utils.OpenFileInDownloads(ctx, appsSvc, uiautoSvc, "ms.html", "Mouse Detection"); err != nil {
		return errors.Wrap(err, "open mouse detection page")
	}
	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files"}); err != nil {
		return errors.Wrap(err, "close files app")
	}
	var mousePosition = map[string]int64{}
	cursorSpeedFinder := ui.Node().Name("Cursor speed").Role(ui.Role_ROLE_SLIDER).Finder()
	webClickFinder := ui.Node().Name("Click").Role(ui.Role_ROLE_BUTTON).Finder()
	xValueFinder := ui.Node().NameContaining("x: ").Role(ui.Role_ROLE_STATIC_TEXT).Finder()
	for _, test := range []struct {
		hotkey string
		speed  string
	}{
		{"Right", "fast"},
		{"Left", "slow"},
	} {
		if err := utils.OpenDeviceSettings(ctx, "Mouse", appsSvc, uiautoSvc); err != nil {
			return errors.Wrap(err, "open device settings page")
		}
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: cursorSpeedFinder}); err != nil {
			return errors.Wrapf(err, "wait for %q from context menu", cursorSpeedFinder)
		}
		if _, err := uiautoSvc.EnsureFocused(ctx, &ui.EnsureFocusedRequest{Finder: cursorSpeedFinder}); err != nil {
			return errors.Wrapf(err, "focus on %q from context menu", cursorSpeedFinder)
		}
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: cursorSpeedFinder}); err != nil {
			return errors.Wrapf(err, "click %q from context menu", cursorSpeedFinder)
		}
		if err := utils.CombineKeyPressKeyboard(ctx, 3*time.Second, []int{utils.SpecialKeyCodes["LWin"].KeyCodeInt, utils.SpecialKeyCodes[test.hotkey].KeyCodeInt}); err != nil {
			return errors.Wrapf(err, "change mouse speed to %s", test.speed)
		}
		if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Settings"}); err != nil {
			return errors.Wrap(err, "close settings app")
		}
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: webClickFinder}); err != nil {
			return errors.Wrapf(err, "wait for %q from context menu", webClickFinder)
		}
		if _, err := uiautoSvc.EnsureFocused(ctx, &ui.EnsureFocusedRequest{Finder: webClickFinder}); err != nil {
			return errors.Wrapf(err, "focus on %q from context menu", webClickFinder)
		}
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: webClickFinder}); err != nil {
			return errors.Wrapf(err, "click %q from context menu", webClickFinder)
		}
		if err := utils.MouseMove(ctx, 100, 0); err != nil {
			return errors.Wrap(err, "mouse move")
		}
		// GoBigSleepLint: Wait for ui update.
		testing.Sleep(ctx, 300*time.Millisecond)
		info, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: xValueFinder})
		if err != nil {
			return errors.Wrap(err, "get the mouse move point")
		}
		x, err := strconv.ParseInt(strings.Replace(info.NodeInfo.Name, "x: ", "", -1), 10, 64)
		if err != nil {
			return errors.Wrap(err, "convert to int")
		}
		mousePosition[test.hotkey] = x
	}
	if mousePosition["Right"] < mousePosition["Left"] {
		return errors.New("failed to setting mouse speed")
	}
	return nil
}

// verifyMouseWheel verifies that the number of mouse wheel scrolls matches the expected value: 10.
func verifyMouseWheel(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, fs *dutfs.Client, detectMouseWheelHTML []byte) error {
	htmlPath := path.Join(utils.MyFilesPath, "Downloads", "ms.html")
	if err := fs.WriteFile(ctx, htmlPath, detectMouseWheelHTML, 0644); err != nil {
		return errors.Wrap(err, "failed to write html to file")
	}
	defer fs.Remove(ctx, htmlPath)
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files"})
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Google Chrome"})

	if err := utils.OpenFileInDownloads(ctx, appsSvc, uiautoSvc, "ms.html", "Mouse Wheel Count"); err != nil {
		return errors.Wrap(err, "open mouse wheel count page")
	}
	resetFinder := ui.Node().Name("Reset").Role(ui.Role_ROLE_BUTTON).Finder()
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: resetFinder}); err != nil {
		return errors.Wrapf(err, "wait for %q from context menu", resetFinder)
	}
	if _, err := uiautoSvc.EnsureFocused(ctx, &ui.EnsureFocusedRequest{Finder: resetFinder}); err != nil {
		return errors.Wrapf(err, "focus on %q from context menu", resetFinder)
	}
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: resetFinder}); err != nil {
		return errors.Wrapf(err, "click %q from context menu", resetFinder)
	}
	for i := 0; i < 10; i++ {
		if err := utils.MouseScroll(ctx, -1); err != nil {
			return errors.Wrap(err, "mouse scroll")
		}
		// GoBigSleepLint: Simulate slowly scrolling the mouse wheel.
		testing.Sleep(ctx, 300*time.Millisecond)
	}
	// GoBigSleepLint: Wait for ui update.
	testing.Sleep(ctx, 300*time.Millisecond)
	wheelCountFinder := ui.Node().NameContaining("Mouse Wheel Count: ").Role(ui.Role_ROLE_STATIC_TEXT).Finder()
	info, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: wheelCountFinder})
	if err != nil {
		return errors.Wrap(err, "get the mouse wheel count")
	}
	count, err := strconv.ParseInt(strings.Replace(info.NodeInfo.Name, "Mouse Wheel Count: ", "", -1), 10, 64)
	if err != nil {
		return errors.Wrap(err, "convert to int")
	}
	if count != 10 {
		return errors.Errorf("mouse wheel time got:%d want:10", count)
	}
	return nil
}

func verifyMousePrimary(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient) error {
	if err := utils.OpenDeviceSettings(ctx, "Mouse", appsSvc, uiautoSvc); err != nil {
		return errors.Wrap(err, "open device settings page")
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Settings"})
	customizeFinder := ui.Node().Name("Customize mouse buttons").Nth(0).Finder()
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: customizeFinder}); err != nil {
		return errors.Wrapf(err, "wait for %q from context menu", customizeFinder)
	}
	if _, err := uiautoSvc.EnsureFocused(ctx, &ui.EnsureFocusedRequest{Finder: customizeFinder}); err != nil {
		return errors.Wrapf(err, "focus on %q from context menu", customizeFinder)
	}
	if _, err := uiautoSvc.DoDefault(ctx, &ui.DoDefaultRequest{Finder: customizeFinder}); err != nil {
		return errors.Wrapf(err, "click %q from context menu", customizeFinder)
	}
	vpsFinder := ui.Node().NameContaining("View page source").Nth(0).Finder()
	primaryFinder := ui.Node().NameRegex("(?i)Primary mouse button").Nth(0).Finder()
	for i := 0; i < 2; i++ {
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: primaryFinder}); err != nil {
			return errors.Wrapf(err, "wait for %q from context menu", primaryFinder)
		}
		if _, err := uiautoSvc.EnsureFocused(ctx, &ui.EnsureFocusedRequest{Finder: primaryFinder}); err != nil {
			return errors.Wrapf(err, "focus on %q from context menu", primaryFinder)
		}
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: primaryFinder}); err != nil {
			return errors.Wrapf(err, "click %q from context menu", primaryFinder)
		}
		// GoBigSleepLint: Wait for ui update.
		testing.Sleep(ctx, 300*time.Millisecond)
		info, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: primaryFinder})
		if err != nil {
			return errors.Wrap(err, "get the primary info")
		}
		mouseKey := "MouseLeft"
		if info.NodeInfo.Checked == ui.Checked_CHECKED_TRUE {
			mouseKey = "MouseLeft"
		}
		if info.NodeInfo.Checked == ui.Checked_CHECKED_FALSE {
			mouseKey = "MouseRight"
		}
		if err := utils.MouseClick(ctx, utils.MouseKeyCodes[mouseKey].KeyCodeBytes, 200*time.Millisecond); err != nil {
			return errors.Wrapf(err, "click %s", mouseKey)
		}
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: vpsFinder}); err != nil {
			return errors.Wrapf(err, "wait for %q from context menu", vpsFinder)
		}
		if err := utils.MouseClick(ctx, utils.MouseKeyCodes[mouseKey].KeyCodeBytes, 200*time.Millisecond); err != nil {
			return errors.Wrapf(err, "click %s after show view page source button", mouseKey)
		}
	}
	return nil
}
