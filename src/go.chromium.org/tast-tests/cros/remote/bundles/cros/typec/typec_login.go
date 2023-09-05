// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"path/filepath"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TypecLogin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Processes login and enables data peripheral access setting (when mode set to complete) to enable TBT/USB4",
		Contacts: []string{
			"chromeos-usb@google.com",
			"rajat.khandelwal@intel.com",
		},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"testcert.p12"},
		BugComponent: "b:958036",

		Params: []testing.Param{
			// To login and change mode without enabling data peripheral access
			{
				Name: "normal",
				Val:  false,
			},
			// To login and enable data peripheral access
			{
				Name: "complete",
				Val:  true,
			}},
	})
}

// TypecLogin does the following:
// If normal, login happens as per the default behavior in Chrome.
// If complete, data peripheral access is also enabled.
func TypecLogin(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	if !dut.Connected(ctx) {
		s.Fatal("Failed DUT connection check at the beginning")
	}

	// Connect to gRPC server
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	// Send key file to DUT
	keyPath := filepath.Join("/tmp", "testcert.p12")
	defer dut.Conn().CommandContext(ctx, "rm", "-r", keyPath).Output()
	if _, err := linuxssh.PutFiles(
		ctx, dut.Conn(), map[string]string{
			s.DataPath("testcert.p12"): keyPath,
		},
		linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send data to remote data path %v: %v", keyPath, err)
	}

	client := typec.NewServiceClient(cl.Conn)

	peripheralDataEnableReq := s.Param().(bool)
	if peripheralDataEnableReq == true {
		_, err = client.NewChromeLoginWithPeripheralDataAccess(ctx, &typec.KeyPath{Path: keyPath})
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
	} else {
		_, err = client.NewChromeLogin(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
	}
}
