// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"strings"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
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
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type networkWifiPerfTestCase struct {
	apOpts []ap.Option
	// If unassigned, use default security config: open network.
	secConfFac         security.ConfigFactory
	powerSave          bool
	shouldTputRequired bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: NetworkWifiPerf,
		Desc: "Measure and verify the maximal receiving and transmitting throughput on a DUT",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		// TODO(b/310040283): Promote test to stable by removing wificell_unstable attribute.
		Attr:        []string{"group:wificell", "wificell_perf", "wificell_unstable"},
		TestBedDeps: []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.autoupdate.UpdateService",
		},
		Vars:         []string{"router"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesRouters),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassPerf, tdreq.WiFiProcPassPerfBeforeUpdates},
		Timeout:      time.Minute * 20,
		Params: []testing.Param{
			{
				// Network: open HE20 802.11ax.
				Name: "he20",
				Val: []networkWifiPerfTestCase{{
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211axPure), ap.Channel(36), ap.HTCaps(ap.HTCapHT20, ap.HTCapLDPC), ap.HEChWidth(ap.HEChWidth20Or40)},
					powerSave: false, shouldTputRequired: false,
				}, {
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211axPure), ap.Channel(157), ap.HTCaps(ap.HTCapHT20, ap.HTCapLDPC), ap.HEChWidth(ap.HEChWidth20Or40)},
					powerSave: false, shouldTputRequired: false,
				}},
				ExtraAttr:         []string{"wificell_openwrt"},
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX),
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax()),
				ExtraRequirements: []string{tdreq.WiFiTputHE20TCPTx, tdreq.WiFiTputHE20TCPRx, tdreq.WiFiTputHE20TCPRxTx, tdreq.WiFiTputHE20UDPTx, tdreq.WiFiTputHE20UDPRx, tdreq.WiFiTputHE20UDPRxTx},
			},
			{
				// Network: AES encrypted HE20 802.11ax.
				Name: "he20_aes",
				Val: []networkWifiPerfTestCase{{
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211axPure), ap.Channel(36), ap.HTCaps(ap.HTCapHT20, ap.HTCapLDPC), ap.HEChWidth(ap.HEChWidth20Or40)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}, {
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211axPure), ap.Channel(157), ap.HTCaps(ap.HTCapHT20, ap.HTCapLDPC), ap.HEChWidth(ap.HEChWidth20Or40)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}},
				ExtraAttr:         []string{"wificell_openwrt"},
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX),
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax()),
				ExtraRequirements: []string{tdreq.WiFiTputHE20TCPTx, tdreq.WiFiTputHE20TCPRx, tdreq.WiFiTputHE20TCPRxTx, tdreq.WiFiTputHE20UDPTx, tdreq.WiFiTputHE20UDPRx, tdreq.WiFiTputHE20UDPRxTx},
			},
			{
				// Network: open HE40 802.11ax.
				Name: "he40",
				Val: []networkWifiPerfTestCase{{
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211axPure), ap.Channel(36), ap.HTCaps(ap.HTCapHT40, ap.HTCapLDPC), ap.HEChWidth(ap.HEChWidth20Or40)},
					powerSave: false, shouldTputRequired: false,
				}},
				ExtraAttr:         []string{"wificell_openwrt"},
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX),
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax()),
				ExtraRequirements: []string{tdreq.WiFiTputHE40TCPTx, tdreq.WiFiTputHE40TCPRx, tdreq.WiFiTputHE40TCPRxTx, tdreq.WiFiTputHE40UDPTx, tdreq.WiFiTputHE40UDPRx, tdreq.WiFiTputHE40UDPRxTx},
			},
			{
				// Network: AES encrypted HE40 802.11ax.
				Name: "he40_aes",
				Val: []networkWifiPerfTestCase{{
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211axPure), ap.Channel(36), ap.HTCaps(ap.HTCapHT40, ap.HTCapLDPC), ap.HEChWidth(ap.HEChWidth20Or40)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}},
				ExtraAttr:         []string{"wificell_openwrt"},
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX),
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax()),
				ExtraRequirements: []string{tdreq.WiFiTputHE40TCPTx, tdreq.WiFiTputHE40TCPRx, tdreq.WiFiTputHE40TCPRxTx, tdreq.WiFiTputHE40UDPTx, tdreq.WiFiTputHE40UDPRx, tdreq.WiFiTputHE40UDPRxTx},
			},
			{
				// Network: open HE80 802.11ax.
				Name: "he80",
				Val: []networkWifiPerfTestCase{{
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211axMixed), ap.Channel(44), ap.HTCaps(ap.HTCapHT40Plus, ap.HTCapLDPC), ap.VHTCaps(ap.VHTCapSGI80, ap.VHTCapMaxAMPDULenExp6), ap.HECenterChannel(42), ap.HEChWidth(ap.HEChWidth80)},
					powerSave: false, shouldTputRequired: false,
				}, {
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211axMixed), ap.Channel(157), ap.HTCaps(ap.HTCapHT40Plus, ap.HTCapLDPC), ap.VHTCaps(ap.VHTCapSGI80, ap.VHTCapMaxAMPDULenExp6), ap.HECenterChannel(155), ap.HEChWidth(ap.HEChWidth80)},
					powerSave: false, shouldTputRequired: false,
				}},
				ExtraAttr:         []string{"wificell_openwrt"},
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX),
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax()),
				ExtraRequirements: []string{tdreq.WiFiTputHE80TCPTx, tdreq.WiFiTputHE80TCPRx, tdreq.WiFiTputHE80TCPRxTx, tdreq.WiFiTputHE80UDPTx, tdreq.WiFiTputHE80UDPRx, tdreq.WiFiTputHE80UDPRxTx},
			},
			{
				// Network: WPA2/WPA3 mixed and PMF enabled HE80 802.11ax.
				Name: "he80_pmf",
				Val: []networkWifiPerfTestCase{{
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211axMixed), ap.Channel(44), ap.HTCaps(ap.HTCapHT40Plus, ap.HTCapLDPC), ap.VHTCaps(ap.VHTCapSGI80, ap.VHTCapRXLDPC, ap.VHTCapMaxAMPDULenExp6), ap.HECenterChannel(42), ap.HEChWidth(ap.HEChWidth80), ap.PMF(ap.PMFRequired)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}, {
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211axMixed), ap.Channel(157), ap.HTCaps(ap.HTCapHT40Plus, ap.HTCapLDPC), ap.VHTCaps(ap.VHTCapSGI80, ap.VHTCapRXLDPC, ap.VHTCapMaxAMPDULenExp6), ap.HECenterChannel(155), ap.HEChWidth(ap.HEChWidth80), ap.PMF(ap.PMFRequired)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}},
				ExtraAttr:         []string{"wificell_openwrt"},
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX),
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax()),
				ExtraRequirements: []string{tdreq.WiFiGenSupportPMF, tdreq.WiFiTputHE80TCPTx, tdreq.WiFiTputHE80TCPRx, tdreq.WiFiTputHE80TCPRxTx, tdreq.WiFiTputHE80UDPTx, tdreq.WiFiTputHE80UDPRx, tdreq.WiFiTputHE80UDPRxTx},
			},
			{
				// Network: open HT20 802.11n.
				Name: "ht20",
				Val: []networkWifiPerfTestCase{{
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(1), ap.HTCaps(ap.HTCapHT20)},
					powerSave: false, shouldTputRequired: false,
				}, {
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(157), ap.HTCaps(ap.HTCapHT20)},
					powerSave: false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiTputHT20UDPTx, tdreq.WiFiTputHT20UDPRx, tdreq.WiFiTputHT20UDPRxTx, tdreq.WiFiTputHT20TCPTx, tdreq.WiFiTputHT20TCPRx, tdreq.WiFiTputHT20TCPRxTx},
			},
			{
				// Network: AES encrypted HT20 802.11n.
				Name: "ht20_aes",
				Val: []networkWifiPerfTestCase{{
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(1), ap.HTCaps(ap.HTCapHT20)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}, {
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(157), ap.HTCaps(ap.HTCapHT20)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiTputHT20UDPTx, tdreq.WiFiTputHT20UDPRx, tdreq.WiFiTputHT20UDPRxTx, tdreq.WiFiTputHT20TCPTx, tdreq.WiFiTputHT20TCPRx, tdreq.WiFiTputHT20TCPRxTx},
			},
			{
				// Network: open HT40 802.11n.
				Name: "ht40",
				Val: []networkWifiPerfTestCase{{
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(1), ap.HTCaps(ap.HTCapHT40)},
					powerSave: false, shouldTputRequired: false,
				}, {
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(157), ap.HTCaps(ap.HTCapHT40Plus)},
					powerSave: false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiTputHT40UDPTx, tdreq.WiFiTputHT40UDPRx, tdreq.WiFiTputHT40UDPRxTx, tdreq.WiFiTputHT40TCPTx, tdreq.WiFiTputHT40TCPRx, tdreq.WiFiTputHT40TCPRxTx},
			},
			{
				// Network: AES encrypted HT40 802.11n.
				Name: "ht40_aes",
				Val: []networkWifiPerfTestCase{{
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(1), ap.HTCaps(ap.HTCapHT40)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}, {
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(157), ap.HTCaps(ap.HTCapHT40Plus)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiTputHT40UDPTx, tdreq.WiFiTputHT40UDPRx, tdreq.WiFiTputHT40UDPRxTx, tdreq.WiFiTputHT40TCPTx, tdreq.WiFiTputHT40TCPRx, tdreq.WiFiTputHT40TCPRxTx},
			},
			{
				// Network: open VHT20 802.11ac.
				Name: "vht20",
				Val: []networkWifiPerfTestCase{{
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211acPure), ap.Channel(36), ap.HTCaps(ap.HTCapHT20), ap.VHTChWidth(ap.VHTChWidth20Or40), ap.VHTCaps(ap.VHTCapMaxAMPDULenExp7)},
					powerSave: false, shouldTputRequired: false,
				}},
				ExtraAttr:         []string{"wificell_perf"},
				ExtraRequirements: []string{tdreq.WiFiTputVHT20UDPTx, tdreq.WiFiTputVHT20UDPRx, tdreq.WiFiTputVHT20UDPRxTx, tdreq.WiFiTputVHT20TCPTx, tdreq.WiFiTputVHT20TCPRx, tdreq.WiFiTputVHT20TCPRxTx},
			},
			{
				// Network: AES encrypted VHT20 802.11ac.
				Name: "vht20_aes",
				Val: []networkWifiPerfTestCase{{
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211acPure), ap.Channel(36), ap.HTCaps(ap.HTCapHT20), ap.VHTChWidth(ap.VHTChWidth20Or40), ap.VHTCaps(ap.VHTCapMaxAMPDULenExp7)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiTputVHT20UDPTx, tdreq.WiFiTputVHT20UDPRx, tdreq.WiFiTputVHT20UDPRxTx, tdreq.WiFiTputVHT20TCPTx, tdreq.WiFiTputVHT20TCPRx, tdreq.WiFiTputVHT20TCPRxTx},
			},
			{
				// Network: open VHT40 802.11ac.
				Name: "vht40",
				Val: []networkWifiPerfTestCase{{
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211acPure), ap.Channel(36), ap.HTCaps(ap.HTCapHT40), ap.VHTChWidth(ap.VHTChWidth20Or40), ap.VHTCaps(ap.VHTCapMaxAMPDULenExp7)},
					powerSave: false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiTputVHT40UDPTx, tdreq.WiFiTputVHT40UDPRx, tdreq.WiFiTputVHT40UDPRxTx, tdreq.WiFiTputVHT40TCPTx, tdreq.WiFiTputVHT40TCPRx, tdreq.WiFiTputVHT40TCPRxTx},
			},
			{
				// Network: AES encrypted VHT40 802.11ac.
				Name: "vht40_aes",
				Val: []networkWifiPerfTestCase{{
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211acPure), ap.Channel(36), ap.HTCaps(ap.HTCapHT40), ap.VHTChWidth(ap.VHTChWidth20Or40), ap.VHTCaps(ap.VHTCapMaxAMPDULenExp7)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiTputVHT40UDPTx, tdreq.WiFiTputVHT40UDPRx, tdreq.WiFiTputVHT40UDPRxTx, tdreq.WiFiTputVHT40TCPTx, tdreq.WiFiTputVHT40TCPRx, tdreq.WiFiTputVHT40TCPRxTx},
			},
			{
				// Network: open HE80 802.11ac.
				Name: "vht80",
				Val: []networkWifiPerfTestCase{{
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211acMixed), ap.Channel(44), ap.HTCaps(ap.HTCapHT40Plus), ap.VHTCaps(ap.VHTCapSGI80, ap.VHTCapMaxAMPDULenExp7), ap.VHTCenterChannel(42), ap.VHTChWidth(ap.VHTChWidth80)},
					powerSave: false, shouldTputRequired: false,
				}, {
					apOpts:    []ap.Option{ap.Mode(ap.Mode80211acMixed), ap.Channel(157), ap.HTCaps(ap.HTCapHT40Plus), ap.VHTCaps(ap.VHTCapSGI80, ap.VHTCapMaxAMPDULenExp7), ap.VHTCenterChannel(155), ap.VHTChWidth(ap.VHTChWidth80)},
					powerSave: false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiTputVHT80UDPTx, tdreq.WiFiTputVHT80UDPRx, tdreq.WiFiTputVHT80UDPRxTx, tdreq.WiFiTputVHT80TCPTx, tdreq.WiFiTputVHT80TCPRx, tdreq.WiFiTputVHT80TCPRxTx},
			},
			{
				// Network: WPA2/WPA3 mixed and PMF enabled VHT80 802.11ac.
				Name: "vht80_pmf",
				Val: []networkWifiPerfTestCase{{
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211acMixed), ap.Channel(44), ap.HTCaps(ap.HTCapHT40Plus), ap.VHTCaps(ap.VHTCapSGI80, ap.VHTCapMaxAMPDULenExp7), ap.VHTCenterChannel(42), ap.VHTChWidth(ap.VHTChWidth80), ap.PMF(ap.PMFRequired)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}, {
					apOpts:     []ap.Option{ap.Mode(ap.Mode80211acMixed), ap.Channel(157), ap.HTCaps(ap.HTCapHT40Plus), ap.VHTCaps(ap.VHTCapSGI80, ap.VHTCapMaxAMPDULenExp7), ap.VHTCenterChannel(155), ap.VHTChWidth(ap.VHTChWidth80), ap.PMF(ap.PMFRequired)},
					secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					powerSave:  false, shouldTputRequired: false,
				}},
				ExtraRequirements: []string{tdreq.WiFiGenSupportPMF, tdreq.WiFiTputVHT80UDPTx, tdreq.WiFiTputVHT80UDPRx, tdreq.WiFiTputVHT80UDPRxTx, tdreq.WiFiTputVHT80TCPTx, tdreq.WiFiTputVHT80TCPRx, tdreq.WiFiTputVHT80TCPRxTx},
			},
		},
	})
}

