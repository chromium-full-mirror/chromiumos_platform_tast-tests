// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/wifi/regdb"
	"go.chromium.org/tast-tests/cros/local/network/iw"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/testing/wlan"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Regulatory,
		// Test notes: We don't verify that the system truly respects the regulatory database rules, but only that it does not
		// reject them. Note that some WiFi drivers "self manage" their domain detection and so this test can't apply everywhere.
		Desc: "Ensure the regulatory database is coherent and that we can switch domains using the 'iw' utility",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		// This test doesn't technically require the wificell fixture, but it's best if non-default regulatory settings are used
		// only in RF chambers.
		Attr:         []string{"group:wificell", "wificell_func"},
		SoftwareDeps: []string{"wifi"},
		// TODO(b/192693354, b/155410645): StP2 + 3.18 doesn't have self-managed regdomain, skip the remaining board before uprev is finished.
		HardwareDeps:    hwdep.D(hwdep.SkipOnPlatform("asuka", "sentry")),
		Requirements:    []string{tdreq.WiFiRegSupportNL80211CMD, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func Regulatory(ctx context.Context, s *testing.State) {
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}

	iface, err := shill.WifiInterface(ctx, manager, 5*time.Second)
	if err != nil {
		s.Fatal("Could not get a WiFi interface: ", err)
	}
	s.Log("WiFi interface: ", iface)

	devInfo, err := wlan.DeviceInfo()
	if err != nil {
		s.Fatal("Failed to get device info: ", err)
	}

	// List of self-managed platforms
	selfManagedDevices := []wlan.DeviceID{
		wlan.Intel7260,
		wlan.Intel7265,
		wlan.Intel9000,
		wlan.Intel9260,
		wlan.Intel22260,
		wlan.Intel22560,
		wlan.IntelAX203,
		wlan.IntelAX211,
		wlan.QualcommWCN6855,
		wlan.QualcommWCN6750,
	}

	expectSelfManaged := false
	for _, id := range selfManagedDevices {
		if id == devInfo.ID {
			expectSelfManaged = true
			break
		}
	}

	iwr := iw.NewLocalRunner()
	if selfManaged, err := iwr.IsRegulatorySelfManaged(ctx); err != nil {
		s.Fatal("Failed to retrieve regulatory status: ", err)
	} else if selfManaged != expectSelfManaged {
		s.Fatalf("Unexpected self-managed value of wiphy, got %t, want %t", selfManaged, expectSelfManaged)
	}

	// If the wiphy is self-managed, we don't need later tests.
	if expectSelfManaged {
		return
	}

	initialDomain, err := iwr.RegulatoryDomain(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve domain: ", err)
	}
	defer func(ctx context.Context) {
		err := iwr.SetRegulatoryDomain(ctx, initialDomain)
		if err != nil {
			s.Error("Failed to reset domain: ", err)
		}
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	db, err := regdb.NewRegulatoryDB()
	if err != nil {
		s.Fatal("Failed to retrieve regulatory database: ", err)
	}

	for i, c := range db.Countries {
		s.Logf("Country %d = %s", i, c.Alpha)
		err := iwr.SetRegulatoryDomain(ctx, c.Alpha)
		if err != nil {
			s.Fatalf("Failed to set country code %s: %v", c.Alpha, err)
		}

		// The kernel processes changes asynchronously, so poll for a short time.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			dom, err := iwr.RegulatoryDomain(ctx)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to retrieve domain"))
			}
			if dom != c.Alpha {
				return errors.Errorf("unexpected country: %q != %q", dom, c.Alpha)
			}
			return nil
		}, &testing.PollOptions{Timeout: time.Second}); err != nil {
			s.Error("Failed to change domains: ", err)
		}
	}
}
