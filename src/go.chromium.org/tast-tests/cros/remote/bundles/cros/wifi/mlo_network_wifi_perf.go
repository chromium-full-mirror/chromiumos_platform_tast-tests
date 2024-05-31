// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil/perfmanager"
	iperf "go.chromium.org/tast-tests/cros/remote/network/iperf"
	"go.chromium.org/tast-tests/cros/remote/updateutil"
	remoteiw "go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/mtk"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/mtk/wireless"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type mloNetworkWifiPerfTestcase struct {
	apConfig           wireless.Config
	secConfFac         security.ConfigFactory
	powerSave          bool
	shouldTputRequired bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: MLONetworkWifiPerf,
		Desc: "Measure and verify the maximal receiving and transmitting throughput for MLO network",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"kaidong@google.com",              // Test author
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName, "tast.cros.autoupdate.UpdateService"},
		Requirements:    []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		Timeout:         time.Minute * 60,
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Name:    "2g5g",
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
				Val: mloNetworkWifiPerfTestcase{
					apConfig: wireless.Config{
						DeviceConfigs: []wireless.DeviceConfig{wireless.WifiDeviceConfig2G, wireless.WifiDeviceConfig5G},
						IfaceConfigs:  []wireless.IfaceConfig{wireless.WifiIfaceConfig2G, wireless.WifiIfaceConfig5G, wireless.WifiIfaceConfig6G},
						MldConfigs: []wireless.MldConfig{{Name: "apmld1", Disabled: false, Mode: "ap", Ifaces: []string{wireless.WiFiIface2G, wireless.WiFiIface5G},
							SSID: hostapd.RandomSSID("MLO_"), Encryption: "sae", Key: "chromeos", Ieee80211w: 1}},
					},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211be()),
			},
			{
				Name:    "2g6g",
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
				Val: mloNetworkWifiPerfTestcase{
					apConfig: wireless.Config{
						DeviceConfigs: []wireless.DeviceConfig{wireless.WifiDeviceConfig2G, wireless.WifiDeviceConfig6G},
						IfaceConfigs:  []wireless.IfaceConfig{wireless.WifiIfaceConfig2G, wireless.WifiIfaceConfig5G, wireless.WifiIfaceConfig6G},
						MldConfigs: []wireless.MldConfig{{Name: "apmld1", Disabled: false, Mode: "ap", Ifaces: []string{wireless.WiFiIface2G, wireless.WiFiIface6G},
							SSID: hostapd.RandomSSID("MLO_"), Encryption: "sae", Key: "chromeos", Ieee80211w: 1}},
					},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211be()),
			},
			{
				Name:    "5g6g",
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
				Val: mloNetworkWifiPerfTestcase{
					apConfig: wireless.Config{
						DeviceConfigs: []wireless.DeviceConfig{wireless.WifiDeviceConfig5G, wireless.WifiDeviceConfig6G},
						IfaceConfigs:  []wireless.IfaceConfig{wireless.WifiIfaceConfig2G, wireless.WifiIfaceConfig5G, wireless.WifiIfaceConfig6G},
						MldConfigs: []wireless.MldConfig{{Name: "apmld1", Disabled: false, Mode: "ap", Ifaces: []string{wireless.WiFiIface5G, wireless.WiFiIface6G},
							SSID: hostapd.RandomSSID("MLO_"), Encryption: "sae", Key: "chromeos", Ieee80211w: 1}},
					},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211be()),
			},
			{
				Name:    "2g5g6g",
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
				Val: mloNetworkWifiPerfTestcase{
					apConfig: wireless.Config{
						DeviceConfigs: []wireless.DeviceConfig{wireless.WifiDeviceConfig2G, wireless.WifiDeviceConfig5G, wireless.WifiDeviceConfig6G},
						IfaceConfigs:  []wireless.IfaceConfig{wireless.WifiIfaceConfig2G, wireless.WifiIfaceConfig5G, wireless.WifiIfaceConfig6G},
						MldConfigs: []wireless.MldConfig{{Name: "apmld1", Disabled: false, Mode: "ap", Ifaces: []string{wireless.WiFiIface2G, wireless.WiFiIface5G, wireless.WiFiIface6G},
							SSID: hostapd.RandomSSID("MLO_"), Encryption: "sae", Key: "chromeos", Ieee80211w: 1}},
					},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211be()),
			},
		},
	})
}

