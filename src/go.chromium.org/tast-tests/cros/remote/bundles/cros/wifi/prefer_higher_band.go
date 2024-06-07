// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type preferHigherBandTestCase struct {
	lowerBandApOpts  []hostapd.Option
	higherBandApOpts []hostapd.Option
	// If unassigned, use default security config: open network.
	secConfFac security.ConfigFactory
}

func init() {
	testing.AddTest(&testing.Test{
		Func: PreferHigherBand,
		Desc: "Verifies that DUT can see two APs in the same network and prefer higher band one",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"arowa@google.com",                // Author
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Name: "2ghz_5ghz",
				Val: preferHigherBandTestCase{
					lowerBandApOpts:  []hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(1), hostapd.VHTChWidth(hostapd.VHTChWidth20Or40), hostapd.PMF(hostapd.PMFRequired)},
					higherBandApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(48), hostapd.VHTChWidth(hostapd.VHTChWidth20Or40), hostapd.PMF(hostapd.PMFRequired)},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
				},
			},
			{
				Name: "5ghz_6ghz",
				Val: preferHigherBandTestCase{
					lowerBandApOpts:  []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.Channel(48)},
					higherBandApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.Channel(21), hostapd.PMF(hostapd.PMFRequired), hostapd.OpClass(131)},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
				},
				// TODO(b/317288421): Promote test to stable by removing wificell_unstable attribute.
				ExtraAttr:         []string{"wificell_unstable"},
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax6E()),
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX_E),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
			{
				Name: "2ghz_6ghz",
				Val: preferHigherBandTestCase{
					lowerBandApOpts:  []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.Channel(1)},
					higherBandApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.Channel(21), hostapd.PMF(hostapd.PMFRequired), hostapd.OpClass(131)},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
				},
				// TODO(b/317288421): Promote test to stable by removing wificell_unstable attribute.
				ExtraAttr:         []string{"wificell_unstable"},
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax6E()),
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX_E),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
		},
	})
}

