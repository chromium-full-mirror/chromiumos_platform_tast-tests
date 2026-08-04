// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wep"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type roamTestcase struct {
	apOpts1          []hostapd.Option
	apOpts2          []hostapd.Option
	secConfFac       security.ConfigFactory
	enableBSSFlush   bool
	performSuspend   bool
	expectedRoamTime time.Duration
}

// EAP certs/keys for EAP tests.
var (
	roamCert = certificate.TestCert1()
)

func (tc roamTestcase) setEnableBSSFlush(enable bool) roamTestcase {
	tc.enableBSSFlush = enable
	return tc
}

func (tc roamTestcase) setRoamTime(d time.Duration) roamTestcase {
	tc.expectedRoamTime = d
	return tc
}

func (tc roamTestcase) setPerformSuspend(perform bool) roamTestcase {
	tc.performSuspend = perform
	return tc
}

var (
	roamTestcaseWithTwoOpenAP = roamTestcase{
		apOpts1:    []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
		apOpts2:    []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
		secConfFac: nil,
	}
	roamTestcaseWithTwoWPAAP = roamTestcase{
		apOpts1:    []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
		apOpts2:    []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
		secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
	}
	roamTestcaseWithTwoWEPAP = roamTestcase{
		apOpts1:    []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
		apOpts2:    []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
		secConfFac: wep.NewConfigFactory([]string{"abcde", "fedcba9876", "ab\xe4\xb8\x89", "\xe4\xb8\x89\xc2\xa2"}, wep.DefaultKey(0), wep.AuthAlgs(wep.AuthAlgoOpen)),
	}
	roamTestcaseWithTwo8021xWPAAP = roamTestcase{
		apOpts1:    []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
		apOpts2:    []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
		secConfFac: wpaeap.NewConfigFactory(roamCert.CACred.Cert, roamCert.ServerCred, wpaeap.ClientCACert(roamCert.CACred.Cert), wpaeap.ClientCred(roamCert.ClientCred)),
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RoamAPGone,
		Desc: "Tests roaming to an AP that disappears while the client is awake",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"rmekonnen@google.com",
			"edgar.chang@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		LifeCycleStage: testing.LifeCycleInDevelopment,
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:            []string{"group:wificell", "wificell_func", "group:release-health", "release-health_wifi"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				// Verifies that DUT can roam between two APs in full view of it.
				Name:              "open",
				Val:               roamTestcaseWithTwoOpenAP.setRoamTime(5 * time.Second),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				// TODO(b/542029943): Re-enable once the test is stable.
				// ExtraAttr:         []string{"wificell_unstable"},
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WPA APs in full view of it.
				Name:              "wpa",
				Val:               roamTestcaseWithTwoWPAAP.setRoamTime(5 * time.Second),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				// TODO(b/542029943): Re-enable once the test is stable.
				// ExtraAttr:         []string{"wificell_unstable"},
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WEP APs in full view of it.
				Name:              "wep",
				Val:               roamTestcaseWithTwoWEPAP.setRoamTime(5 * time.Second),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiWEP(), hwdep.WifiNotMarvell()),
				ExtraRequirements: []string{tdreq.WiFiSecSupportWEP},
				// TODO(b/542029943): Re-enable once the test is stable.
				// ExtraAttr:         []string{"wificell_unstable"},
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel_WEP"}`,
			}, {
				// Verifies that DUT can roam between two WPA-EAP APs in full view of it.
				Name:              "8021xwpa",
				Val:               roamTestcaseWithTwo8021xWPAAP.setRoamTime(5 * time.Second),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				// TODO(b/542029943): Re-enable once the test is stable.
				// ExtraAttr:         []string{"wificell_unstable"},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise},
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two APs with minimal idle time after bss flush.
				Name:              "flushbss",
				Val:               roamTestcaseWithTwoOpenAP.setRoamTime(10 * time.Second).setEnableBSSFlush(true),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				// TODO(b/542029943): Re-enable once the test is stable.
				// ExtraAttr:         []string{"wificell_unstable"},
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two APs in full view of it.
				Name:              "marvell",
				Val:               roamTestcaseWithTwoOpenAP.setRoamTime(12 * time.Second),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WPA APs in full view of it.
				Name:              "wpa_marvell",
				Val:               roamTestcaseWithTwoWPAAP.setRoamTime(12 * time.Second),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WEP APs in full view of it.
				Name:              "wep_marvell",
				Val:               roamTestcaseWithTwoWEPAP.setRoamTime(12 * time.Second),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiWEP(), hwdep.WifiMarvell()),
				ExtraRequirements: []string{tdreq.WiFiSecSupportWEP},
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel_WEP"}`,
			}, {
				// Verifies that DUT can roam between two WPA-EAP APs in full view of it.
				Name:              "8021xwpa_marvell",
				Val:               roamTestcaseWithTwo8021xWPAAP.setRoamTime(12 * time.Second),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise},
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two APs with minimal idle time after bss flush.
				Name:              "flushbss_marvell",
				Val:               roamTestcaseWithTwoOpenAP.setRoamTime(12 * time.Second).setEnableBSSFlush(true),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WPA APs in full view of it after suspend/resume.
				Name:              "wpa_suspend",
				Val:               roamTestcaseWithTwoWPAAP.setRoamTime(5 * time.Second).setPerformSuspend(true),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				// TODO(b/362115332): Remove this attribute after the test is stable.
				// ExtraAttr:       []string{"wificell_unstable"},
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WPA APs in full view of it after suspend/resume.
				Name:              "wpa_suspend_marvell",
				Val:               roamTestcaseWithTwoWPAAP.setRoamTime(12 * time.Second).setPerformSuspend(true),
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				// TODO(b/362115332): Remove this attribute after the test is stable.
				// ExtraAttr:       []string{"wificell_unstable"},
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
		},
	})
}

