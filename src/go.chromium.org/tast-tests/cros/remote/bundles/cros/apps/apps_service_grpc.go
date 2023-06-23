// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParams struct {
	chromeRequest *ui.NewRequest
	browserID     string
}

var disableFeatures = []string{"DefaultWebAppInstallation"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppsServiceGRPC,
		Desc:         "Check basic functionalities of AppsService",
		Contacts:     []string{"chromeos-engprod-syd@google.com", "msta@google.com"},
		BugComponent: "b:1103568", // ChromeOS -> EngProd -> Apps
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model("betty")),
		LacrosStatus: testing.LacrosVariantExists,
		Params: []testing.Param{
			{
				Name: "ash",
				Val: testParams{
					chromeRequest: &ui.NewRequest{DisableFeatures: disableFeatures},
					browserID:     "mgndgikekgjfcpckkfioiadnlibdjbkf", // See Chrome.ID in local/apps/apps.go.
				},
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: testParams{
					chromeRequest: &ui.NewRequest{DisableFeatures: disableFeatures, EnableFeatures: []string{"LacrosOnly"}, LacrosExtraArgs: []string{"--no-first-run"}},
					browserID:     "jaimifaeiicidiikhmjedcgdimealfbh", // See LacrosID in local/apps/apps.go.
				},
			},
		},
	})
}

// AppsServiceGRPC tests basic functionalities of UI AppsService.
func AppsServiceGRPC(ctx context.Context, s *testing.State) { // NOLINT
	variant := s.Param().(testParams)

	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, variant.chromeRequest, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(ctx, &empty.Empty{})

	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Unknown app", TimeoutSecs: 1}); err == nil {
		s.Fatal("Launch non-existent app succeeded")
	}

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch files app: ", err)
	}

	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)

	filesAppWindowFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Files - My files"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: filesAppWindowFinder}); err != nil {
		s.Fatal("Files app never appeared: ", err)
	}

	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close files app: ", err)
	}

	browser, err := appsSvc.LaunchPrimaryBrowser(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to launch primary browser: ", err)
	}
	// We compare the app IDs rather than the names because in the LacrosOnly configuration Lacros is called "Chrome".
	if browser.Id != variant.browserID {
		s.Fatalf("Incorrect browser ID: got %v; want %v", browser.Id, variant.browserID)
	}

	browserWindowFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: "New Tab"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: browserWindowFinder}); err != nil {
		s.Fatal("Browser never appeared: ", err)
	}
}
