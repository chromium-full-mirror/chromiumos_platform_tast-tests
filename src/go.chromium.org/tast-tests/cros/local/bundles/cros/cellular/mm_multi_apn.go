// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           MMMultiApn,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that Modem Manager supports multiple concurrent APN connections(multiplex)",
		Contacts:       []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Fixture:        "cellularModemManager",
	})
}

// MMMultiApn Test
func MMMultiApn(ctx context.Context, s *testing.State) {
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		s.Fatal("Failed to create Modem: ", err)
	}
	modem3gpp, err := modem.GetModem3gpp(ctx)
	if err != nil {
		s.Fatal("Could not get modem3gpp object: ", err)
	}
	simpleModem, err := modem.GetSimpleModem(ctx)
	if err != nil {
		s.Fatal("Could not get simplemodem object: ", err)
	}
	// Clear the attach APN to use the default attach APN
	if err := modemmanager.SetInitialEpsBearerSettings(ctx, modem3gpp, map[string]interface{}{"apn": ""}); err != nil {
		testing.ContextLog(ctx, "Failed to clear the initial EPS bearer settings: ", err)
	}
	if err := modemmanager.EnsureRegistered(ctx, modem, simpleModem); err != nil {
		s.Fatal("Modem not registered: ", err)
	}
	operatorID, err := modem.GetOperatorIdentifier(ctx)
	if err != nil {
		s.Fatal("Cannot get the OperatorIdentifier: ", err)
	}
	knownAPNs, err := cellular.GetKnownAPNsForOperator(operatorID)
	if err != nil {
		s.Fatal("Cannot find known APNs: ", err)
	}

	// Shorten deadline to leave time for cleanup.
	clearAttachCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Second)
	defer func(ctx context.Context) {
		s.Log("Disconnect")
		if err := simpleModem.Call(ctx, mmconst.ModemDisconnect, dbus.ObjectPath("/")).Err; err != nil {
			testing.ContextLog(ctx, "Modem disconnect failed with: ", err)
		}
		// Restart ModemManager, since te multiplex feature is still experimental in ChromeOS.
		testing.ContextLogf(ctx, "Restarting : %q", modemmanager.JobName)
		if err := upstart.RestartJob(ctx, modemmanager.JobName); err != nil {
			testing.ContextLogf(ctx, "Failed to restart job: %q, %s", modemmanager.JobName, err)
		}
	}(clearAttachCtx)
	defer cancel()

	maxActiveBearers, err := modem.GetMaxActiveMultiplexedBearers(ctx, modem)
	if err != nil {
		s.Fatal("Failed to get MaxActiveMultiplexedBearers: ", err)
	}

	var bearerPaths = []dbus.ObjectPath{}
	usedApns := map[string]struct{}{}
	for _, knownAPN := range knownAPNs {
		mmApnInfo := knownAPN.GetAPNForModemManager()
		apnName := mmApnInfo[mmconst.BearerPropertyApn].(string)
		// Check if the APN was already used, since there are multiple entries per APN name.
		if _, exists := usedApns[apnName]; exists {
			continue
		}
		// Set the multiplex property
		mmApnInfo[mmconst.BearerPropertyMultiplex] = mmconst.BearerMultiplexSupportRequested
		testing.ContextLogf(ctx, "Creating connection number: %d with APN: %q", len(bearerPaths)+1, mmApnInfo)
		mmApnInfo[mmconst.BearerPropertyApnType] = mmconst.BearerAPNTypeDefault
		path, err := modemmanager.Connect(ctx, simpleModem, mmApnInfo)
		if err != nil {
			if len(bearerPaths) == 1 {
				err = cellular.TagKnownBugOnModemType(ctx, err, "b/249388479", []cellularconst.ModemType{cellularconst.ModemTypeNL668})
				err = cellular.TagKnownBugOnModemType(ctx, err, "b/236295106", []cellularconst.ModemType{cellularconst.ModemTypeFM350})
			}
			s.Fatal("Modem connect failed with error: ", err)
		}
		testing.ContextLogf(ctx, "Created connection #%d with APN: %q on bearer: %q", len(bearerPaths)+1, apnName, path)
		bearerPaths = append(bearerPaths, path)
		usedApns[apnName] = struct{}{}
		// Check that all bearers are still connected.
		for _, bearerPath := range bearerPaths {
			bearerProps, err := modem.GetBearerProperties(ctx, bearerPath)
			if err != nil {
				s.Fatal("Failed to read bearer properties: ", err)
			}
			connected, err := bearerProps.GetBool(mmconst.BearerPropertyConnected)
			if err != nil {
				s.Fatal("Missing connected property: ", err)
			}
			if !connected {
				s.Fatalf("Bearer %q is not connected: %q", bearerPath, err)
			}
		}
		// The list of known APNs could be larger than maxActiveBearers. In that case, the test
		// should not try to create more bearers than the maximum supported by the modem.
		if uint32(len(bearerPaths)) >= maxActiveBearers {
			break
		}
	}
}
