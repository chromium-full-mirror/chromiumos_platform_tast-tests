// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"encoding/hex"
	"reflect"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcwifi"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// wifiInShillByCmdTestCase describes the parameters of a single test case.
type wifiInShillByCmdTestCase struct {
	// args consist of a cmd that will be run to create network.
	args []string
	// ssid of the network to be added.
	ssid string
	// props are expected properties of created network.
	props map[string]interface{}
}

func init() {
	// Set of tests designed to reproduce command line interaction to create network
	// from ARC. It is expected for shill to successfully create WiFi network with
	// given arguments.
	testing.AddTest(&testing.Test{
		Func:     ARCCreateWifiInShillByCmd,
		Desc:     "Test if wifi network can be correctly created in shill",
		Contacts: []string{"cros-networking@google.com", "chuweih@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Fixture:      "shillSimulatedWiFiWithArcBooted",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi", "chrome", "arc"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{
			{
				Name: "auto_reconnect_disabled",
				Val: wifiInShillByCmdTestCase{
					args: []string{"-d"},
					ssid: "GoogleGuest",
					props: map[string]interface{}{
						shillconst.ServicePropertyAutoConnect: false,
					},
				},
			},
			{
				Name: "dns_and_search_domains",
				Val: wifiInShillByCmdTestCase{
					args: []string{"--dns", "8.8.8.8,8.8.4.4", "--search-domains", "test1.com,test2.com"},
					ssid: "GoogleGuest",
					props: map[string]interface{}{
						shillconst.ServicePropertyAutoConnect:    true,
						shillconst.IPConfigPropertyNameServers:   []string{"8.8.8.8", "8.8.4.4"},
						shillconst.IPConfigPropertySearchDomains: []string{"test1.com", "test2.com"},
					},
				},
			},
			{
				Name: "auto_reconnect_disabled_with_pac_url",
				Val: wifiInShillByCmdTestCase{
					args: []string{"-d", "--pac", "http://test/test"},
					ssid: "GoogleGuest",
					props: map[string]interface{}{
						shillconst.ServicePropertyAutoConnect: false,
						shillconst.ServicePropertyProxyConfig: `{"mode":"pac_script","pac_mandatory":false,"pac_url":"http://test/test"}`,
					},
				},
			},
			{
				Name: "manual_proxy",
				Val: wifiInShillByCmdTestCase{
					args: []string{"-d", "--proxy-host", "hostName", "--proxy-port", "2222"},
					ssid: "GoogleGuest",
					props: map[string]interface{}{
						shillconst.ServicePropertyAutoConnect: false,
						shillconst.ServicePropertyProxyConfig: `{"mode":"direct"}`,
					},
				},
			},
		},
	})
}

// ARCCreateWifiInShillByCmd expects a wifi with given configs are created correctly through shell cmd.
func ARCCreateWifiInShillByCmd(ctx context.Context, s *testing.State) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}

	a := s.FixtValue().(*hwsim.ShillSimulatedWiFi).ARC
	tc := s.Param().(wifiInShillByCmdTestCase)
	// Reserve a little time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
	defer cancel()

	// Append customized args of every test case to base cmd that is shared to form a cmd.
	baseCmd := []string{"wifi", "add-network", "GoogleGuest", "open"}
	out, err := a.Command(ctx, "cmd", append(baseCmd, tc.args...)...).Output(testexec.DumpLogOnError)
	// If the output contains failure message, it indicates failure to add network.
	if err != nil {
		s.Fatal("Failed to add network: ", err)
	}
	defer func() {
		if err := arcwifi.ForgetNetwork(cleanupCtx, a, tc.ssid); err != nil {
			s.Fatal("Failed to forget network: ", err)
		}
	}()

	failReg := regexp.MustCompile(`.*Save failed.*`)
	if failReg.Match(out) {
		s.Fatal("Failed to add network, output is: ", string(out))
	}

	services, _, err := m.ServicesByTechnology(ctx, shill.TechnologyWifi)
	if err != nil {
		s.Fatal("Failed to get wifi services: ", err)
	}

	for _, service := range services {
		p, err := service.GetProperties(ctx)
		if err != nil {
			s.Error("Failed to get wifi service properties: ", err)
		}

		// Get the SSID of current wifi service.
		hexSSID, err := p.GetString(shillconst.ServicePropertyWiFiHexSSID)
		decodeSSID, err := hex.DecodeString(hexSSID)
		if err != nil {
			s.Error("Failed to decode SSID: ", err)
		}

		curSSID := string(decodeSSID)

		// If SSID of current service does not match SSID in the test case, means that it is
		// not newly added wifi network, continue.
		if curSSID != tc.ssid {
			continue
		}

		// Check if properties of newly added network match expectations.
		for prop := range tc.props {
			switch prop {
			case shillconst.ServicePropertyAutoConnect:
				version, err := arc.SDKVersion()
				if err != nil {
					s.Fatal("Failed to get SDK version: ", err)
				}
				// Setting of AutoConnect is only supported from ARC R
				if version >= arc.SDKR {
					val, err := p.Get(prop)
					if err != nil {
						s.Fatal("Failed to get proxy config property from service: ", err)
					}

					expected := tc.props[prop]
					if val != expected {
						s.Errorf("%v is %v, want: %v", prop, val, expected)
					}
				}
			case shillconst.ServicePropertyProxyConfig:
				val, err := p.Get(prop)
				if err != nil {
					s.Fatal("Failed to get proxy config property from service: ", err)
				}

				expected := tc.props[prop]
				if val != expected {
					s.Errorf("%v is %v, want: %v", prop, val, expected)
				}
			case shillconst.IPConfigPropertyNameServers:
				staticIPConfig, err := p.Get(shillconst.ServicePropertyStaticIPConfig)
				if err != nil {
					s.Fatal("Failed to get static IP config property from service: ", err)
				}

				nameServers := staticIPConfig.(map[string]interface{})[shillconst.IPConfigPropertyNameServers].([]string)
				expected := tc.props[prop].([]string)
				// The reason we are checking one by one if name server is contained in list rather than equal
				// is because the list would be filled with default value 0.0.0.0 if the number of given name
				// server is less than 4.
				for _, ns := range expected {
					if !containsElement(nameServers, ns) {
						s.Error(ns + " is expected to be included but it is not")
					}
				}
			case shillconst.IPConfigPropertySearchDomains:
				staticIPConfig, err := p.Get(shillconst.ServicePropertyStaticIPConfig)
				if err != nil {
					s.Fatal("Failed to get static IP config property from service: ", err)
				}

				searchDomains := staticIPConfig.(map[string]interface{})[shillconst.IPConfigPropertySearchDomains]
				expected := tc.props[prop].([]string)
				if !reflect.DeepEqual(expected, searchDomains.([]string)) {
					s.Errorf("searchDomains is: %+v, want: %+v", searchDomains.([]string), expected)
				}
			}
		}
	}
}

func containsElement(list []string, a string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
