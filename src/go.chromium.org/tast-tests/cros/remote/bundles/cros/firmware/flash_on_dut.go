// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FlashOnDUT,
		Desc: "Flashes the usb drive on the dut",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      fixture.NormalMode,
		Timeout:      2 * time.Hour,
	})
}

func FlashOnDUT(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)
	h := v.Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod: ", err)
	}
	if err := h.RequireRPCUtils(ctx); err != nil {
		s.Fatal("Failed to connect utils service: ", err)
	}
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to switch usb to dut: ", err)
	}
	var localhostServers []string
	var remoteServers []string
	for _, devserver := range s.CloudStorage().Devservers() {
		if strings.HasPrefix(devserver, "http://127.0.0.1:") {
			localHostPort := strings.TrimPrefix(devserver, "http://")
			remoteHostPort, err := h.DUT.Conn().ForwardRemoteToLocal("tcp", "127.0.0.1:0", localHostPort, nil)
			if err != nil {
				s.Fatal("Failed to forward port to DUT: ", err)
			}
			defer remoteHostPort.Close()
			localhostServers = append(localhostServers, "http://"+remoteHostPort.ListenAddr().String())
		} else {
			remoteServers = append(remoteServers, devserver)
		}
	}
	if _, err := h.RPCUtils.FlashUSBDrive(ctx, &firmware.FlashUSBDriveRequest{
		Devserver: localhostServers,
		GsPath:    s.CloudStorage().BuildArtifactsURL(),
	}); err != nil {
		s.Fatal("Failed to flash usb drive: ", err)
	}
}
