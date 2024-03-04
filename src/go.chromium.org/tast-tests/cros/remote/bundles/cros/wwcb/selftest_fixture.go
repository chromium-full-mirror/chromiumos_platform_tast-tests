// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SelftestFixture,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check all Allion fixture functions and whether external devices are properly connected",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "category"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService"},
		Data:         []string{"Capabilities.json"},
	})
}

func SelftestFixture(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dut := s.DUT()

	verifyTimeout, verifyInterval := 15*time.Second, 2*time.Second

	category := s.RequiredVar("category")

	switch category {
	case "docking", "docking_daisychain", "monitor", "monitor_daisychain":
		break
	default:
		s.Fatalf("Failed to unsupported %s category", category)

	}

	// Clear the capabilities json file.
	switch category {
	case "monitor", "monitor_daisychain":
		if err := utils.WriteConfigFile(s, map[string]interface{}{}); err != nil {
			s.Fatal("Failed to write config file: ", err)
		}
	}

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize the fixture: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Retrieve all online fixtures.
	fixtureOnline := utils.GetFixtureOnline()
	if len(fixtureOnline) == 0 {
		s.Fatal("Failed to fixtureOnline is empty")
	}

	// Get original usb list.
	oriUsbList, err := utils.GetUSBDevice(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get usb devices: ", err)
	}

	capabilitiesMap := map[string]interface{}{}
	switch category {
	case "docking", "docking_daisychain":
		capabilitiesMap["Upstream"] = map[string]interface{}{
			"Power supply": map[string]interface{}{},
			"Interface":    map[string]interface{}{},
			"DockingID":    "",
		}
		capabilitiesMap["Downstream"] = map[string]interface{}{
			"USB Type A": map[string]interface{}{},
			"Ethernet": map[string]interface{}{
				"EthernetID": "",
			},
			"Display": map[string]interface{}{},
		}
		capabilitiesMap["utils.wwcbIPPowerIp"] = ""
	case "monitor", "monitor_daisychain":
		capabilitiesMap["Downstream"] = map[string]interface{}{
			"Display": map[string]interface{}{},
		}
	}

	// If the category is "docking" the ID of the docking fixture needs to be retrieved.
	switch category {
	case "docking", "docking_daisychain":
		// Read Capabilities.json to obtain the IP Power IP.
		IppowerIPMap, err := utils.ReadConfigFile(s)
		if err != nil {
			s.Fatal("Failed to read config file: ", err)
		}
		IppowerIP := IppowerIPMap["utils.wwcbIPPowerIp"]
		if IppowerIP == nil {
			s.Fatal("Failed to utils.wwcbIPPowerIp is nil")
		}
		capabilitiesMap["utils.wwcbIPPowerIp"] = IppowerIP.(string)
		utils.SetIppowerIP(IppowerIP.(string))

		// First find the docking statiion.
		for key := range fixtureOnline {
			if strings.Contains(key, "19129") {
				// Open IP power to supply docking power.
				ipPowerPorts := []int{1}
				if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
					s.Fatal("Failed to open IP power: ", err)
				}
				defer utils.CloseIppower(cleanupCtx, ipPowerPorts)

				if err := utils.ControlFixture(ctx, key, "on"); err != nil {
					s.Fatalf("Failed to open fixture: %s", key)
				}
				// GoBigSleepLint: Make sure to detect all interface from the docking station.
				testing.Sleep(ctx, 10*time.Second)
				if err := testing.Poll(ctx, func(ctx context.Context) error {
					usbList, err := utils.GetUSBDevice(ctx, dut)
					if err != nil {
						return errors.Wrapf(err, "failed to get usb devices before connect fixture: %s", key)
					}
					if len(usbList)-len(oriUsbList) <= 0 {
						return errors.New("failed to detect docking station")
					}
					capabilitiesMap["Upstream"].(map[string]interface{})["DockingID"] = key
					oriUsbList = usbList
					return nil
				}, &testing.PollOptions{Timeout: verifyTimeout, Interval: verifyInterval}); err != nil {
					if err := utils.ControlFixture(ctx, key, "off"); err != nil {
						s.Fatalf("Failed to close fixture: %s", key)
					}
				}
				if capabilitiesMap["Upstream"].(map[string]interface{})["DockingID"] != "" {
					defer utils.ControlFixture(ctx, capabilitiesMap["Upstream"].(map[string]interface{})["DockingID"].(string), "off")
					break
				}
			}
		}
		if capabilitiesMap["Upstream"].(map[string]interface{})["DockingID"] == "" {
			s.Fatal("Failed to detect docking station")
		}
	}

	usbCount, displayCount := 0, 0
	// Find other devices.
	for key := range fixtureOnline {
		switch category {
		case "docking", "docking_daisychain":
			// Skip docking station and ethernet fixture.
			if key == capabilitiesMap["Upstream"].(map[string]interface{})["DockingID"] {
				continue
			} else if strings.Contains(key, "j45sw") {
				capabilitiesMap["Downstream"].(map[string]interface{})["Ethernet"].(map[string]interface{})["EthernetID"] = key
				continue
			}
		}

		// Skip when the category contains 'daisychain' and the key is DP fixture.
		switch category {
		case "docking_daisychain", "monitor_daisychain":
			if strings.Contains(key, "21090") {
				continue
			}
		}

		if err := utils.ControlFixture(ctx, key, "on"); err != nil {
			s.Fatalf("Failed to open fixture: %s before connect docking station", key)
		}
		fixtureType := ""
		// Check the fixture type.
		if strings.Contains(key, "19129") {
			fixtureType = "Type C"
		} else if strings.Contains(key, "20079") {
			fixtureType = "HDMI"
		} else if strings.Contains(key, "21090") {
			fixtureType = "DP"
		}
		switch category {
		case "docking", "docking_daisychain":
			// Obtain USBA, display fixture ID.
			if err := utils.VerifyUSBDeviceConnectionChangeCount(ctx, dut, len(oriUsbList), 1); err == nil {
				usbCount++
				capabilitiesUSBA := capabilitiesMap["Downstream"].(map[string]interface{})["USB Type A"].(map[string]interface{})
				capabilitiesUSBA[fmt.Sprint("Port", usbCount)] = map[string]interface{}{"Gen": "", "USBTypeAIDArray": key}
			}
			if category == "docking" {
				if err := utils.VerifyDisplayCount(ctx, dut, 2); err == nil {
					displayCount++
					capabilitiesDis := capabilitiesMap["Downstream"].(map[string]interface{})["Display"].(map[string]interface{})
					capabilitiesDis[fmt.Sprint("Port", displayCount)] = map[string]interface{}{"Type": fixtureType, fmt.Sprint("ExtDispID", displayCount): key}
				}
			}
		case "monitor", "monitor_daisychain":
			// Obtain display fixture ID.
			if err := utils.VerifyDisplayCount(ctx, dut, 2); err == nil {
				displayCount++
				capabilitiesDis := capabilitiesMap["Downstream"].(map[string]interface{})["Display"].(map[string]interface{})
				capabilitiesDis[fmt.Sprint("Port", displayCount)] = map[string]interface{}{"Type": fixtureType, fmt.Sprint("ExtDispID", displayCount): key}
			}
		}

		if err := utils.ControlFixture(ctx, key, "off"); err != nil {
			s.Fatalf("Failed to close fixture:%s before connect docking station", key)
		}
	}

	switch category {
	case "docking_daisychain", "monitor_daisychain":
		// Obtain display fixture ID.
		dpFixtureIDs := []string{}
		for key := range fixtureOnline {
			if strings.Contains(key, "21090") {
				dpFixtureIDs = append(dpFixtureIDs, key)
			}
		}
		// Verify display count.
		for _, fixID := range dpFixtureIDs {
			if err := utils.ControlFixture(ctx, fixID, "on"); err != nil {
				s.Fatalf("Failed to open fixture: %s before connect docking station", fixID)
			}
		}
		if err := utils.VerifyDisplayCount(ctx, dut, 3); err == nil {
			for _, fixID := range dpFixtureIDs {
				displayCount++
				if category == "docking_daisychain" {
					capabilitiesDis := capabilitiesMap["Downstream"].(map[string]interface{})["Display"].(map[string]interface{})
					capabilitiesDis[fmt.Sprint("Port", displayCount)] = map[string]interface{}{"Type": "DP", fmt.Sprint("ExtDispID", displayCount): fixID}
				} else {
					capabilitiesDis := capabilitiesMap["Downstream"].(map[string]interface{})["Display"].(map[string]interface{})
					capabilitiesDis[fmt.Sprint("Port", displayCount)] = map[string]interface{}{"Type": "DP", fmt.Sprint("ExtDispID", displayCount): fixID}
				}
			}
		}
		for _, fixID := range dpFixtureIDs {
			if err := utils.ControlFixture(ctx, fixID, "off"); err != nil {
				s.Fatalf("Failed to close fixture:%s before connect docking station", fixID)
			}
		}
	}

	switch category {
	case "docking", "docking_daisychain":
		if capabilitiesMap["Downstream"].(map[string]interface{})["Ethernet"].(map[string]interface{})["EthernetID"] == "" {
			s.Fatal("Failed to detect ethernet fixture")
		}
	}

	if err := utils.WriteConfigFile(s, capabilitiesMap); err != nil {
		s.Fatal("Failed to write config file: ", err)
	}
}
