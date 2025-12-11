// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	shillStart        = "seconds_kernel_to_shill_start"
	wifiRegistered    = "seconds_kernel_to_wifi_registered"
	wifiAssociating   = "seconds_kernel_to_wifi_association"
	wifiConfiguring   = "seconds_kernel_to_wifi_configuration"
	wifiConfigured    = "seconds_kernel_to_wifi_ready"
	wifiOnline        = "seconds_kernel_to_wifi_online"
	patchpanelStart   = "seconds_kernel_to_patchpanel_start"
	patchpanelStarted = "seconds_kernel_to_patchpanel_started"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ConnectionLatencyPerf,
		Desc: "Measures Wi-Fi (auto)-connection latency since boot",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func", "wificell_unstable"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName, "tast.cros.platform.BootPerfService"},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func ConnectionLatencyPerf(ctx context.Context, s *testing.State) {
	// This test simulates user experience from DUT boot to WiFi connected, by
	// measuring time profiles of multiple connection states from kernel boot.
	// 1. Connect to the AP to ensure the network profile is saved by shill.
	// 2. Reboot the DUT.
	// 3. Wait for the DUT to automatically reconnect to the saved network.
	// 4. Collect WiFi boot performance time measures and upload to crosbolt.
	tf := s.FixtValue().(*wificell.TestFixture)

	// Reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Configure the AP.
	apOpts := []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)}
	ap, err := tf.ConfigureAP(ctx, apOpts, nil)
	if err != nil {
		s.Fatal("Failed to configure AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfigure AP: ", err)
		}
	}(cleanupCtx)
	s.Log("AP configured")

	// Connect to the network. Shill will save the profile.
	if _, err := tf.ConnectWifiAP(ctx, ap); err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	s.Log("Connected to WiFi")

	// Reboot the DUT. The fixture will automatically re-establish the rpc.
	s.Log("Rebooting DUT")
	if err := tf.RebootDUT(ctx, wificell.DefaultDUT); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	// Wait until device is auto-connected to the saved network.
	// There's no lab Internet through AP so it won't reach the online state.
	states := []string{
		shillconst.ServiceStateAssociation,
		shillconst.ServiceStateConfiguration,
		shillconst.ServiceStateReady,
	}
	if err := tf.DUTWifiClient(wificell.DefaultDUT).WaitForWiFiServiceStates(ctx, ap.Config().SSID, states); err != nil {
		s.Fatal("DUT: failed to wait for WiFi auto-connection: ", err)
	}
	s.Log("WiFi autoconnected")
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}(cleanupCtx)

	// Get Boot Perf Metrics.
	bootPerfService := platform.NewBootPerfServiceClient(tf.RPC().Conn)
	m, err := bootPerfService.GetWiFiBootPerfMetrics(ctx, &empty.Empty{})
	if err != nil {
		s.Error("Failed to get boot performance metrics: ", err)
		return
	}
	pv := perf.NewValues()
	eventMetrics := []struct {
		Event    string
		Required bool
	}{
		{shillStart, true},
		{wifiRegistered, true},
		{wifiAssociating, true},
		{wifiConfiguring, true},
		{wifiConfigured, true},
		{wifiOnline, false},
		{patchpanelStart, true},
		{patchpanelStarted, true},
	}
	for _, event := range eventMetrics {
		if timeMeasure, ok := m.GetMetrics()[event.Event]; !ok {
			if event.Required {
				s.Errorf("Event metric %s not found in WiFi boot perf metrics", event.Event)
				return
			}
			s.Logf("Event metric %s not found in WiFi boot perf metrics, skipping", event.Event)
		} else {
			s.Logf("Duration for %s: %f", event.Event, timeMeasure)
			pv.Append(perf.Metric{
				Name:      event.Event,
				Unit:      "seconds",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
			}, timeMeasure)
		}
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf data: ", err)
	}
}