func MLONetworkWifiPerf(ctx context.Context, s *testing.State) {
	/*
		This test measures MLO WiFi Throughput Performance:
		* Read the board name which is used to get the expected throughput.
		1- Configure AP with the apConfig and secConfFac.
		2- Connect DUT to the AP.
		3- Create perf.NewTestManager
		4- Set powersaving mode on the DUT.
		5- Assure no disconnecton during the test.
		6- Iterate through the test types in []perfTestTypes{}.
			a) Get the expected throughput based on the router type, test type, connection mode, and channel width.
			b) Run iperf session.
		7- Verify the throughtput results.
			a) If the board has boardMaxExpectation and it's lower than mustExpectedThroughput, then use boardMaxExpectation for verifying the must throughput.
			b) If shouldTputRequired is true and measured throughtput is lower than shouldExpectedThroughput, then add failing logs to lowThroughputTests.
			c) Else If measured throughtput lower than mustExpectedThroughput then add failing logs to lowThroughputTests.
		8- Fail the the test if lowThroughputTests is not empty.
	*/

	tf := s.FixtValue().(*wificell.TestFixture)
	tc := s.Param().(mloNetworkWifiPerfTestcase)
	rt, ok := tf.Router().(*mtk.Router)
	if !ok {
		s.Fatal("Router used is not the MTK 7988 router")
	}

	var perfTestTypes = []perfmanager.TestType{
		perfmanager.TestTypeTCPTx,
		perfmanager.TestTypeTCPRx,
		perfmanager.TestTypeTCPBidirectional,
		perfmanager.TestTypeUDPTx,
		perfmanager.TestTypeUDPRx,
		perfmanager.TestTypeUDPBidirectional,
	}

	routerType := tf.Router().RouterType()
	var lowThroughputTests []string

	// Read the current board name from the lsb file
	// for the MaxExpectedThroughputForBoard.
	var board string
	lsbContent := map[string]string{
		lsbrelease.Board: "",
	}
	if err := updateutil.FillFromLSBRelease(ctx, s.DUT(), s.RPCHint(), lsbContent); err != nil {
		s.Fatal("Failed to read from lsb file: ", err)
	}
	// Builder path is used in selecting the update image.
	board = lsbContent[lsbrelease.Board]

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()

	logPerfValues := func(label string, values []float64, dir perf.Direction, multi bool) {
		pv.Set(perf.Metric{
			Name:      label,
			Unit:      "Mbps",
			Direction: dir,
			Multiple:  multi,
		}, values...)
		s.Logf("%s: %v", label, values)
	}

	perfKeyVal, err := wifiutil.NewKeyValsFile(s.OutDir())
	if err != nil {
		s.Error("Failed to create keyval file, err: ", err)
	}
	defer perfKeyVal.Close()

	// Disable background scan which causes severe udp_rx throughput drops that can cause
	// a disconnection during perf tests. Refer to b/315880821.
	ctx, restoreBgAndFg, err := tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	ws, err := rt.StartWireless(ctx, "", &tc.apConfig)
	if err != nil {
		s.Fatal("Failed to configure ap, err: ", err)
	}
	defer func(ctx context.Context) {
		if err := rt.StopWireless(ctx, ws); err != nil {
			s.Error("Failed to deconfig ap, err: ", err)
		}
	}(ctx)
	ctx, cancel := ws.ReserveForClose(ctx)
	defer cancel()
	s.Log("AP setup done")

	defer func(ctx context.Context) {
		req := &wifi.DeleteEntriesForSSIDRequest{Ssid: []byte(ws.Config().MldConfigs[0].SSID)}
		if _, err := tf.WifiClient().DeleteEntriesForSSID(ctx, req); err != nil {
			s.Errorf("Failed to remove entries for ssid=%s, err: %v", ws.Config().MldConfigs[0].SSID, err)
		}
	}(ctx)

	securityConfig, err := tc.secConfFac.Gen()
	if err != nil {
		s.Fatal("Fail to generate scurity config")
	}
	opts := dutcfg.ConnSecurity(securityConfig)
	if _, err := tf.ConnectWifiFromDUT(ctx, wificell.DefaultDUT, ws.Config().MldConfigs[0].SSID, opts); err != nil {
		s.Fatal("Failed to connect to WiFi, err: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi, err: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	s.Log("Connected")

	clientIP, err := tf.ClientIPv4Addrs(ctx)
	if err != nil || len(clientIP) == 0 {
		s.Fatal("Failed to get client IP address, err: ", err)
	}
	clientIface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get client interface, err: ", err)
	}
	manager, err := perfmanager.NewTestManager(ctx, tf.DUTConn(wificell.DefaultDUT), tf.APConn(), tf.APConnByID(1), routerType, clientIP[0].String(), wireless.DefaultGatewayIP, clientIface)
	if err != nil {
		s.Fatal("Failed to get performance test manager, err: ", err)
	}
	defer func(ctx context.Context) error {
		if err := manager.Close(ctx); err != nil {
			s.Error("Failed to Close Perf Test Manager, err: ", err)
		}
		return nil
	}(ctx)
	ctx, cancel = ctxutil.Shorten(ctx, 2*time.Second)

	iwr := remoteiw.NewRemoteRunner(s.DUT().Conn())
	psMode, err := iwr.PowersaveMode(ctx, clientIface)
	if err != nil {
		s.Error("Failed to get the powersave mode of the WiFi interface, err: ", err)
	}
	defer func(ctx context.Context) error {
		if err := iwr.SetPowersaveMode(ctx, clientIface, psMode); err != nil {
			s.Errorf("Failed to set the powersave mode %t: %v", psMode, err)
		}
		return nil
	}(ctx)
	ctx, cancel = ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()
	if err := iwr.SetPowersaveMode(ctx, clientIface, tc.powerSave); err != nil {
		s.Fatalf("Failed to set the powersave mode %t: %v", tc.powerSave, err)
	}

	// Create apConfigTag which is used in the keyval
	var allTags []string
	var apConfigTag string
	psModeStr := "on"
	if !tc.powerSave {
		psModeStr = "off"
	}
	psTag := fmt.Sprintf("PS%s", psModeStr)
	allTags = append(allTags, psTag)
	apConfigDesc := "MLO"
	allTags = append(allTags, apConfigDesc)
	allTags = append(allTags, tf.Router().RouterModel())
	apConfigTag = strings.Join(allTags, "_")

	signalLevel, err := iwr.WifiInterfaceSignalLevel(ctx, clientIface)
	if err != nil {
		s.Error("Failed to get the singal level of the WiFi interface, err: ", err)
	}
	signalDesc := apConfigDesc + "_signal{perf}"
	// Write perf keyval
	perfKeyVal.WriteKeyVals(map[string]string{signalDesc: signalLevel})

	doRun := func(ctx context.Context) error {
		channelWidth := maxChannelWidth(tc.apConfig)
		for _, testType := range perfTestTypes {
			s.Logf("Performing [[ %s ]]", testType)
			config, err := manager.Config(routerType, testType, 0)
			if err != nil {
				return errors.Wrapf(err, "failed to get the iperf/netperf configuration for test type %s", testType)
			}
			session, err := manager.Session(ctx, testType)
			if err != nil {
				return errors.Wrapf(err, "failed to get the iperf/netperf session for test type %s", testType)
			}
			expectedThrougput, err := perfmanager.ExpectedThroughputWiFi(routerType, testType, hostapd.Mode80211bePure, channelWidth)
			if err != nil {
				return errors.Wrap(err, "failed to get expected throughput")
			}
			finalResult, results, err := session.Run(ctx, config)
			if err != nil {
				return errors.Wrap(err, "failed to run session")
			}
			if len(results) == 0 {
				return errors.Errorf("failed to take measurement for %s", testType)
			}
			var values []iperf.BitRate
			for _, sample := range results {
				values = append(values, sample.Throughput/iperf.Mbps)
			}
			logPerfValues(fmt.Sprintf("%s.%s_dev", apConfigTag, testType), []float64{float64(finalResult.StdDeviation / iperf.Mbps)}, perf.SmallerIsBetter, false)
			failedResults := perfmanager.VerifyResults(ctx, float64(finalResult.Throughput/iperf.Mbps), expectedThrougput.Must, expectedThrougput.Should, testType, tc.powerSave, tc.shouldTputRequired, tc.apConfig.DeviceConfigs[0].Channel, board)
			if failedResults != "" {
				lowThroughputTests = append(lowThroughputTests, failedResults)
			}
			valuesFloat64 := make([]float64, len(values))
			for i, v := range values {
				valuesFloat64[i] = float64(v)
			}
			logPerfValues(fmt.Sprintf("%s.%s", apConfigTag, testType), valuesFloat64, perf.BiggerIsBetter, true)
			perfKeyVal.WriteKeyVals(map[string]string{fmt.Sprintf("%s_%s__throughput{perf}", apConfigTag, testType): fmt.Sprintf("%0.2f+-%0.2f", finalResult.Throughput/iperf.Mbps, finalResult.StdDeviation/iperf.Mbps)})
			perfKeyVal.WriteKeyVals(map[string]string{fmt.Sprintf("%s_%s__dev{perf}", apConfigTag, testType): fmt.Sprintf("%f", finalResult.StdDeviation/iperf.Mbps)})
		}
		return nil
	}
	if err := tf.AssertNoDisconnect(ctx, wificell.DefaultDUT, doRun); err != nil {
		s.Error("Failed run performance test, err: ", err)
	}
	s.Log("Deconfiguring")

}

func maxChannelWidth(apConfig wireless.Config) hostapd.ChWidthEnum {
	var maxChannelWidth hostapd.ChWidthEnum
	for _, deviceConfig := range apConfig.DeviceConfigs {
		channelWidth := deviceConfig.ChannelWidth()
		if channelWidth != hostapd.ChWidthUnknown && channelWidth > maxChannelWidth {
			maxChannelWidth = channelWidth
		}
	}
	return maxChannelWidth
}