func NetworkWifiPerf(ctx context.Context, s *testing.State) {
	/*
		This test measures WiFi Throughput Performance:
		* Read the board name which is used to get the expected throughput.
		* Iterate through the subtests in []networkWiFiPerfTestCase{}.
		1- Configure AP with the apOpts and secConfFac.
		2- Connect DUT to the AP.
		3- Create perf.NewTestManager
			a) Assure router type is routerSupport.LegacyT (Gale).
			b) Configure 3 way setup.
			c) Verify Ethernet connection between Router and Pcap.
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

	var perfTestTypes = []perfmanager.TestType{
		perfmanager.TestTypeTCPTx,
		perfmanager.TestTypeTCPRx,
		perfmanager.TestTypeUDPTx,
		perfmanager.TestTypeUDPRx,
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

	// Verify that performance test result passes the must and should throughput requirements.
	verifyResults := func(ctx context.Context, result, mustExpectedThroughput, shouldExpectedThroughput float64, testType perfmanager.TestType, powerSave, shouldTputRequired bool, channel int, board string) string {
		mustTputFailed := false
		shouldTputFailed := false
		var failedPerformanceTest string
		// If the must requirement is greater than our maximum expectation for a
		// board, use the maximum expectation instead of the must requirement.
		// get the board name using the package "go.chromium.org/tast/core/lsbrelease"
		boardMaxExpectation, found := perfmanager.MaxExpectedThroughputForBoard(board, testType)
		if found && boardMaxExpectation < mustExpectedThroughput {
			mustExpectedThroughput = boardMaxExpectation
		}

		if result < mustExpectedThroughput {
			s.Logf("Throughput is too low for %s. Expected (must) %0.2f Mbps, got %0.2f", testType, mustExpectedThroughput, result)
			mustTputFailed = true
		}

		if result < shouldExpectedThroughput {
			s.Logf("Throughput is too low for %s. Expected (should) %0.2f Mbps, got %0.2f", testType, shouldExpectedThroughput, result)
			if shouldTputRequired {
				shouldTputFailed = true
			}
		}

		if mustTputFailed || shouldTputFailed {
			failedTests := []string{fmt.Sprintf("[test_type=%s, channel=%d, power_save_on=%t, measured_Tput=%0.2f", testType, channel, powerSave, result)}
			if mustTputFailed {
				failedTests = append(failedTests, fmt.Sprintf("must_expected_Tput_failed=%0.2f", mustExpectedThroughput))
			} else if shouldTputFailed {
				failedTests = append(failedTests, fmt.Sprintf("should_expected_Tput_failed=%0.2f", shouldExpectedThroughput))
			}
			failedPerformanceTest = strings.Join(failedTests, ",") + "]"
		}
		return failedPerformanceTest
	}

	testOnce := func(ctx context.Context, s *testing.State, options []ap.Option, fac security.ConfigFactory, powerSave, shouldTputRequired bool, boardName string) {
		apIface, err := tf.ConfigureAP(ctx, options, fac)
		if err != nil {
			s.Fatal("Failed to configure ap, err: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, apIface); err != nil {
				s.Error("Failed to deconfig ap, err: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigAP(ctx, apIface)
		defer cancel()
		s.Log("AP setup done")

		_, err = tf.ConnectWifiAP(ctx, apIface)
		if err != nil {
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
		manager, err := perfmanager.NewTestManager(ctx, tf.DUTConn(wificell.DefaultDUT), tf.APConn(), tf.APConnByID(1), routerType, clientIP[0].String(), apIface.ServerIP().String(), clientIface)
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
		if err := iwr.SetPowersaveMode(ctx, clientIface, powerSave); err != nil {
			s.Fatalf("Failed to set the powersave mode %t: %v", powerSave, err)
		}

		// Create apConfigTag which is used in the keyval
		var allTags []string
		var apConfigTag string
		psModeStr := "on"
		if !powerSave {
			psModeStr = "off"
		}
		psTag := fmt.Sprintf("PS%s", psModeStr)
		allTags = append(allTags, psTag)
		apConfigDesc := apIface.Config().PerfDesc()
		allTags = append(allTags, apConfigDesc)
		allTags = append(allTags, routerType.String())
		apConfigTag = strings.Join(allTags, "_")

		signalLevel, err := iwr.WifiInterfaceSignalLevel(ctx, clientIface)
		if err != nil {
			s.Error("Failed to get the singal level of the WiFi interface, err: ", err)
		}
		signalDesc := apConfigDesc + "_signal{perf}"
		// Write perf keyval
		perfKeyVal.WriteKeyVals(map[string]string{signalDesc: signalLevel})

		doRun := func(ctx context.Context) error {
			for _, testType := range perfTestTypes {
				s.Logf("Performing [[ %s ]]", testType)
				config, err := manager.Config(testType, 0)
				if err != nil {
					return errors.Wrapf(err, "failed to get the iperf/netperf configuration for test type %s", testType)
				}
				session, err := manager.Session(ctx, testType)
				if err != nil {
					return errors.Wrapf(err, "failed to get the iperf/netperf session for test type %s", testType)
				}
				channelWidth, _ := apIface.Config().ChannelWidthAndMode()
				expectedThrougput, err := perfmanager.ExpectedThroughputWiFi(routerType, testType, apIface.Config().Mode, channelWidth)
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
				failedResults := verifyResults(ctx, float64(finalResult.Throughput/iperf.Mbps), expectedThrougput.Must, expectedThrougput.Should, testType, powerSave, shouldTputRequired, apIface.Config().Channel, boardName)
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

	testcases := s.Param().([]networkWifiPerfTestCase)
	for i, tc := range testcases {
		subtest := func(ctx context.Context, s *testing.State) {
			testOnce(ctx, s, tc.apOpts, tc.secConfFac, tc.powerSave, tc.shouldTputRequired, board)
		}
		if !s.Run(ctx, fmt.Sprintf("Testcase #%d", i), subtest) {
			// Stop if any sub-test failed.
			return
		}
	}
	if len(lowThroughputTests) != 0 {
		s.Fatalf("Failed throughput performance too low for test type(s): %s", strings.Join(lowThroughputTests, ","))
	}
	s.Log("Tearing down")
}