func RoamAPGone(ctx context.Context, s *testing.State) {
	/*
		This test checks a DUT's ability to naturally roam after AP lost
		by using the following steps:
		1 - Configure AP1.
		2 - Associate DUT to AP1.
		3 - Configure AP2 with the same SSID as AP1.
		4 - Either flush all BSSes to force the DUT to rescan after AP1
		    disappears or trigger a new scan now, so that the DUT can
		    autoconnect after AP1 disappears.
		5 - Suspend and resume the device on demand.
		6 - Either deconfigure AP1 when DUT is asleep or awake.
		7 - Verify the DUT roams to AP2.
		8 - Deconfigure the DUT.
		9 - Deconfigure AP2.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)

	// Configure the initial AP.
	param := s.Param().(roamTestcase)
	ap1, err := tf.ConfigureAP(ctx, param.apOpts1, param.secConfFac)
	if err != nil {
		s.Fatal("Failed to configure AP, err: ", err)
	}
	ssid := ap1.Config().SSID
	defer func(ctx context.Context) {
		if ap1 == nil {
			// ap1 is already deconfigured.
			return
		}
		if err := tf.DeconfigAP(ctx, ap1); err != nil {
			s.Error("Failed to deconfig AP1, err: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap1)
	defer cancel()
	s.Log("AP1 setup done")

	// Schedule defer for AP2 before connection. Otherwise, we
	// will teardown AP2 before disconnect, and the DUT might
	// get disconnected due to inactivity and causing flaky
	// Disconnect failure.
	var ap2 *wificell.APIface
	defer func(ctx context.Context) {
		if ap2 == nil {
			return
		}
		if err := tf.DeconfigAP(ctx, ap2); err != nil {
			s.Error("Failed to deconfig AP2, err: ", err)
		}
	}(ctx)
	// We don't have ap2 yet, borrow the reserve of ap1.
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap1)
	defer cancel()

	// Connect to the initial AP.
	var servicePath string
	if resp, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap1); err != nil {
		s.Fatal("Failed to connect to WiFi, err: ", err)
	} else {
		servicePath = resp.ServicePath
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
			s.Error("Failed to disconnect WiFi, err: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	s.Log("Connected to AP1")

	if err := tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, ap1); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}

	// Generate the BSSID for second AP.
	mac, err := hostapd.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSSID: ", err)
	}
	ap2BSSID := mac.String()

	// Configure the second AP.
	var ops []hostapd.Option
	ops = append(ops, param.apOpts2...)
	// Override SSID and BSSID as we need the same SSID as the first AP
	// and the BSSID that we're waiting.
	ops = append(ops, hostapd.SSID(ssid), hostapd.BSSID(ap2BSSID))
	ap2, err = tf.ConfigureAP(ctx, ops, param.secConfFac)
	if err != nil {
		s.Fatal("Failed to configure AP, err: ", err)
	}
	// defer deconfig already scheduled above.
	s.Log("AP2 setup done")

	clientIface, err := tf.DUTClientInterface(ctx, wificell.DefaultDUT)
	if err != nil {
		s.Fatal("Unable to get DUT interface name: ", err)
	}

	if param.enableBSSFlush {
		// Flush all BSSes from cache to ensure that we are forced to rescan
		// after disconnect.
		s.Log("Flushing BSS cache")
		if err := tf.DUTWifiClient(wificell.DefaultDUT).FlushBSS(ctx, clientIface, 0); err != nil {
			s.Fatal("Failed to flush BSS list: ", err)
		}
	} else {
		// Discover AP2 before disconnecting from AP1.
		s.Logf("Waiting for AP2 discovery: %s", ap2BSSID)
		if err := tf.DUTWifiClient(wificell.DefaultDUT).DiscoverBSSID(ctx, ap2BSSID, clientIface, []byte(ssid)); err != nil {
			s.Fatal("Unable to discover AP2 BSSID: ", err)
		}
	}

	suspendFor := 15 * time.Second

	eg, suspendCtx := errgroup.WithContext(ctx)
	if param.performSuspend {
		eg.Go(func() error { return tf.DUTWifiClient(wificell.DefaultDUT).Suspend(suspendCtx, suspendFor) })

		// Ensure deconfiguration of ap occurs after DUT suspension.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if s.DUT().Connected(ctx) {
				return errors.New("DUT has not yet successfully suspended")
			}
			return nil
		}, &testing.PollOptions{Timeout: suspendFor}); err != nil {
			s.Fatal("Failed to suspend the DUT before deconfigure AP: ", err)
		}
	}

	// Deconfigure the initial AP.
	if err := tf.DeconfigAP(ctx, ap1); err != nil {
		s.Error("Failed to deconfig AP, err: ", err)
	}
	ap1 = nil
	s.Log("Deconfigured AP1")

	// Wait for device to resume if suspended.
	if err := eg.Wait(); err != nil {
		s.Fatal("Failed to suspend and resume dut: ", err)
	}

	props := []*wificell.ShillProperty{
		{
			Property:       shillconst.ServicePropertyWiFiBSSID,
			ExpectedValues: []interface{}{ap2BSSID},
			// May roam to the other Wi-Fi before checking connection status through Shill manager,
			// which will prevent waiting for the property change signal, therefore,
			// ExpectShillPropertyRequest_ON_CHANGE is not a preference.
			Method: wifi.ExpectShillPropertyRequest_CHECK_WAIT,
		},
	}

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	waitForProps, err := tf.WifiClient().ExpectShillProperty(waitCtx, servicePath, props, nil)
	if err != nil {
		s.Fatal("DUT: failed to create a property watcher, err: ", err)
	}

	startTime := time.Now()
	if _, err := waitForProps(); err != nil {
		s.Fatal("DUT: failed to wait for the properties, err: ", err)
	}
	roamTime := time.Since(startTime)

	s.Logf("DUT: roamed in %s", roamTime)
	if roamTime > param.expectedRoamTime {
		s.Fatalf("DUT: took to long to roam: %v, expected to roam in %s", roamTime, param.expectedRoamTime)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, ap2)
	}, &testing.PollOptions{
		Timeout:  20 * time.Second,
		Interval: time.Second,
	}); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}
}
