// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ModemmanagerEnableAndConnect,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that modemmanager can trigger modem enable, disable, connect and disconnect succeeds",
		Contacts:       []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_sim_active", "group:release-health", "release-health_cellular"},
		Fixture:        "cellularModemManager",
		Timeout:        5 * time.Minute,
	})
}

// ModemmanagerEnableAndConnect Test
func ModemmanagerEnableAndConnect(ctx context.Context, s *testing.State) {
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	operatorID, gid1, err := modem.GetOperatorAndGid1Identifiers(ctx)
	if err != nil {
		s.Fatal("Cannot get the Operator and GID1 Identifiers: ", err)
	}

	knownAPNs, err := cellular.GetKnownAPNsForOperator(operatorID, gid1)
	if err != nil {
		s.Fatal("Cannot find known APNs: ", err)
	}

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer func(ctx context.Context) {
		if err := modem.Enable(ctx); err != nil {
			testing.ContextLog(ctx, "Modem enable failed with: ", err)
		}
		if err := modem.DisconnectAll(ctx); err != nil {
			testing.ContextLog(ctx, "Modem disconnect failed with: ", err)
		}
	}(cleanupCtx)
	defer cancel()

	optionalAPNExist := false
	optionalAPNSucceeded := false
	usedApns := map[string]struct{}{}
	for _, knownAPN := range knownAPNs {
		mmApnInfo := knownAPN.GetAPNForModemManager()
		if apnName, ok := mmApnInfo[mmconst.BearerPropertyApn].(string); ok {
			// GetKnownAPNsForOperator returns both DEFAULT+IA and DEFAULT-only entries
			// for the same APN name (with DEFAULT+IA ordered first) so Shill tests can
			// exercise both configurations. Skip duplicate APN names here so a subsequent
			// DEFAULT-only entry does not overwrite the attach APN with an empty APN ("").
			if _, exists := usedApns[apnName]; exists {
				continue
			}
			usedApns[apnName] = struct{}{}
		}

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

		attachApn := map[string]interface{}{"apn": ""}
		if knownAPN.IsAttachAPN() {
			attachApn = mmApnInfo
		}
		s.Log("Attaching with: ", attachApn)
		if err := modem.SetInitialEpsBearerSettings(ctx, attachApn); err != nil {
			s.Log("Failed to set initial EPS bearer settings: ", err)
		}
		if err := modem.EnsureRegistered(ctx); err != nil {
			if knownAPN.Optional {
				s.Log(ctx, "Failed to register")
				continue
			}
			s.Fatal("Modem not registered: ", err)
		}
		// Set the multiplex property
		mmApnInfo[mmconst.BearerPropertyMultiplex] = mmconst.BearerMultiplexSupportRequested
		mmApnInfo[mmconst.BearerPropertyApnType] = mmconst.BearerAPNTypeDefault
		s.Log("Connecting with ", mmApnInfo)

		if _, err = modem.Connect(ctx, mmApnInfo); err != nil {
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
		if err := modem.DisconnectAll(ctx); err != nil {
			s.Fatal("Modem disconnect failed with: ", err)
		}

		if err := modem.EnsureConnectState(ctx, false); err != nil {
			s.Fatal("Modem not disconnected: ", err)
		}

		// When a carrier defines multiple optional APNs (e.g., LTE "broadband" and
		// 5G "nrbroadband" on AT&T), only one may be provisioned on the SIM. Stop
		// once an optional APN succeeds so a subsequent unprovisioned optional APN
		// does not overwrite the working attach APN in modem NVM and leave the modem
		// detached.
		if knownAPN.Optional {
			break
		}
	}

	if optionalAPNExist && !optionalAPNSucceeded {
		s.Fatal("None of the optional APNs connected")
	}
}
