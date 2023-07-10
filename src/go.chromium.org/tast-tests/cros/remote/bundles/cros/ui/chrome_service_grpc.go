// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	defaultUsername = "testuser@gmail.com"
	defaultPassword = "testpass"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeServiceGRPC,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check basic functionality of ChromeService",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"ui.gaiaPoolDefault"},
		HardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Clamshell)),
		Params: []testing.Param{{
			Name: "default_fake_login",
			Val:  &pb.NewRequest{},
		}, {
			Name: "fake_login",
			Val: &pb.NewRequest{
				LoginMode: pb.LoginMode_LOGIN_MODE_FAKE_LOGIN,
				Credentials: &pb.NewRequest_Credentials{
					Username: defaultUsername,
					Password: defaultPassword,
				},
				TryReuseSession: false,
				KeepState:       false,
				EnableFeatures:  []string{"GwpAsanMalloc", "GwpAsanPartitionAlloc"},
				ExtraArgs:       []string{"--enable-logging"},
			},
		}, {
			Name: "fake_login_try_reuse_sessions",
			Val: &pb.NewRequest{
				LoginMode: pb.LoginMode_LOGIN_MODE_FAKE_LOGIN,
				Credentials: &pb.NewRequest_Credentials{
					Username: defaultUsername,
					Password: defaultPassword,
				},
				TryReuseSession: true,
				KeepState:       true,
				// Requesting the same features and args in order to reuse the same session
				EnableFeatures: []string{"GwpAsanMalloc", "GwpAsanPartitionAlloc"},
				ExtraArgs:      []string{"--enable-logging"},
			},
		}, {
			Name: "gaia_login",
			Val: &pb.NewRequest{
				// The test definition block has no access to testing.State and "ui.gaiaPoolDefault".
				// Credentials will be populated based on "ui.gaiaPoolDefault" in the main test function.
				LoginMode: pb.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			},
		}, {
			Name: "default_fake_login_lacros",
			Val: &pb.NewRequest{
				// Default to using Rootfs Lacros.
				Lacros: &pb.Lacros{Selection: pb.Lacros_SELECTION_ROOTFS}},
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:              "disabled_lacros",
			Val:               &pb.NewRequest{},
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// ChromeServiceGRPC tests ChromeService functionalities for managing chrome lifecycle.
func ChromeServiceGRPC(ctx context.Context, s *testing.State) {
	// Using port 0 allows cros server to pick any available port
	cl, err := crosserverutil.Dial(ctx, s.DUT(), "localhost", 0, true)
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Populate credentials from Tast variable for the Gaia login test case.
	loginReq := s.Param().(*pb.NewRequest)
	if loginReq.LoginMode == pb.LoginMode_LOGIN_MODE_GAIA_LOGIN && loginReq.Credentials == nil {
		if loginReq.Credentials, err = pickRandomCreds(s.RequiredVar("ui.gaiaPoolDefault")); err != nil {
			s.Fatal("Failed to get login credentials: ", err)
		}
	}

	// Start Chrome on DUT
	cs := pb.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	// Close Chrome on DUT
	if _, err := cs.Close(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close Chrome: ", err)
	}
}

// pickRandomCreds picks a random user and password from a list of credentials.
func pickRandomCreds(creds string) (*pb.NewRequest_Credentials, error) {
	randomCreds, err := credconfig.PickRandomCreds(creds)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get login creds")
	}

	return &pb.NewRequest_Credentials{
		Username: randomCreds.User,
		Password: randomCreds.Pass,
	}, nil
}