func PreferHigherBand(ctx context.Context, s *testing.State) {
	// This test verifies that the DUT will connect to the higher band
	// network when both lower and higher bands are available by using
	// the following steps:
	// 1- Initialize the regulatory domain to the US for 6 Ghz tests.
	// 2- Configure the AP with the lower band network.
	// 3- Configure the AP with a higher band network using the same SSID.
	// 4- Connect the DUT to the SSID.
	// 5- Verify the band of the connection is the higher band by checking
	//    the frequency.

	tf := s.FixtValue().(*wificell.TestFixture)
	tc := s.Param().(preferHigherBandTestCase)

	var cancel context.CancelFunc

	// Turn off background and foreground scans, so that all the WiFi scans are triggered by shill::Manager::RequestScan()
	ctx, restoreBgAndFg, err := tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	// Set WiFi request scan type to active on the DUT
	originalRequestScanType, err := tf.WifiClient().GetRequestScanTypeProperty(ctx)
	if err != nil {
		s.Error("Failed to get WiFi RequestScan type: ", err)
	}
	if originalRequestScanType != shillconst.WiFiRequestScanTypeActive {
		defer func(ctx context.Context) {
			if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, originalRequestScanType); err != nil {
				s.Errorf("Failed to reset WiFi RequestScan type to %s: %v", originalRequestScanType, err)
			}
			s.Log("Reset WiFi RequestScan type to ", originalRequestScanType)
		}(ctx)
		ctx, cancel = ctxutil.Shorten(ctx, 500*time.Millisecond)
		defer cancel()
		if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, shillconst.WiFiRequestScanTypeActive); err != nil {
			s.Fatal("Failed to set WiFi RequestScan type to active: ", err)
		}
		s.Log("Set WiFi RequestScan type to active")
	}

	ssid := hostapd.RandomSSID("TAST_TEST_")
	apConfigs := []hostapd.ApConfig{
		{ApOpts: append([]hostapd.Option{hostapd.SSID(ssid)}, tc.lowerBandApOpts...), SecConfFac: tc.secConfFac},
		{ApOpts: append([]hostapd.Option{hostapd.SSID(ssid)}, tc.higherBandApOpts...), SecConfFac: tc.secConfFac},
	}
	apIface, err := tf.ConfigureMultiAP(ctx, wificell.DefaultRouter, apConfigs)
	if err != nil {
		s.Fatal("Failed to configure ap, err: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, apIface); err != nil {
			s.Error("Failed to deconfig ap, err: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, apIface)
	defer cancel()
	s.Log("AP setup done")

	initialRegDomain, err := tf.InitializeRegdomainUS(ctx)
	if err != nil {
		s.Fatal("Failed to initialize the regulatory domain: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.ResetRegdomain(ctx, initialRegDomain); err != nil {
			s.Error("Failed to reset the regulatory domain: ", err)
		}
	}(ctx)
	ctx, cancel = ctxutil.Shorten(ctx, 500*time.Millisecond)
	defer cancel()

	lowerBand, freqLowerBand, err := apIface.Configs()[0].OperatingBandAndFreq()
	if err != nil {
		s.Errorf("Failed to get operating band on the interface %s: %v", apIface.Interfaces()[0], err)
	}
	higherBand, freqHigherBand, err := apIface.Configs()[1].OperatingBandAndFreq()
	if err != nil {
		s.Errorf("Failed to get operating band on the interface %s: %v", apIface.Interfaces()[1], err)
	}
	s.Logf("AP setup done. Expecting the DUT to see the SSID on both %s and %s channels", lowerBand.String(), higherBand.String())

	// Check SSID on both lower and higher band channels.
	req := &wifi.ExpectWifiFrequenciesRequest{
		Ssid:        []byte(ssid),
		Frequencies: []uint32{uint32(freqLowerBand), uint32(freqHigherBand)},
	}
	if _, err := tf.WifiClient().ExpectWifiFrequencies(ctx, req); err != nil {
		s.Error("Failed to expect a service with two WiFi frequencies: ", err)
	}
	s.Log("Verified. Asserting the connection")
	secConf, err := tc.secConfFac.Gen()
	if err != nil {
		s.Error("Failed to generate security config: ", err)
	}
	if _, err := tf.ConnectWifi(ctx, ssid, dutcfg.ConnHidden(false), dutcfg.ConnSecurity(secConf)); err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	freqSignal, err := wifiSignal(ctx, tf, s.DUT(), ssid)
	if err != nil {
		s.Fatal("Failed to get wifi signal: ", err)
	}
	s.Log("WiFi signal: ", listSignal(freqSignal))

	service, err := tf.WifiClient().QueryService(ctx)
	if err != nil {
		s.Fatal("Failed to get the active WiFi service from DUT: ", err)
	}
	if service.Wifi.Frequency != uint32(freqHigherBand) {
		s.Fatalf("Got frequency %d; want %d", service.Wifi.Frequency, freqHigherBand)
	}
	s.Logf("Verified that the DUT is using %s band... Tearing down", higherBand.String())
}

// wifiSignal returns a frequency-signal mapping of the given SSID.
func wifiSignal(ctx context.Context, tf *wificell.TestFixture, dut *dut.DUT, ssid string) (map[int]float64, error) {
	iface, err := tf.ClientInterface(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the DUT's WiFi interface")
	}

	scanResult, err := iw.NewRemoteRunner(dut.Conn()).ScanDump(ctx, iface)
	if err != nil {
		return nil, errors.Wrap(err, "failed to perform iw scan dump")
	}
	ret := map[int]float64{}
	for _, data := range scanResult {
		if data.SSID == ssid {
			ret[data.Frequency] = data.Signal
		}

	}
	return ret, nil
}

// listSignal returns a string of frequency:signal strength pairs.
func listSignal(freqSignal map[int]float64) string {
	freqs := make([]int, 0, len(freqSignal))
	for f := range freqSignal {
		freqs = append(freqs, f)
	}
	sort.Ints(freqs)
	ret := make([]string, 0, len(freqs))
	for _, f := range freqs {
		ret = append(ret, fmt.Sprintf("Freq: %dGHz  Signal: %.2fdBm", f, freqSignal[f]))
	}
	return strings.Join(ret, " / ")
}
