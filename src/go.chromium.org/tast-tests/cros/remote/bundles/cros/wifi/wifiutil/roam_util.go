// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// BSSTMRequestTimeout is the waiting for roaming property timeout.
	BSSTMRequestTimeout = 30 * time.Second
)

// RoamTest holds all variables to be accessible for the whole roam test.
type RoamTest struct {
	tf             *wificell.TestFixture
	restoreBgAndFg func() error
	servicePath    []string
	roamSucceeded  bool
	ap1            *wificell.APIface
	ap2            *wificell.APIface
}

// SimpleRoamInitialSetup sets up AP1, connects DUTs to it, then sets up AP2.
// Background and foreground scans are disabled and the ScanAllowRoam property
// is set to the specified value as part of this test setup.
func SimpleRoamInitialSetup(ctx context.Context, tf *wificell.TestFixture, duts []wificell.DutIdx, ap1Config, ap2Config hostapd.ApConfig, scanAllowRoam bool) (context.Context, *RoamTest, DestructorStackDestroyF, error) {
	rt := &RoamTest{tf: tf}
	ds, destroyIfNotExported := newDestructorStack()
	defer destroyIfNotExported()

	// Turn off background and foreground scans to prevent unwanted discovery of
	// APs.
	var err error
	for _, index := range duts {
		ctx, rt.restoreBgAndFg, err = rt.tf.DUTWifiClient(index).TurnOffBgAndFgscan(ctx)
		if err != nil {
			return ctx, nil, nil, errors.Wrap(err, "failed to turn off the background and/or foreground scan")
		}
		ds.push(func() (err error) {
			if err := rt.restoreBgAndFg(); err != nil {
				return errors.Wrap(err, "failed to restore the background and/or foreground scan config")
			}
			return nil
		})

		// Set ScanAllowRoam property to scanAllowRoam.
		allowRoamResp, err := tf.DUTWifiClient(index).GetScanAllowRoamProperty(ctx, &empty.Empty{})
		if err != nil {
			return ctx, nil, nil, errors.Wrap(err, "failed to get the ScanAllowRoam property")
		}
		if allowRoamResp.Allow != scanAllowRoam {
			if _, err := tf.DUTWifiClient(index).SetScanAllowRoamProperty(ctx, &wifi.SetScanAllowRoamPropertyRequest{Allow: scanAllowRoam}); err != nil {
				return ctx, nil, nil, errors.Wrapf(err, "failed to set the ScanAllowRoam property to %v", scanAllowRoam)
			}
			ds.push(func() (err error) {
				if _, err := tf.DUTWifiClient(index).SetScanAllowRoamProperty(ctx, &wifi.SetScanAllowRoamPropertyRequest{Allow: allowRoamResp.Allow}); err != nil {
					return errors.Wrapf(err, "failed to set the ScanAllowRoam property back to %v", allowRoamResp.Allow)
				}
				return nil
			})
		}
	}

	// Generate BSSIDs for the two APs.
	mac1, err := hostapd.RandomMAC()
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to generate BSSID")
	}
	mac2, err := hostapd.RandomMAC()
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to generate BSSID")
	}
	ap1BSSID := mac1.String()
	ap2BSSID := mac2.String()

	// Configure the initial AP.
	ap1Config.ApOpts = append(ap1Config.ApOpts, hostapd.BSSID(ap1BSSID))
	rt.ap1, err = tf.ConfigureAP(ctx, ap1Config.ApOpts, ap1Config.SecConfFac)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to configure the AP")
	}
	ds.push(func() (err error) {
		if err := tf.DeconfigAP(ctx, rt.ap1); err != nil {
			return errors.Wrap(err, "failed to deconfig the AP")
		}
		return nil
	})
	testing.ContextLog(ctx, "Setup the first AP")

	ap1SSID := rt.ap1.Config().SSID
	rt.servicePath = make([]string, len(duts))
	// Connect to the initial AP.
	for _, index := range duts {
		resp, err := tf.ConnectWifiAPFromDUT(ctx, index, rt.ap1)
		if err != nil {
			return ctx, nil, nil, errors.Wrap(err, "DUT: failed to connect to WiFi")
		}
		rt.servicePath[index] = resp.ServicePath
		rt.roamSucceeded = false
		ds.push(func() (err error) {
			if rt.roamSucceeded {
				return nil
			}
			if err := tf.CleanDisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
				return errors.Wrap(err, "failed to disconnect WiFi")
			}
			return nil
		})

		if err := tf.VerifyConnectionFromDUT(ctx, index, rt.ap1); err != nil {
			return ctx, nil, nil, errors.Wrap(err, "DUT: failed to verify connection")
		}
	}

	testing.ContextLog(ctx, "Connected to the first AP")

	// Set up the second AP on the same SSID as the first AP.
	ap2Config.ApOpts = append(ap2Config.ApOpts, hostapd.BSSID(ap2BSSID), hostapd.SSID(ap1SSID))
	rt.ap2, err = tf.ConfigureAP(ctx, ap2Config.ApOpts, ap2Config.SecConfFac)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to configure the AP")
	}
	ds.push(func() (err error) {
		if err := tf.DeconfigAP(ctx, rt.ap2); err != nil {
			return errors.Wrap(err, "failed to deconfig the AP")
		}
		return nil
	})
	testing.ContextLog(ctx, "Setup the second AP")

	return ctx, rt, ds.export().destroy, nil
}

