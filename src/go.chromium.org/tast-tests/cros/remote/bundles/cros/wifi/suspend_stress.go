// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const suspendIterationTimeout = 40 * time.Second
const baseTimeout = 30 * time.Second

type suspendStressParam struct {
	suspendCount int
	// A slice of configs for colocated AP's that are controlled by one
	// hostapd.
	apConfigs []hostapd.ApConfig
	// isColocated indicates whether the 6GHz AP is co-located.
	isColocated bool
	// sameSSID indicates whether or not to put the 6GHz AP on the same SSID as the co-located AP.
	sameSSID bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: SuspendStress,
		Desc: "Asserts WiFi connectivity after suspend-resume cycle using powerd_dbus_suspend command",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates},
		Params: []testing.Param{
			{
				Name:      "80211g",
				ExtraAttr: []string{"wificell_func", "wificell_suspend"},
				Timeout:   baseTimeout + suspendIterationTimeout*15,
				Val: []suspendStressParam{
					{suspendCount: 5, apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(1), hostapd.Mode(hostapd.Mode80211g)}}}, isColocated: false},
					{suspendCount: 5, apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(6), hostapd.Mode(hostapd.Mode80211g)}}}, isColocated: false},
					{suspendCount: 5, apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(11), hostapd.Mode(hostapd.Mode80211g)}}}, isColocated: false},
				},
				ExtraRequirements: []string{tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
			},
			{
				Name:      "80211n24ht40",
				ExtraAttr: []string{"wificell_func", "wificell_suspend"},
				Timeout:   baseTimeout + suspendIterationTimeout*5,
				Val: []suspendStressParam{
					{
						suspendCount: 5,
						apConfigs:    []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(6), hostapd.Mode(hostapd.Mode80211nPure), hostapd.HTCaps(hostapd.HTCapHT40)}}},
						isColocated:  false,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
			},
			{
				Name:      "80211n5ht40",
				ExtraAttr: []string{"wificell_func", "wificell_suspend"},
				Timeout:   baseTimeout + suspendIterationTimeout*5,
				Val: []suspendStressParam{
					{
						suspendCount: 5,
						apConfigs:    []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(48), hostapd.Mode(hostapd.Mode80211nPure), hostapd.HTCaps(hostapd.HTCapHT40Minus)}}},
						isColocated:  false,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
			},
			{
				Name:      "80211acvht80",
				ExtraAttr: []string{"wificell_func", "wificell_suspend"},
				Timeout:   baseTimeout + suspendIterationTimeout*5,
				Val: []suspendStressParam{
					{
						suspendCount: 5,
						apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{
							hostapd.Channel(157), hostapd.Mode(hostapd.Mode80211acPure), hostapd.HTCaps(hostapd.HTCapHT40Plus),
							hostapd.VHTChWidth(hostapd.VHTChWidth80), hostapd.VHTCenterChannel(155), hostapd.VHTCaps(hostapd.VHTCapSGI80),
						}}},
						isColocated: false,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
			},
			{
				Name:      "hidden",
				ExtraAttr: []string{"wificell_func", "wificell_suspend"},
				Timeout:   baseTimeout + suspendIterationTimeout*15,
				Val: []suspendStressParam{
					{suspendCount: 5, apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(6), hostapd.Mode(hostapd.Mode80211g), hostapd.Hidden(), hostapd.SpectrumManagement()}}}, isColocated: false},
					{suspendCount: 5, apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(36), hostapd.Mode(hostapd.Mode80211nPure), hostapd.Hidden(), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.SpectrumManagement()}}}, isColocated: false},
					{suspendCount: 5, apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(48), hostapd.Mode(hostapd.Mode80211nPure), hostapd.Hidden(), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.SpectrumManagement()}}}, isColocated: false},
				},
				ExtraRequirements: []string{tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
			},
			{
				Name:      "wpa2",
				ExtraAttr: []string{"wificell_func", "wificell_suspend"},
				Timeout:   baseTimeout + suspendIterationTimeout*5,
				Val: []suspendStressParam{
					{
						suspendCount: 5,
						apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(1), hostapd.Mode(hostapd.Mode80211nPure), hostapd.HTCaps(hostapd.HTCapHT20)},
							SecConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP))}},
						isColocated: false,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates, tdreq.WiFiSecSupportWPA2Personal},
			},
			{
				Name:              "6ghz_psc",
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				ExtraAttr:         []string{"wificell_func", "wificell_suspend", "wificell_unstable"},
				Val: []suspendStressParam{
					{ // 6GHz AP is on a PSC-channel without legacy AP advertisement.
						suspendCount: 5,
						apConfigs: []hostapd.ApConfig{
							{
								ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.Channel(21), hostapd.HTCaps(hostapd.HTCapHT20),
									hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.OpClass(131), hostapd.PMF(hostapd.PMFRequired)},
								SecConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
							},
						},
						isColocated: false,
					},
				},
				ExtraRequirements: []string{"wifi-gen-0003-v01", "wifi-rf-0006-v01"},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax6E()),
				ExtraTestBedDeps:  []string{"wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_AX_E"},
			},
			{
				Name:              "6ghz_non_psc",
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				ExtraAttr:         []string{"wificell_func", "wificell_suspend", "wificell_unstable"},
				Val: []suspendStressParam{
					{ // 6GHz AP is on a non-PSC channel with legacy AP advertisement on a different SSID.
						suspendCount: 5,
						apConfigs: []hostapd.ApConfig{
							{
								ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.Channel(9), hostapd.HTCaps(hostapd.HTCapHT20),
									hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.OpClass(131), hostapd.PMF(hostapd.PMFRequired)},
								SecConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
							}, {
								ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(40), hostapd.SpectrumManagement()},
							},
						},
						isColocated: true,
					},
				},
				ExtraRequirements: []string{"wifi-gen-0003-v01", "wifi-rf-0006-v01"},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax6E()),
				ExtraTestBedDeps:  []string{"wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_AX_E"},
			},
			{
				Name:              "6ghz_non_psc_same_ssid",
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				ExtraAttr:         []string{"wificell_func", "wificell_suspend", "wificell_unstable"},
				Val: []suspendStressParam{
					{ // 6GHz AP is on a non-PSC channel with legacy AP advertisement on the same SSID. Expect that we connect directly to the 6GHz AP.
						suspendCount: 5,
						apConfigs: []hostapd.ApConfig{
							{
								ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.Channel(9), hostapd.HTCaps(hostapd.HTCapHT20),
									hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.OpClass(131), hostapd.PMF(hostapd.PMFRequired)},
								SecConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
							}, {
								ApOpts:     []hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(40), hostapd.SpectrumManagement(), hostapd.PMF(hostapd.PMFRequired)},
								SecConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
							},
						},
						isColocated: true,
						sameSSID:    true,
					},
				},
				ExtraRequirements: []string{"wifi-gen-0003-v01", "wifi-rf-0006-v01"},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax6E()),
				ExtraTestBedDeps:  []string{"wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_AX_E"},
			},
			{
				Name:      "stress_80211n24ht40",
				ExtraAttr: []string{"wificell_stress"},
				Timeout:   baseTimeout + suspendIterationTimeout*690,
				Val: []suspendStressParam{
					{
						suspendCount: 690,
						apConfigs:    []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(6), hostapd.Mode(hostapd.Mode80211nPure), hostapd.HTCaps(hostapd.HTCapHT40)}}},
						isColocated:  false,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiProcPassStress, tdreq.WiFiProcPassStressBeforeUpdates},
			},
			{
				Name:      "stress_wpa2",
				ExtraAttr: []string{"wificell_stress"},
				Timeout:   baseTimeout + suspendIterationTimeout*690,
				Val: []suspendStressParam{
					{
						suspendCount: 690,
						apConfigs: []hostapd.ApConfig{{ApOpts: []hostapd.Option{hostapd.Channel(1), hostapd.Mode(hostapd.Mode80211nPure), hostapd.HTCaps(hostapd.HTCapHT20)},
							SecConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP))}},
						isColocated: false,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiProcPassStress, tdreq.WiFiProcPassStressBeforeUpdates},
			},
		},
	})
}

