// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShortKeysCombinationsWithEmulator,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check short keys combinations and work correctly",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_hid"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"USBID"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.ui.AutomationService"},
	})
}
func ShortKeysCombinationsWithEmulator(ctx context.Context, s *testing.State) {
	/*
		Test steps:
			1. Boot and loging to ChromeOS
			2. Trigger 'View Keyboard overlay' by pressing 'Ctrl+Search+s'
			3. Reproduce the 'Popular Shortcuts' behavior
	*/
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	USBID := s.RequiredVar("USBID")
	// Set up the servo attached to the DUT.
	dut := s.DUT()
	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after power the chrombook on: ", err)
	}
	defer cl.Close(cleanupCtx)
	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)
	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start chrome after power the chrombook on: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})
	// Plug in the USB devices.
	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to control fixture to connect the corded keyboard: ", err)
	}
	if err := utils.InitSimulator(ctx); err != nil {
		s.Fatal("Failed to initialize simulator: ", err)
	}
	defer utils.ReleaseSimulator(cleanupCtx)
	// Get device event kbd.
	kbdList, err := utils.GetDeviceEventKbd(ctx, dut)
	if err != nil {
		s.Fatal("Failed to retrieve the devices event kbd: ", err)
	}
	// Here, two device events will be obtained.
	// 1. Allion emulator.
	if len(kbdList) != 1 {
		s.Fatalf("Failed to proceed due to incorrect number of device event kbd, expect: 1, actual: %d", len(kbdList))
	}
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	defer func() {
		if s.HasError() {
			uiTree, _ := uiautoSvc.GetUITree(ctx, &ui.GetUITreeRequest{})
			s.Log(uiTree.UiTree)
		}
	}()
	keyPressDuration := 100 * time.Millisecond
	// Press ctrl + search + s.
	s.Log("Press ctrl + search + s")
	if err := utils.CombineKeyPressKeyboard(ctx, keyPressDuration, []int{utils.SpecialKeyCodes["LControlKey"].KeyCodeInt, utils.SpecialKeyCodes["LWin"].KeyCodeInt, utils.ChatKeyCodes["S"].KeyCodeInt}); err != nil {
		s.Fatal("Failed to press ctrl + search + s: ", err)
	}
	keyShortcutsFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_APPLICATION}},
			{Value: &ui.NodeWith_Name{Name: "Key Shortcuts"}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: keyShortcutsFinder}); err != nil {
		s.Fatal("Failed to open key shortcuts page: ", err)
	}

	s.Log("Press ctrl + search + scale")
	// Press ctrl + scale.
	if err := utils.CombineKeyPressKeyboard(ctx, keyPressDuration, []int{utils.SpecialKeyCodes["LControlKey"].KeyCodeInt, utils.SpecialKeyCodes["LWin"].KeyCodeInt, utils.TopRowKeyCodes["F5"].KeyCodeInt}); err != nil {
		s.Fatal("Failed to press ctrl + scale: ", err)
	}
	// GoBigSleepLint: wait for screenshot.
	testing.Sleep(ctx, 5*time.Second)
	fs := dutfs.NewClient(cl.Conn)
	defer fs.RemoveAll(cleanupCtx, utils.DownloadsPath)
	files, err := fs.ReadDir(ctx, utils.DownloadsPath)
	if err != nil {
		s.Fatal("Failed to read downloads folder: ", err)
	}
	verifyScreenshot := false
	// Check screenshot is success.
	for _, file := range files {
		if strings.Contains(file.Name(), "Screenshot") {
			verifyScreenshot = true
			break
		}
	}
	if !verifyScreenshot {
		s.Fatal("Failed to screenshot")
	}
	s.Log("Press search + c")
	// Press search + c.
	if err := utils.CombineKeyPressKeyboard(ctx, keyPressDuration, []int{utils.SpecialKeyCodes["LWin"].KeyCodeInt, utils.ChatKeyCodes["C"].KeyCodeInt}); err != nil {
		s.Fatal("Failed to press search + scale: ", err)
	}
	calendarFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Calendar"}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: calendarFinder}); err != nil {
		s.Fatal("Failed to open calendar: ", err)
	}
	// Close all windows.
	s.Log("Press esc")
	if err := utils.SinglePressKeyboard(ctx, utils.SpecialKeyCodes["Esc"].KeyCodeBytes, keyPressDuration); err != nil {
		s.Fatal("Failed to close key shortcuts page: ", err)
	}
	if err := utils.SinglePressKeyboard(ctx, utils.SpecialKeyCodes["Esc"].KeyCodeBytes, keyPressDuration); err != nil {
		s.Fatal("Failed to close key shortcuts page: ", err)
	}
	closeKeys := []int{utils.SpecialKeyCodes["LControlKey"].KeyCodeInt, utils.ChatKeyCodes["W"].KeyCodeInt}
	s.Log("Press ctrl + w")
	// Close key chortcuts.
	if err := utils.CombineKeyPressKeyboard(ctx, keyPressDuration, closeKeys); err != nil {
		s.Fatal("Failed to close key shortcuts page: ", err)
	}
}
