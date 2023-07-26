// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillModemmanager,
		Desc:         "Verifies that Shill behaves correctly when modemmanager is restarted",
		Contacts:     []string{"chromeos-cellular-team@google.com", "stevenjb@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_cq", "cellular_ota_avl"},
		Fixture:      "cellular",
	})
}

func ShillModemmanager(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	modem1, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}
	deviceProps, err := helper.Device.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to get Device properties: ", err)
	}
	modemPath, err := deviceProps.GetString(shillconst.DevicePropertyDBusObject)
	if err != nil {
		s.Fatal("Failed to get Device.DBusObject property: ", err)
	}
	if modemPath != modem1.String() {
		s.Fatalf("Path mismatch, got: %q, want: %q", modemPath, modem1.String())
	}

	if err := cellular.RestartModemManager(ctx); err != nil {
		s.Fatal("Failed to restart modemmanager: ", err)
	}

	var modem2 *modemmanager.Modem
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		modem2, err = modemmanager.NewModem(ctx)
		return err
	}, &testing.PollOptions{
		Timeout:  60 * time.Second,
		Interval: 1 * time.Second,
	}); err != nil {
		s.Fatal("Failed to create Modem after restart: ", err)
	}

	if helper.Device.WaitForProperty(ctx, shillconst.DevicePropertyDBusObject, modem2.String(), 30*time.Second); err != nil {
		s.Fatal("Failed to get matching Device.DBus.Object: ", err)
	}
	if _, err := helper.FindService(ctx); err != nil {
		s.Fatal("Unable to find Cellular Service after modemmanager restart: ", err)
	}
}
