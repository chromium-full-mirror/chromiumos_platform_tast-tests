// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/remote/crosserverutil"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	uipb "go.chromium.org/tast-tests/cros/services/cros/ui"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TouchServiceGRPC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check basic functionality of TouchService",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

// TouchServiceGRPC check TouchService basic functionalities like Swipe.
func TouchServiceGRPC(ctx context.Context, s *testing.State) {
	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Start Chrome on the DUT.
	cs := uipb.NewChromeServiceClient(cl.Conn)
	loginReq := &uipb.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(ctx, &empty.Empty{})

	// Use Chrome connection service to bring up the Chrome version page.
	connSvc := uipb.NewConnServiceClient(cl.Conn)
	defer connSvc.CloseAll(ctx, &empty.Empty{})
	url := "chrome://version/"
	if _, err := connSvc.NewConn(ctx, &uipb.NewConnRequest{Url: url}); err != nil {
		s.Fatal("Failed to open Chrome version page: ", err)
	}

	// Find out the dimension of the main display.
	uiautoSvc := uipb.NewAutomationServiceClient(cl.Conn)
	rootWindowFinder := &uipb.Finder{
		NodeWiths: []*uipb.NodeWith{
			{Value: &uipb.NodeWith_HasClass{HasClass: "RootWindow-0"}},
			{Value: &uipb.NodeWith_Role{Role: uipb.Role_ROLE_WINDOW}},
		},
	}
	rootWindowInfo, err := uiautoSvc.Info(ctx, &uipb.InfoRequest{Finder: rootWindowFinder})
	if err != nil {
		s.Fatal("Failed to return information for root window: ", err)
	}
	width, height := rootWindowInfo.NodeInfo.Location.Width, rootWindowInfo.NodeInfo.Location.Height

	// Create the starting point and end point of a Swipe request,
	// with coordinates relative to the dimensions of the display.
	touchSvc := inputspb.NewTouchServiceClient(cl.Conn)
	startX, startY := width/2, height/2
	endX, endY := startX, startY-height/5
	swipeRequest := &inputspb.SwipeRequest{
		X0:               startX,
		Y0:               startY,
		X1:               endX,
		Y1:               endY,
		Dx:               width / 20,
		Dy:               0,
		Touches:          2,
		TimeMilliSeconds: 200,
	}

	// Multiple requests for 2 touch swipe up will be issued, which scrolls down the version page.
	// To validate that the screen scrolling is done properly, we are relying the very last item
	// "Profile Page" on the version page. Initially, the "Profile Page" line was offscreen.
	// As we swipe on the touchscreen, "Profile Page" will become visible on the screen.
	profilePathOffscreenFinder := &uipb.Finder{
		NodeWiths: []*uipb.NodeWith{
			{Value: &uipb.NodeWith_Role{Role: uipb.Role_ROLE_STATIC_TEXT}},
			{Value: &uipb.NodeWith_Name{Name: "Profile Path"}},
		},
	}

	numSwipes := 10
	for i := 0; i < numSwipes; i++ {
		if _, err := touchSvc.Swipe(ctx, swipeRequest); err != nil {
			s.Fatal("Failed to swipe: ", err)
		}
		// Sleep for 1 second to ensure that UI is done updating after the swipe.
		testing.Sleep(ctx, 1*time.Second)

		// Check the offscreen state of the "Profile Page" item.
		// Swiping works if the "Profile Page" item appears on the screen.
		profilePathInfo, err := uiautoSvc.Info(ctx, &uipb.InfoRequest{Finder: profilePathOffscreenFinder})
		if err != nil {
			s.Fatal("Failed to get Profile Path node: ", err)
		}
		val, ok := profilePathInfo.NodeInfo.State["offscreen"]
		if !ok || !val {
			return
		}
	}
	s.Fatalf("Failed to scroll to the bottom of the page after %d swipes", numSwipes)
}
