// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

// unifiedSystemTray is the finder for the unified system tray.
var unifiedSystemTray = ui.Node().HasClass("UnifiedSystemTray").Nth(0).Finder()

// powerMenuButton is the finder for the power menu button.
var powerMenuButton = ui.Node().Name("Power menu").Role(ui.Role_ROLE_BUTTON).Nth(0).Finder()

// powerMenuLockItem is the finder for the "Lock" item in the power menu.
var powerMenuLockItem = ui.Node().NameContaining("Lock").Role(ui.Role_ROLE_MENU_ITEM).Nth(0).Finder()

// passwordTextfield is the finder for the password textfield in the lock screen.
var passwordTextfield = ui.Node().NameContaining("Password").Role(ui.Role_ROLE_TEXT_FIELD).Nth(0).Finder()

// launcherButton is the finder for the luancher button in the unlock screen.
var launcherButton = ui.Node().Name("Launcher").Role(ui.Role_ROLE_BUTTON).HasClass("HomeButton").Nth(0).Finder()

func init() {
	testing.AddTest(&testing.Test{
		Func:         CordedKeyboardFunctionalityCheckAfterLockUnlockScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the external keyboard functionality after lock/unlock the DUT",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_hid"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps: []string{
			"tast.cros.nearbyservice.NearbyShareService",
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.inputs.KeyboardService",
			"tast.cros.ui.ChromeUIService",
		}, Params: []testing.Param{{
			Name:    "clamshell_mode",
			Fixture: "enableServoAndDisableTabletMode",
		}, {
			Name:    "tablet_mode",
			Fixture: "enableServoAndTabletMode",
		}},
	})
}

func CordedKeyboardFunctionalityCheckAfterLockUnlockScreen(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dut := s.DUT()
	USBID := s.RequiredVar("USBID")

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after power the chrombook on: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to control fixture to connect the USB keyboard emulator: ", err)
	}

	// Retrieve the kbd device event and verify its count.
	kbdList, err := utils.GetDeviceEventKbd(ctx, dut)
	if err != nil {
		s.Fatal("Failed to retrieve the keyboard emulator info: ", err)
	} else if len(kbdList) != 1 {
		s.Fatalf("Expect to get only one keyboard emulator, but got: %d", len(kbdList))
	}
	keyboardEmulator := "/dev/input/by-id/" + kbdList[0]
	testing.ContextLogf(ctx, "Found the USB keyboard emulator: %s", keyboardEmulator)

	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)
	defer func(ctx context.Context) {
		if s.HasError() {
			uiTreeResponse, err := uiautoSvc.GetUITree(ctx, &ui.GetUITreeRequest{})
			if err != nil {
				s.Log("Unable to get UI tree: ", err)
			}

			if err := os.WriteFile(filepath.Join(s.OutDir(), "ui_tree.txt"), []byte(uiTreeResponse.UiTree), 0644); err != nil {
				s.Log("Unable to save UI tree on the host: ", err)
			}
		}
	}(cleanupCtx)

	if err := lockScreen(ctx, uiautoSvc); err != nil {
		s.Fatal("Failed to lock screen: ", err)
	}

	if err := unlockWithPassword(ctx, uiautoSvc, keyboardSvc, "testpass"); err != nil {
		s.Fatal("Failed to unlock with password: ", err)
	}

	if err := utils.InitSimulator(ctx); err != nil {
		s.Fatal("Failed to initialize simulator: ", err)
	}
	defer utils.ReleaseSimulator(cleanupCtx)

	if err := utils.CheckTopRowkeys(ctx, s, keyboardEmulator); err != nil {
		s.Fatal("Failed to check top row keys: ", err)
	}

	if err := utils.CheckSpecialKeys(ctx, s, cl, appsSvc, uiautoSvc, keyboardSvc, keyboardEmulator); err != nil {
		s.Fatal("Failed to check special keys: ", err)
	}

	if err := utils.CheckCharacterKeys(ctx, s, cl, appsSvc, uiautoSvc, keyboardSvc, keyboardEmulator); err != nil {
		s.Fatal("Failed to check character keys: ", err)
	}
}

// lockScreen locks the screen.
func lockScreen(ctx context.Context, uiautoSvc ui.AutomationServiceClient) error {
	for _, finder := range []*ui.Finder{
		unifiedSystemTray,
		powerMenuButton,
		powerMenuLockItem,
	} {
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: finder}); err != nil {
			return errors.Wrapf(err, "failed to wait until the %q exists", finder)
		}
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: finder}); err != nil {
			return errors.Wrapf(err, "failed to click on the %q", finder)
		}
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: passwordTextfield}); err != nil {
		return errors.Wrapf(err, "failed to wait until the %q exists", passwordTextfield)
	}

	return nil
}

// unlockWithPassword unlocks the screen by typing the given password, assuming that the screen is locked.
func unlockWithPassword(ctx context.Context, uiautoSvc ui.AutomationServiceClient, kb inputs.KeyboardServiceClient, password string) error {
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: passwordTextfield}); err != nil {
		return errors.Wrapf(err, "failed to wait until the %q exists", passwordTextfield)
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: passwordTextfield}); err != nil {
		return errors.Wrapf(err, "failed to click on the %q", passwordTextfield)
	}

	if _, err := kb.Type(ctx, &inputs.TypeRequest{Key: password}); err != nil {
		return errors.Wrap(err, "type in password")
	}

	if _, err := kb.Type(ctx, &inputs.TypeRequest{Key: "\n"}); err != nil {
		return errors.Wrap(err, "sumbit password")
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: launcherButton}); err != nil {
		return errors.Wrapf(err, "failed to wait until the %q exists", launcherButton)
	}
	return nil
}
