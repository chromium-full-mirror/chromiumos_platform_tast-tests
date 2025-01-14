// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package boca

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/services/cros/boca"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Launch,
		Desc: "Launch boca app",
		Contacts: []string{
			"avantidorle@google.com",
			"cros-edu-eng@google.com",
		},
		BugComponent: "b:1568002", // ChromeOS Server Projects > Enterprise Management > Edu Features > School Tools > Class Hub
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.boca.ClassToolsService"},
		Vars:         []string{"boca.Launch.username_teacher", "boca.Launch.password_teacher", "boca.Launch.username_student", "boca.Launch.password_student"},
	})
}

func Launch(ctx context.Context, s *testing.State) {

	d1 := s.DUT()
	client1, err := rpc.Dial(ctx, d1, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer client1.Close(ctx)

	bs1 := boca.NewClassToolsServiceClient(client1.Conn)
	loginReq := &boca.CrOSLoginRequest{}
	loginReq.Username = s.RequiredVar("boca.Launch.username_teacher")
	loginReq.Password = s.RequiredVar("boca.Launch.password_teacher")
	loginReq.EnabledFlags = "Boca"
	if _, err = bs1.NewChromeLogin(ctx, loginReq); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer bs1.CloseChrome(ctx, &empty.Empty{})

	// Connect to the companion device cd1.
	d2 := s.CompanionDUT("cd1")
	if d2 == nil {
		s.Fatal("Failed to get companion DUT cd1: ", d2)
	}
	client2, err := rpc.Dial(ctx, d2, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the compaion device cd1: ", err)
	}
	defer client2.Close(ctx)

	bs2 := boca.NewClassToolsServiceClient(client2.Conn)
	loginReqCompanion := &boca.CrOSLoginRequest{}
	loginReqCompanion.Username = s.RequiredVar("boca.Launch.username_student")
	loginReqCompanion.Password = s.RequiredVar("boca.Launch.password_student")
	loginReqCompanion.EnabledFlags = "Boca,BocaConsumer"
	if _, err = bs2.NewChromeLogin(ctx, loginReqCompanion); err != nil {
		s.Fatal("Failed to start Chrome on companion DUT: ", err)
	}
	defer bs2.CloseChrome(ctx, &empty.Empty{})

	if _, err = bs1.VerifySessionStart(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to verify Class tools launch: ", err)
	}

	if _, err = bs2.VerifyStudentSession(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to verify student session has started: ", err)
	}

	if _, err = bs1.EndSession(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to end session: ", err)
	}
}
