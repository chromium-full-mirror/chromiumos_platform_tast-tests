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
	"go.chromium.org/tast-tests/cros/common/usbdevice"
	"go.chromium.org/tast-tests/cros/local/usbip"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceAttach,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify devices can be attached",
		Contacts: []string{
			"chromeos-dev-engprod@google.com",
			"mattlui@google.com",
			"jinrongwu@google.com",
		},
		BugComponent: "b:1103568", // ChromeOS -> EngProd -> Developer
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Fixture:      "usbipServer",
		Timeout:      2 * time.Minute,
	})
}

// DeviceAttach verifies devices can be attached.
func DeviceAttach(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	server := s.FixtValue().(*usbip.ServerFixtureData).Server
	attachKeyboard := server.AddDevice(usbdevice.NewUsbKeyboard(ctx))
	detachKeyboard, err := attachKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to attach a virtual keyboard: ", err)
	}
	defer func(cleanupCtx context.Context) {
		if err := detachKeyboard(cleanupCtx); err != nil {
			s.Fatal("Failed to detach virtual keyboard: ", err)
		}
	}(cleanupCtx)

	attachMouse := server.AddDevice(usbdevice.NewUsbMouse(ctx))
	detachMouse, err := attachMouse(ctx)
	if err != nil {
		s.Fatal("Failed to attach a virtual mouse: ", err)
	}
	defer func(cleanupCtx context.Context) {
		if err := detachMouse(cleanupCtx); err != nil {
			s.Fatal("Failed to detach virtual keyboard: ", err)
		}
	}(cleanupCtx)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		lsusb := testexec.CommandContext(ctx, "lsusb")
		lsusbOut, err := lsusb.CombinedOutput()
		if err != nil {
			return errors.Wrap(err, "failed to get lsusb")
		}
		allDevices := string(lsusbOut)

		keyboardID := fmt.Sprintf("%04x:%04x", usbdevice.VendorIDGoogle, usbdevice.ProductIDGoogleKeyboard)
		if !strings.Contains(allDevices, keyboardID) {
			return errors.Errorf("Usbip port returned unexpected imported devices: expected to contain %q got: %q", keyboardID, allDevices)
		}
		mouseID := fmt.Sprintf("%04x:%04x", usbdevice.VendorIDGoogle, usbdevice.ProductIDGoogleMouse)
		if !strings.Contains(allDevices, mouseID) {
			return errors.Errorf("Usbip port returned unexpected imported devices: expected to contain %q got: %q", mouseID, allDevices)
		}
		return nil
	}, &testing.PollOptions{Interval: 3 * time.Second, Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to attach virtual devices: ", err)
	}
}
