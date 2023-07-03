// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/usbdevice"
	"go.chromium.org/tast-tests/cros/local/usbip"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceList,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify device listing works",
		Contacts: []string{
			"chromeos-engprod-sydney@google.com",
			"ashpakov@google.com",
		},
		BugComponent: "b:1103568", // ChromeOS -> EngProd -> Developer
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "usbipServer",
		Timeout:      2 * time.Minute,
	})
}

// DeviceList verifies device listing works.
func DeviceList(ctx context.Context, s *testing.State) {
	server := s.FixtValue().(*usbip.ServerFixtureData).Server
	server.AddDevice(usbdevice.NewUsbKeyboard(ctx))
	server.AddDevice(usbdevice.NewUsbMouse(ctx))

	getDevList := testexec.CommandContext(ctx, "usbip", "list", "-r", "localhost")
	output, err := getDevList.CombinedOutput()
	if err != nil {
		s.Fatal("Failed to get usbip device list: ", err)
	}
	devList := string(output)

	expectedKeyboard := fmt.Sprintf("1-1.0: unknown vendor : unknown product (%04x:%04x)", usbdevice.VendorIDGoogle, usbdevice.ProductIDGoogleKeyboard)
	if !strings.Contains(devList, expectedKeyboard) {
		s.Fatalf("Usbip list returned unexpected result: expected to contain %q got: %q", expectedKeyboard, devList)
	}
	expectedMouse := fmt.Sprintf("1-1.1: unknown vendor : unknown product (%04x:%04x)", usbdevice.VendorIDGoogle, usbdevice.ProductIDGoogleMouse)
	if !strings.Contains(devList, expectedMouse) {
		s.Fatalf("Usbip list returned unexpected result: expected to contain %q got: %q", expectedMouse, devList)
	}
}
