// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LacrosServiceGRPC,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check basic functionality of LacrosService",
		BugComponent: "crbug:OS>LaCrOS",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "ythjkt@google.com"},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "lacros"},
	})
}

// LacrosServiceGRPC tests LacrosService functionalities for managing lacros lifecycle.
func LacrosServiceGRPC(ctx context.Context, s *testing.State) {
	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	newReq := &pb.NewRequest{
		Lacros:    &pb.Lacros{},
		LoginMode: pb.LoginMode_LOGIN_MODE_FAKE_LOGIN,
		Credentials: &pb.NewRequest_Credentials{
			Username: "user@test.com",
			Password: "password",
		},
	}

	// Start Chrome on DUT.
	cs := pb.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, newReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	ls := pb.NewLacrosServiceClient(cl.Conn)

	if _, err := ls.Launch(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to launch Lacros: ", err)
	}

	if _, err := ls.Connect(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to connect to Lacros: ", err)
	}

	if _, err := ls.Close(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close Lacros: ", err)
	}

	if _, err := ls.LaunchWithURL(ctx, &pb.LaunchWithURLRequest{Url: "about:blank"}); err != nil {
		s.Fatal("Failed to connect to Lacros: ", err)
	}

	if _, err := ls.Close(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close Lacros: ", err)
	}

	// Close Chrome on DUT.
	if _, err := cs.Close(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close Chrome: ", err)
	}
}
