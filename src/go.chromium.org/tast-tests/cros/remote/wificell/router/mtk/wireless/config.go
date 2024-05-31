// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wireless

// DefaultGatewayIP is the default IP address of the gateway
const DefaultGatewayIP = "192.168.1.1"

// Constants for phy, wifi-device, wifi-iface, wifi-mld on MTK router
const (
	// WiFi bands
	WiFiBand2G = "2.4G"
	WiFiBand5G = "5G"
	WiFiBand6G = "6G"

	// WiFi phy
	WiFiPhy2G = "phy0"
	WiFiPhy5G = "phy1"
	WiFiPhy6G = "phy2"

	// WiFiDevice struct
	WiFiDevice   = "wifi-device"
	WiFiDevice2G = "MT7990_1_1"
	WiFiDevice5G = "MT7990_1_2"
	WiFiDevice6G = "MT7990_1_3"

	// WiFiIface struct
	WiFiIface   = "wifi-iface"
	WiFiIface2G = "ra0"
	WiFiIface5G = "rai0"
	WiFiIface6G = "rax0"

	// WiFiMld struct
	WiFiMld = "wifi-mld"
)

// Default WiFiDevice, WiFiIface configs
var (
	WifiDeviceConfig2G = DeviceConfig{Name: WiFiDevice2G, Channel: 1, Disabled: false, Band: WiFiBand2G, Country: "US", HtMode: "EHT40", HtCoex: true}
	WifiDeviceConfig5G = DeviceConfig{Name: WiFiDevice5G, Channel: 36, Disabled: false, Band: WiFiBand5G, Country: "US", HtMode: "EHT160"}
	WifiDeviceConfig6G = DeviceConfig{Name: WiFiDevice6G, Channel: 5, Disabled: false, Band: WiFiBand6G, Country: "US", HtMode: "EHT320"}
	WifiIfaceConfig2G  = IfaceConfig{Name: WiFiIface2G, Disabled: false, Device: WiFiDevice2G, Network: "lan", Mode: "ap", Ieee80211w: 1}
	WifiIfaceConfig5G  = IfaceConfig{Name: WiFiIface5G, Disabled: false, Device: WiFiDevice5G, Network: "lan", Mode: "ap", Ieee80211w: 1}
	WifiIfaceConfig6G  = IfaceConfig{Name: WiFiIface6G, Disabled: false, Device: WiFiDevice6G, Network: "lan", Mode: "ap", Ieee80211w: 1}
)

// DeviceConfig is the configuration to start hostapd on a router.
type DeviceConfig struct {
	Name     string
	Type     string
	Channel  int
	Disabled bool
	Band     string
	Country  string
	HtMode   string
	HtCoex   bool
	HtExtcha bool
}

// IfaceConfig is the configuration to start hostapd on a router.
type IfaceConfig struct {
	Name            string
	Disabled        bool
	VifIdx          int
	Device          string
	Network         string
	Mode            string
	SSID            string
	BSSID           string
	Encryption      string
	Pairwise        []string
	GroupCipher     string
	GroupMgmtCipher string
	Key             string
	Ieee80211w      int
}

// MldConfig is the configuration to start hostapd on a router.
type MldConfig struct {
	Name        string
	Disabled    bool
	Mode        string
	Ifaces      []string
	SSID        string
	Encryption  string
	Key         string
	Ieee80211w  int
	PmfSha256   bool
	SaePassword string
}

// Config is the configuration to start hostapd on a router.
type Config struct {
	DeviceConfigs []DeviceConfig
	IfaceConfigs  []IfaceConfig
	MldConfigs    []MldConfig
}
