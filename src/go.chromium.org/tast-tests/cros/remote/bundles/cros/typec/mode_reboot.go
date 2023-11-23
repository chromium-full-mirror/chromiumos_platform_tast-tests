// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// The maximum number of USB Type C ports that a Chromebook supports.
const maxTypeCPorts = 8

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModeReboot,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Demonstrates USB Type C mode selection after reboot",
		Contacts:     []string{"chromeos-usb@google.com", "pmalani@chromium.org"},
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		SoftwareDeps: []string{"tpm2", "reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Data:         []string{"testcert.p12"},
		Params: []testing.Param{
			// For running manually.
			{
				Name:      "manual",
				ExtraAttr: []string{"typec_lab"},
				Val:       false,
			},
			// For automated testing.
			{
				Name:              "smoke",
				ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model("volteer", "voxel")),
				Val:               true,
			}},
	})
}

// ModeReboot does the following:
// - Log in on a DUT.
// - Validate TBT alt mode is working correctly.
// - Reboot the system.
// - Validate that we are *not* in TBT alt mode and that USB+DP mode is working correctly.
//
// This test requires the following H/W topology to run.
//
//	  DUT ------> Thunderbolt3 (>= Titan Ridge) dock -----> DP monitor.
//	(USB4)
func ModeReboot(ctx context.Context, s *testing.State) {
	d := s.DUT()
	if !d.Connected(ctx) {
		s.Fatal("Failed DUT connection check at the beginning")
	}

	// Connect to gRPC server
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	// Check if a TBT device is connected. If one isn't, we should skip
	// execution.
	present, err := checkPortsForTBTPartner(ctx, d)
	if err != nil {
		s.Log("Couldn't find TBT device from PD identity: ", err)
		return
	}

	// Return early for smoke testing (CQ).
	if smoke := s.Param().(bool); smoke {
		return
	}

	if !present {
		s.Fatal("No TBT device connected to DUT")
	}

	// Send key file to DUT.
	keyPath := filepath.Join("/tmp", "testcert.p12")
	defer d.Conn().CommandContext(ctx, "rm", "-r", keyPath).Output()
	if _, err := linuxssh.PutFiles(
		ctx, d.Conn(), map[string]string{
			s.DataPath("testcert.p12"): keyPath,
		},
		linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send data to remote data path %v: %v", keyPath, err)
	}

	// Login to Chrome.
	client := typec.NewServiceClient(cl.Conn)
	_, err = client.NewChromeLoginWithPeripheralDataAccess(ctx, &typec.KeyPath{Path: keyPath})
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	s.Log("Verifying that a TBT device is enumerated")
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		return typecutils.CheckTBTDevice(ctx, d, true, typecutils.TbtGenAny)
	}, &testing.PollOptions{Interval: 100 * time.Millisecond, Timeout: 20 * time.Second}); err != nil {
		s.Fatal("Failed TBT enumeration after login: ", err)
	}

	s.Log("Rebooting the DUT")
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	cl, err = rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	if !d.Connected(ctx) {
		s.Fatal("Failed to connect to DUT post reboot")
	}

	if err = testing.Poll(ctx, func(ctx context.Context) error {
		return typecutils.CheckTBTDevice(ctx, d, false, typecutils.TbtGenAny)
	}, &testing.PollOptions{Interval: 100 * time.Millisecond, Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed TBT non-enumeration after reboot: ", err)
	}

	// TODO(b/179338646): Check that there is a connected DP monitor.
}

// checkPortsForTBTPartner checks whether the device has a connected Thunderbolt device.
// We use the 'ectool typecdiscovery' command to accomplish this.
//
// This functions returns:
// - Whether a TBT device is connected to the DUT.
// - The error value if the command didn't run, else nil.
func checkPortsForTBTPartner(ctx context.Context, d *dut.DUT) (bool, error) {
	for i := 0; i < maxTypeCPorts; i++ {
		out, err := d.Conn().CommandContext(ctx, "ectool", "typecdiscovery", strconv.Itoa(i), "0").CombinedOutput()
		if err != nil {
			// If we get an invalid param error, that means there are no more ports left.
			// In that case, we shouldn't return an error, but should return false.
			//
			// TODO(pmalani): Determine how many ports a device supports, instead of
			// relying on INVALID_PARAM.
			if bytes.Contains(out, []byte("INVALID_PARAM")) {
				return false, nil
			}

			return false, errors.Wrap(err, "failed to run ectool command")
		}

		// Look for a TBT SVID in the output. If one doesn't exist, return false.
		if bytes.Contains(out, []byte("SVID 0x8087")) {
			return true, nil
		}
	}

	return false, nil
}