func SuspendStress(ctx context.Context, s *testing.State) {
	/*
		This test checks a DUT's ability to connect to an AP after a
		suspend-resume cycle.
		Iterate through []suspendStressParam:
			1 - Turn off background and foreground scans.
			2 - Configure the main AP with apConfigs, which includes a colocated
			    legacy band AP in the non-PSC 6GHz test case.
			3 - For PSC 6GHz tests, configure a second AP on 5GHz to initialize
			    the DUT reg domain to US upon each suspend-resume.
			4 - Connect the DUT to the main AP.
			5 - Ping the DUT.
			6 - Iterate suspendCount number of times:
				a) Suspend the DUT.
				b) Resume the DUT and record the reconnect time.
			7 - Log the fastest, slowest and average reconnection times.
	*/

	const (
		suspendTime = 10 * time.Second
	)

	tf := s.FixtValue().(*wificell.TestFixture)

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Log("Failed to save perf data, err: ", err)
		}
	}()

	// Turn off periodic scans to ensure that the DUT successfully
	// reconnects after suspend-resume without relying on foreground scans.
	ctx, restoreBgAndFg, err := tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	testOnce := func(ctx context.Context, s *testing.State, suspendCount int, apConfigs []hostapd.ApConfig, isColocated, sameSSID bool) {
		if sameSSID {
			ssid := hostapd.RandomSSID("TAST_SUSPEND_STRESS_")
			for _, config := range apConfigs {
				config.ApOpts = append(config.ApOpts, hostapd.SSID(ssid))
			}
		}
		// Configure the main AP and the colocated legacy band AP for the
		// non-PSC test case.
		apMain, err := tf.ConfigureMultiAP(ctx, wificell.DefaultRouter, apConfigs)
		if err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, apMain); err != nil {
				s.Error("Failed to deconfig the AP: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigAP(ctx, apMain)
		defer cancel()
		s.Log("Setup the main AP")

		// For PSC 6GHz channels, spin up a non-colocated 5GHz AP for the
		// duration of the test so that the regulatory domain is set to US upon
		// each suspend-resume.
		if hostapd.Is6GHzOpClass(apMain.Config().OpClass) && !isColocated {
			ap5GHzOpts := append([]hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure),
				hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(40), hostapd.SpectrumManagement()})
			ap5GHz, err := tf.ConfigureAP(ctx, ap5GHzOpts, nil)
			if err != nil {
				s.Fatal("Failed to configure the 5 GHz AP: ", err)
			}
			defer func(ctx context.Context) {
				if err := tf.DeconfigAP(ctx, ap5GHz); err != nil {
					s.Error("Failed to deconfig the 5 GHz AP: ", err)
				}
			}(ctx)
			_, cancel := tf.ReserveForDeconfigAP(ctx, ap5GHz)
			defer cancel()
			s.Log("Setup regdomain seeding AP")
		}

		s.Log("AP setup done; Connecting to the main AP")
		if _, err := tf.ConnectWifiAP(ctx, apMain); err != nil {
			s.Fatal("Failed to connect to WiFi: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.CleanDisconnectWifi(ctx); err != nil {
				s.Error("Failed to disconnect WiFi: ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()

		if err := tf.PingFromDUT(ctx, apMain.ServerIP().String()); err != nil {
			s.Error("Failed to ping from the DUT: ", err)
		}

		s.Logf("Start suspend-resume for %d times", suspendCount)
		var connectTimes []float64
		for i := 0; i < suspendCount; i++ {
			connectTime, err := tf.WifiClient().SuspendAssertConnect(ctx, suspendTime)
			if err != nil {
				s.Error("Failed to assert WiFi connection after suspend-resume: ", err)
			} else {
				connectTimes = append(connectTimes, connectTime.Seconds())
				// Publish test progress on every 10 iterations.
				if i%10 == 0 {
					s.Logf("Suspend stress iteration count: %d", i+1)
				}
			}

			if sameSSID {
				service, err := tf.WifiClient().QueryService(ctx)
				if err != nil {
					s.Error("Failed to get the active WiFi service from DUT: ", err)
				}
				_, freqHigherBand, err := apMain.Config().OperatingBandAndFreq()
				if err != nil {
					s.Error("Failed to get the frequency from the AP config: ", err)
				}
				if service.Wifi.Frequency != uint32(freqHigherBand) {
					s.Errorf("Got frequency %d; want %d", service.Wifi.Frequency, freqHigherBand)
				}
			}
		}

		if len(connectTimes) == 0 {
			s.Error("Suspend stress finished; no successful suspend")
			return
		}

		// Calculate the fastest, slowest, and average connect time.
		fastest := math.Inf(1)
		slowest := math.Inf(-1)
		var total float64
		for _, t := range connectTimes {
			fastest = math.Min(fastest, t)
			slowest = math.Max(slowest, t)
			total += t
		}
		average := total / float64(len(connectTimes))
		s.Logf("Suspend stress finished; success rate=%d/%d; reconnect time (seconds): fastest=%f, slowest=%f, average=%f", len(connectTimes), suspendCount, fastest, slowest, average)

		name := "suspend_stress_reconnect_time_" + apMain.Config().PerfDesc()
		pv.Set(perf.Metric{
			Name:      name,
			Variant:   "Fastest",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
		}, fastest)
		pv.Set(perf.Metric{
			Name:      name,
			Variant:   "Slowest",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
		}, slowest)
		pv.Set(perf.Metric{
			Name:      name,
			Variant:   "Average",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
		}, average)
	}

	testcases := s.Param().([]suspendStressParam)
	for i, tc := range testcases {
		subtest := func(ctx context.Context, s *testing.State) {
			testOnce(ctx, s, tc.suspendCount, tc.apConfigs, tc.isColocated, tc.sameSSID)
		}
		if !s.Run(ctx, fmt.Sprintf("Testcase #%d", i), subtest) {
			// Stop if one of the subtest's parameter set fails the test.
			return
		}
	}
}
