// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type secConf struct {
	config           security.ConfigFactory
	expectedSecurity string
}

// EAP cert for EAP tests.
var fwcCert1 = certificate.TestCert1()

func init() {
	testing.AddTest(&testing.Test{
		Func: FgsecWpaChange,
		Desc: "Verifies connection to AP that has changed security",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		ServiceDeps:     []string{wificell.ShillServiceName},
		SoftwareDeps:    []string{"wpa3_sae"},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiGenSupportLegacy, tdreq.WiFiGenSupportPMF, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Name: "wpa_1_2_3",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP)),
						expectedSecurity: shillconst.SecurityWPA,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPAWPA2,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Personal, tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "wpa_2_1_3",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPA2,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP)),
						expectedSecurity: shillconst.SecurityWPAWPA2,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Personal, tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "wpa_1_3",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP)),
						expectedSecurity: shillconst.SecurityWPA,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "wpa_1_23",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP)),
						expectedSecurity: shillconst.SecurityWPA,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Personal, tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "wpa_2_23_1",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPA2,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPA2WPA3,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Personal, tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "wpa_3_12",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPA3,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixed), wpa.Ciphers(wpa.CipherTKIP), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Personal, tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "wpa_3_2_1",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPA3,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPA2WPA3,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Personal, tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "wpa_12_3",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixed), wpa.Ciphers(wpa.CipherTKIP), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPAWPA2,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Personal, tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "wpa_23_1",
				Val: []secConf{
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
						expectedSecurity: shillconst.SecurityWPA2WPA3,
					},
					{
						config:           wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP)),
						expectedSecurity: shillconst.SecurityWPAAll,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Personal, tdreq.WiFiSecSupportWPA3Personal},
			},
			{
				Name: "8021xwpa_1_2_3",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA)),
						expectedSecurity: shillconst.SecurityWPAEnterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA2)),
						expectedSecurity: shillconst.SecurityWPAWPA2Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA3)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise, tdreq.WiFiSecSupportWPA3Enterprise},
			},
			{
				Name: "8021xwpa_2_1_3",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA2)),
						expectedSecurity: shillconst.SecurityWPA2Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA)),
						expectedSecurity: shillconst.SecurityWPAWPA2Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA3)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise, tdreq.WiFiSecSupportWPA3Enterprise},
			},
			{
				Name: "8021xwpa_1_3",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA)),
						expectedSecurity: shillconst.SecurityWPAEnterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA3)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA3Enterprise},
			},
			{
				Name: "8021xwpa_1_23",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA)),
						expectedSecurity: shillconst.SecurityWPAEnterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModeMixedWPA3)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise, tdreq.WiFiSecSupportWPA3Enterprise},
			},
			{
				Name: "8021xwpa_2_23_1",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA2)),
						expectedSecurity: shillconst.SecurityWPA2Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModeMixedWPA3)),
						expectedSecurity: shillconst.SecurityWPA2WPA3Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise, tdreq.WiFiSecSupportWPA3Enterprise},
			},
			{
				Name: "8021xwpa_3_12",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA3)),
						expectedSecurity: shillconst.SecurityWPA3Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModeMixed)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise, tdreq.WiFiSecSupportWPA3Enterprise},
			},
			{
				Name: "8021xwpa_3_2_1",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA3)),
						expectedSecurity: shillconst.SecurityWPA3Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA2)),
						expectedSecurity: shillconst.SecurityWPA2WPA3Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise, tdreq.WiFiSecSupportWPA3Enterprise},
			},
			{
				Name: "8021xwpa_12_3",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModeMixed)),
						expectedSecurity: shillconst.SecurityWPAWPA2Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA3)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise, tdreq.WiFiSecSupportWPA3Enterprise},
			},
			{
				Name: "8021xwpa_23_1",
				Val: []secConf{
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModeMixedWPA3)),
						expectedSecurity: shillconst.SecurityWPA2WPA3Enterprise,
					},
					{
						config: wpaeap.NewConfigFactory(
							fwcCert1.CACred.Cert, fwcCert1.ServerCred,
							wpaeap.ClientCACert(fwcCert1.CACred.Cert),
							wpaeap.ClientCred(fwcCert1.ClientCred),
							wpaeap.Mode(wpa.ModePureWPA)),
						expectedSecurity: shillconst.SecurityWPAAllEnterprise,
					},
				},
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise, tdreq.WiFiSecSupportWPA3Enterprise},
			},
		},
	})
}

// FgsecWpaChange tests connectivity to an AP with changed WPA settings security mode.
// Each subtest is a loop over list of security configuration specified above. For each element:
//  1. Configure AP according to security configuration (keeping SSID so all the time it is regarded
//     as the same network by the shill).
//  2. Test ability to connect.
//  3. Check that:
//     - the service path has not changed,
//     - service has correct Security property.//
//  4. Deconfigure AP.
func FgsecWpaChange(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	ssid := ap.RandomSSID("TAST_FGSEC_")
	apOpts := []ap.Option{ap.SSID(ssid), ap.Mode(ap.Mode80211g), ap.Channel(1), ap.PMF(ap.PMFOptional)}
	servicePath := ""

	connectAP := func(ctx context.Context, sec *secConf) (retErr error) {
		ap, err := tf.ConfigureAP(ctx, apOpts, sec.config)
		if err != nil {
			return errors.Wrap(err, "failed to configure the AP1")
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, ap); err != nil {
				retErr = errors.Wrap(err, "failed to deconfig the AP")
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()

		connResp, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap)
		if err != nil {
			return errors.Wrap(err, "failed to connect to WiFi")
		}
		defer func(ctx context.Context) {
			if err := tf.DisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
				retErr = errors.Wrap(err, "failed to disconnect WiFi")
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()

		if servicePath == "" {
			servicePath = connResp.ServicePath
		} else if servicePath != connResp.ServicePath {
			return errors.Errorf("Service path has changed: got %s, want %s", connResp.ServicePath, servicePath)
		}

		srvcResp, err := tf.WifiClient().QueryService(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get service properties")
		}
		s.Log("Connected with Security: ", srvcResp.Wifi.Security)
		if srvcResp.Wifi.Security != sec.expectedSecurity {
			return errors.Errorf("Wrong service security: got %s, want %s",
				srvcResp.Wifi.Security, sec.expectedSecurity)
		}
		return nil
	}

	securityConfigs := s.Param().([]secConf)

	// Get the name of the DUT WiFi interface to flush BSS from WPA
	// supplicant after each connection to make sure it uses
	// currently visible BSSes for reconnection.
	clientIface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Unable to get DUT interface name: ", err)
	}
	for _, c := range securityConfigs {
		if err = connectAP(ctx, &c); err != nil {
			s.Fatal("Failure during AP connection: ", err)
		}
		s.Log("Flushing BSS cache")
		if err := tf.WifiClient().FlushBSS(ctx, clientIface, 0); err != nil {
			s.Fatal("Failed to flush BSS list: ", err)
		}
	}
}