// SetupDUTForRoaming prepares the dut for roaming.
func (rt *RoamTest) SetupDUTForRoaming(ctx context.Context, dut wificell.DutIdx, fromBSSID, toBSSID, testSSID string, waitForScan bool) error {
	// Get the name and MAC address of the DUT WiFi interface.
	dutIface, err := rt.tf.DUTClientInterface(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "unable to get DUT interface name")
	}
	// Flush all scanned BSS from wpa_supplicant so that test behavior is consistent.
	testing.ContextLog(ctx, "Flushing BSS cache")
	if err := rt.tf.DUTWifiClient(dut).FlushBSS(ctx, dutIface, 0); err != nil {
		return errors.Wrap(err, "failed to flush BSS list")
	}

	// Wait for roamBSSID to be discovered if waitForScan is set.
	if waitForScan {
		testing.ContextLogf(ctx, "Waiting for roamBSSID: %s", toBSSID)
		if err := rt.tf.DUTWifiClient(dut).DiscoverBSSID(ctx, toBSSID, dutIface, []byte(testSSID)); err != nil {
			return errors.Wrap(err, "Unable to discover roam BSSID")
		}
	}

	err = rt.tf.ClearBSSIDIgnoreDUT(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to clear wpa BSSID_IGNORE")
	}
	// Before sending the BSSTM request, add the current BSSID into the
	// DUT's ignorelist to avoid any any potential race condition. Adding a
	// BSSID to the ignore list does not trigger the device to roam away
	// from the BSSID, but it should prevent it from roaming back.
	// NB: Each time we add the BSSID to the ignore list, it increases
	// the duration for which the BSSID is ignored. Each wpa_cli
	// invocation results in the BSSID being added to the ignore list
	// twice, so the two calls here translate to 4 insertions in
	// wpa_supplicant, which results in an ignorelist duration of 120
	// seconds, which should be plenty.

	// We add the BSSID into the ignorelist twice purposely to ensure the
	// ignore duration is sufficient.
	err = rt.tf.AddToBSSIDIgnoreDUT(ctx, dut, fromBSSID)
	if err != nil {
		return errors.Wrap(err, "failed to add wpa BSSID_IGNORE")
	}
	err = rt.tf.AddToBSSIDIgnoreDUT(ctx, dut, fromBSSID)
	if err != nil {
		return errors.Wrap(err, "failed to add wpa BSSID_IGNORE")
	}

	return nil
}

// SendBSSTMReqAndWaitConnected sends a BSSTM Request and waits for the dut to be connected, then verifies the connections.
func (rt *RoamTest) SendBSSTMReqAndWaitConnected(ctx context.Context, dut wificell.DutIdx, fromBSSID, toBSSID string, fromAP, toAP *wificell.APIface, req hostapd.BSSTMReqParams, servicePath string, expectConnectFail bool) error {
	dutMACAddr, err := rt.tf.DUTHardwareAddr(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "Unable to get DUT MAC address")
	}
	dutMAC := dutMACAddr.String()
	// Set up a watcher for the Shill WiFi BSSID property.
	waitCtx, cancel := context.WithTimeout(ctx, BSSTMRequestTimeout)
	defer cancel()
	waitForProps, err := rt.tf.DUTWifiClient(dut).GenerateRoamPropertyWatcher(waitCtx, toBSSID, servicePath)
	// Send BSS Transition Management Request to client.
	testing.ContextLogf(ctx, "Sending BSS Transition Management Request from AP %s to DUT %s", fromBSSID, dutMAC)
	if err := fromAP.SendBSSTMRequest(ctx, dutMAC, req); err != nil {
		return errors.Wrap(err, "failed to send BSS TM Request")
	}

	// Wait for the DUT to roam to the second AP, then assert that there was
	// no disconnection during roaming.
	testing.ContextLog(ctx, "Waiting for roaming")
	monitorResult, err := waitForProps()
	if err != nil {
		if expectConnectFail {
			testing.ContextLog(ctx, "Connection failed as expected")
			return nil
		}
		return errors.Wrap(err, "failed to roam within timeout")
	}
	if expectConnectFail {
		return errors.Wrap(err, "expected roam to fail but it succeeded")
	}

	if err := VerifyNoDisconnections(monitorResult); err != nil {
		return errors.Wrap(err, "DUT: failed to stay connected during the roaming process")
	}
	// Just for good measure make sure we're properly connected.
	testing.ContextLogf(ctx, "Verifying connection to AP %s", toBSSID)
	if err := rt.tf.VerifyConnection(ctx, toAP); err != nil {
		return errors.Wrap(err, "DUT: failed to verify connection")
	}

	return nil
}

// RoamSucceeded gets the roamsucceeded property.
func (rt *RoamTest) RoamSucceeded() bool {
	return rt.roamSucceeded
}

// SetRoamSucceeded sets the roamsucceeded property.
func (rt *RoamTest) SetRoamSucceeded(roamSucceeded bool) {
	rt.roamSucceeded = roamSucceeded
}

// AP1SSID gets the SSID of AP1.
func (rt *RoamTest) AP1SSID() string {
	return rt.ap1.Config().SSID
}

// AP1BSSID gets the BSSID of AP1.
func (rt *RoamTest) AP1BSSID() string {
	return rt.ap1.Config().BSSID
}

// AP2BSSID gets the BSSID of AP2.
func (rt *RoamTest) AP2BSSID() string {
	return rt.ap2.Config().BSSID
}

// AP1 gets the AP1 interface.
func (rt *RoamTest) AP1() *wificell.APIface {
	return rt.ap1
}

// AP2 gets the AP2 interface.
func (rt *RoamTest) AP2() *wificell.APIface {
	return rt.ap2
}

// ServicePath gets the service path after connecting to AP1.
func (rt *RoamTest) ServicePath() string {
	return rt.servicePath[0]
}

// ServicePathOfDUT gets the service path after connecting to AP1.
func (rt *RoamTest) ServicePathOfDUT(index wificell.DutIdx) string {
	return rt.servicePath[index]
}
