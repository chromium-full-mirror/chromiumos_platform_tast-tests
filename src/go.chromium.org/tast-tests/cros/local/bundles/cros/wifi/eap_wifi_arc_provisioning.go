// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/wifi/certutil"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/wifi/eap"
	"go.chromium.org/tast-tests/cros/local/hostapd"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

var failReg = regexp.MustCompile(`.*Save failed.*`)

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
		Func:         EAPWifiARCProvisioning,
		Desc:         "EAP WiFi network ARC provisioning tests",
		Contacts:     []string{"cros-networking@google.com", "chuweih@google.com"},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFiWithArcBooted",
		LacrosStatus: testing.LacrosVariantUnneeded,
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

// EAPWifiARCProvisioning tests that EAP WiFi could be provisioned from
// ARC and connect successfully to networks with different EAP credentials
// and disconnect from the network when credentials is removed from ARC.
func EAPWifiARCProvisioning(ctx context.Context, s *testing.State) {
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
		if err := forgetNetwork(cleanupCtx, a, apSSID); err != nil {
			s.Fatal("Failed to forget network: ", err)
		}
	}(cleanupCtx)

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
	if err := forgetNetwork(cleanupCtx, a, apSSID); err != nil {
		s.Fatal("Failed to forget network: ", err)
	}

	// Wait for the station to dissociate with the access point.
	if err := hostapd.WaitForSTADissociated(ctx, h, clientIface, hostapd.STAAssociationTimeout); err != nil {
		s.Error("Failed to check station dissociation: ", err)
	}
}

func forgetNetwork(ctx context.Context, a *arc.ARC, ssid string) error {
	out, err := a.Command(ctx, "cmd", "wifi", "list-networks").Output(testexec.DumpLogOnError)
	if err != nil {
		return err
	}

	for _, network := range strings.Split(string(out), "\n") {
		// Get network information by shell cmd, if len(network) is 0, means len(netInfos) is also 0,
		// information line is empty, skip.
		if len(network) == 0 {
			continue
		}
		netInfos := strings.Split(network, " ")

		// The output is in the format of:
		// networkid=<netID> SSID=<SSID> BSSID=<BSSID> guid=<guid> security=<security>
		// currentSSID is the SSID of current network. If currentSSID matches newly added
		// network, get networkId of this network and forget network through shell cmd.
		currentSSID := strings.Split(string(netInfos[1]), "\"")[1]

		if currentSSID == ssid {
			netID := strings.Split(netInfos[0], "=")[1]
			return a.Command(ctx, "cmd", "wifi", "forget-network", netID).Run(testexec.DumpLogOnError)
		}
	}
	return nil
}
