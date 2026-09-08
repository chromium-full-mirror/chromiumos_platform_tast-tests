// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcwifi"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/certutil"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/eap"
	"go.chromium.org/tast-tests/cros/local/hostapd"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	certManager "go.chromium.org/tast-tests/cros/local/networkui/certificate"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

var testCerts = certificate.TestCert3()

// Domain suffix and alternative subject match are defined along with TestCert3
// which is borrowed from Autotest (client/common_lib/cros/site_eap_certs.py).
var domainSuffix = "example.com"
var altSubjectMatch = "DNS:mail.example.com;EMAIL:example@domain.com"

const (
	// Delay to wait for a network to be discovered.
	scanAndWaitTimeout = time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCEAPWifiProvisioning,
		Desc:         "EAP WiFi network ARC provisioning tests",
		Contacts:     []string{"cros-networking@google.com", "chuweih@google.com"},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: arc.ArcAppHwDeps,
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Fixture:      "shillSimulatedWiFiWithArcBootedWithoutUIAutomator",
		Timeout:      7 * time.Minute,
		Requirements: []string{tdreq.WiFiGenSupportWiFi},
		Params: []testing.Param{
			{
				Name: "eap_wifi_ttls",
				Val:  eap.AuthTTLS,
			},
			{
				Name: "eap_wifi_tls",
				Val:  eap.AuthTLS,
			},
		},
	})
}

// ARCEAPWifiProvisioning tests that EAP WiFi could be provisioned from
// ARC and connect successfully to networks with different EAP credentials
// and disconnect from the network when credentials is removed from ARC.
func ARCEAPWifiProvisioning(ctx context.Context, s *testing.State) {
	// Reserve a little time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
	defer cancel()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}

	// Obtain the simulated interfaces from the fixture environment.
	ifaces := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	if len(ifaces.AP) < 1 {
		s.Fatal("Test requires at least one simulated interface")
	}
	if len(ifaces.Client) < 1 {
		s.Fatal("Test requires at least one simulated client interface")
	}

	// Get ARC handle to provision credentials.
	a := s.FixtValue().(*hwsim.ShillSimulatedWiFi).ARC
	cr := s.FixtValue().(*hwsim.ShillSimulatedWiFi).Chrome

	apIface, clientIface := ifaces.AP[0], ifaces.Client[0]

	// Since SSID contains '.' is invalid in hostapd configuration,
	// only take last part of test name as SSID.
	apSSID := strings.Split(s.TestName(), ".")[2]
	apAuth := s.Param().(eap.Auth)

	ap := eap.AccessPoint{
		SSID: apSSID,
		Auth: apAuth,
	}

	server := ap.ToServer(apIface, s.OutDir(), testCerts)
	if err := server.Start(ctx); err != nil {
		s.Fatal("Failed to start access point: ", err)
	}
	defer server.Stop()
	// Create a monitor to collect access point events.
	h := hostapd.NewMonitor()
	if err := h.Start(ctx, server); err != nil {
		s.Error("Failed to start hostapd monitor: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Stop(ctx); err != nil {
			s.Error("Failed to stop hostapd monitor: ", err)
		}
	}(cleanupCtx)

	caCert := base64.StdEncoding.EncodeToString([]byte(testCerts.CACred.Cert))

	cmd := []string{"wifi", "add-network", apSSID, "eap", "--username", eap.TestUser, "--ca-cert", caCert, "--domain-suffix", domainSuffix, "--alt-subject", altSubjectMatch}
	switch apAuth {
	case eap.AuthTTLS:
		cmd = append(cmd, "--eap-method", "TTLS", "--phase2", "MSCHAPV2", "--password", eap.TestPassword)
	case eap.AuthTLS:
		pkcs12Cert, err := certutil.PreparePKCS12Cert(ctx, testCerts)
		if err != nil {
			s.Fatal("Failed to create PKCS#12 certificate: ", err)
		}
		clientCert := base64.StdEncoding.EncodeToString([]byte(pkcs12Cert))
		cmd = append(cmd, "--eap-method", "TLS", "--client-cert", clientCert)
	}

	// Provision WiFi credentials from ARC.
	failReg := regexp.MustCompile(`.*Save failed.*`)
	if out, err := a.Command(ctx, "cmd", cmd...).Output(testexec.DumpLogOnError); err != nil {
		// If the output contains failure message, it indicates failure to add network.
		s.Fatal("Failed to add EAP WiFi config from ARC: ", err)
	} else if failReg.Match(out) {
		s.Fatal("Failed to add EAP WiFi, output is: ", string(out))
	}

	removed := false
	defer func(ctx context.Context) {
		if removed {
			return
		}
		if err := arcwifi.ForgetNetwork(cleanupCtx, a, apSSID); err != nil {
			s.Fatal("Failed to forget network: ", err)
		}
	}(cleanupCtx)

	// Forgetting network does not remove the certs tied to the network.
	// Manually clean up the certs to avoid leaking them to the next tests.
	if apAuth == eap.AuthTLS {
		defer func(ctx context.Context) {
			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Error("Failed to create Test API connection: ", err)
				return
			}
			if err := certManager.DeleteCert(
				tconn,
				cr,
				certManager.NewCertData(testCerts, certManager.TypeClient),
			)(ctx); err != nil {
				s.Error("Failed to delete certificate: ", err)
			}
		}(cleanupCtx)
	}

	wifi, err := shill.NewWifiManager(ctx, m)
	if err != nil {
		s.Fatal("Failed to obtain Wi-Fi manager: ", err)
	}

	// Trigger a scan.
	if _, err := wifi.ScanAndWaitForService(ctx, apSSID, scanAndWaitTimeout); err != nil {
		s.Fatal("Failed to request an active scan: ", err)
	}

	// Wait for the station to associate with the access point.
	if err := hostapd.WaitForSTAAssociated(ctx, h, clientIface, hostapd.STAAssociationTimeout); err != nil {
		s.Fatal("Failed to check station association: ", err)
	}

	// Remove EAP WiFi config from ARC.
	removed = true
	if err := arcwifi.ForgetNetwork(cleanupCtx, a, apSSID); err != nil {
		s.Fatal("Failed to forget network: ", err)
	}

	// Wait for the station to dissociate with the access point.
	if err := hostapd.WaitForSTADissociated(ctx, h, clientIface, hostapd.STAAssociationTimeout); err != nil {
		s.Error("Failed to check station dissociation: ", err)
	}
}
