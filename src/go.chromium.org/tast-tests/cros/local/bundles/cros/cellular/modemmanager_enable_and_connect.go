// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModemmanagerEnableAndConnect,
		Desc:         "Verifies that modemmanager can trigger modem enable, disable, connect and disconnect succeeds",
		Contacts:     []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_run_isolated"},
		Fixture:      "cellularModemManager",
		Timeout:      5 * time.Minute,
	})
}

// ModemmanagerEnableAndConnect Test
func ModemmanagerEnableAndConnect(ctx context.Context, s *testing.State) {
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	simpleModem, err := modem.GetSimpleModem(ctx)
	if err != nil {
		s.Fatal("Could not get simplemodem object: ", err)
	}

	operatorID, err := modem.GetOperatorIdentifier(ctx)
	if err != nil {
		s.Fatal("Cannot get the OperatorIdentifier: ", err)
	}

	knownAPNs, err := cellular.GetKnownAPNsForOperator(operatorID)
	if err != nil {
		s.Fatal("Cannot find known APNs: ", err)
	}

	modem3gpp, err := modem.GetModem3gpp(ctx)
	if err != nil {
		s.Fatal("Could not get modem3gpp object: ", err)
	}

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer func(ctx context.Context) {
		if err := modem.Enable(ctx); err != nil {
			testing.ContextLog(ctx, "Modem enable failed with: ", err)
		}
		if err := simpleModem.Call(ctx, mmconst.ModemDisconnect, dbus.ObjectPath("/")).Err; err != nil {
			testing.ContextLog(ctx, "Modem disconnect failed with: ", err)
		}
		if err := modemmanager.SetInitialEpsBearerSettings(ctx, modem3gpp, map[string]interface{}{"apn": ""}); err != nil {
			testing.ContextLog(ctx, "Failed to clear the initial EPS bearer settings: ", err)
		}
	}(cleanupCtx)
	defer cancel()

	optionalAPNExist := false
	optionalAPNSucceeded := false
	for _, knownAPN := range knownAPNs {
		// Test Disable / Enable / Connect / Disconnect.
		s.Log("Disable")
		if err := modem.Disable(ctx); err != nil {
			s.Fatal("Modem disable failed with: ", err)
		}

		s.Log("Enable")
		if err := modem.Enable(ctx); err != nil {
			s.Fatal("Modem enable failed with: ", err)
		}

		if knownAPN.Optional {
			optionalAPNExist = true
		}

		mmApnInfo := knownAPN.GetAPNForModemManager()
		attachApn := map[string]interface{}{"apn": ""}
		if knownAPN.IsAttachAPN() {
			attachApn = mmApnInfo
		}
		s.Log("Attaching with: ", attachApn)
		if err := modemmanager.SetInitialEpsBearerSettings(ctx, modem3gpp, attachApn); err != nil {
			s.Log("Failed to set initial EPS bearer settings: ", err)
		}
		if err := modemmanager.EnsureRegistered(ctx, modem, simpleModem); err != nil {
			if knownAPN.Optional {
				s.Log(ctx, "Failed to register")
				continue
			}
			s.Fatal("Modem not registered: ", err)
		}
		// Set the multiplex property
		mmApnInfo[mmconst.BearerPropertyMultiplex] = mmconst.BearerMultiplexSupportRequested
		s.Log("Connecting with ", mmApnInfo)

		if _, err = modemmanager.Connect(ctx, simpleModem, mmApnInfo); err != nil {
			if knownAPN.Optional {
				s.Log("Failed to connect")
				continue
			}
			s.Fatal("Modem connect failed with error: ", err)
		}
		if knownAPN.Optional {
			optionalAPNSucceeded = true
		}
		s.Log("Disconnect")
		if err := simpleModem.Call(ctx, mmconst.ModemDisconnect, dbus.ObjectPath("/")).Err; err != nil {
			s.Fatal("Modem disconnect failed with: ", err)
		}

		if err := modemmanager.EnsureConnectState(ctx, modem, simpleModem, false); err != nil {
			s.Fatal("Modem not disconnected: ", err)
		}
	}

	if optionalAPNExist && !optionalAPNSucceeded {
		s.Fatal("None of the optional APNs connected")
	}
}
