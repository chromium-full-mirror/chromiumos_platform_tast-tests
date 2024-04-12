// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// RoamTest holds all variables to be accessible for the whole roam test.
type RoamTest struct {
	tf             *wificell.TestFixture
	restoreBgAndFg func() error
	servicePath    string
	roamSucceeded  bool
	ap1            *wificell.APIface
	ap2            *wificell.APIface
}

// SimpleRoamInitialSetup sets up AP1, connects the DUT to it, then sets up AP2.
// Background and foreground scans are disabled and the ScanAllowRoam property
// is set to the specified value as part of this test setup.
func SimpleRoamInitialSetup(ctx context.Context, tf *wificell.TestFixture, DUTIdx wificell.DutIdx, ap1Config, ap2Config hostapd.ApConfig, scanAllowRoam bool) (context.Context, *RoamTest, DestructorStackDestroyF, error) {
	rt := &RoamTest{tf: tf}
	ds, destroyIfNotExported := newDestructorStack()
	defer destroyIfNotExported()

	// Turn off background and foreground scans to prevent unwanted discovery of
	// APs.
	var err error
	ctx, rt.restoreBgAndFg, err = rt.tf.DUTWifiClient(DUTIdx).TurnOffBgAndFgscan(ctx)
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
	allowRoamResp, err := tf.DUTWifiClient(DUTIdx).GetScanAllowRoamProperty(ctx, &empty.Empty{})
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to get the ScanAllowRoam property")
	}
	if allowRoamResp.Allow != scanAllowRoam {
		if _, err := tf.DUTWifiClient(DUTIdx).SetScanAllowRoamProperty(ctx, &wifi.SetScanAllowRoamPropertyRequest{Allow: scanAllowRoam}); err != nil {
			return ctx, nil, nil, errors.Wrapf(err, "failed to set the ScanAllowRoam property to %v", scanAllowRoam)
		}
		ds.push(func() (err error) {
			if _, err := tf.DUTWifiClient(DUTIdx).SetScanAllowRoamProperty(ctx, &wifi.SetScanAllowRoamPropertyRequest{Allow: allowRoamResp.Allow}); err != nil {
				return errors.Wrapf(err, "failed to set the ScanAllowRoam property back to %v", allowRoamResp.Allow)
			}
			return nil
		})
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

	// Connect to the initial AP.
	resp, err := tf.ConnectWifiAPFromDUT(ctx, DUTIdx, rt.ap1)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "DUT: failed to connect to WiFi")
	}
	rt.servicePath = resp.ServicePath
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

	if err := tf.VerifyConnectionFromDUT(ctx, 0, rt.ap1); err != nil {
		return ctx, nil, nil, errors.Wrap(err, "DUT: failed to verify connection")
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
	return rt.servicePath
}
