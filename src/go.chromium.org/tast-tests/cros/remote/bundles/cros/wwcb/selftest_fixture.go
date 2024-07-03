// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
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
		Timeout:      20 * time.Minute,
	})
}
func SelftestFixture(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	// Extend the timeout to 90 seconds to make sure the test can be completed.
	ctx, cancel := ctxutil.Shorten(ctx, 90*time.Second)
	defer cancel()
	dut := s.DUT()
	if err := dut.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot dut: ", err)
	}
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
			"Interface":    "",
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
				capabilitiesUSBA[fmt.Sprint("Port", usbCount)] = map[string]interface{}{"Speed": "", "USBTypeAIDArray": key}
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
		cap, err := identifyPeripheralsCapabilities(ctx, dut, capabilitiesMap)
		if err != nil {
			s.Fatal("Failed to identify the device capabilities: ", err)
		}
		if err := utils.WriteConfigFile(s, cap); err != nil {
			s.Fatal("Failed to write config file: ", err)
		}
	case "monitor", "monitor_daisychain":
		if err := utils.WriteConfigFile(s, capabilitiesMap); err != nil {
			s.Fatal("Failed to write config file: ", err)
		}
	}
}

// identifyPeripheralsCapabilities returns the peripherals capabilities by controlling the fixtures and retrieving the system information
func identifyPeripheralsCapabilities(ctx context.Context, dut *dut.DUT, cap map[string]interface{}) (map[string]interface{}, error) {
	if err := utils.CloseAllFixture(ctx); err != nil {
		return cap, errors.Wrap(err, "failed to close all fixture")
	}
	defaultEthernets, err := utils.ListEthernets(ctx, dut)
	if err != nil {
		return cap, errors.Wrap(err, "failed to list Ethernets")
	}
	dockingID := cap["Upstream"].(map[string]interface{})["DockingID"].(string)
	if dockingID != "" {
		// Retrieve the docking station power supply.
		dockCharger, err := utils.FindDockingPowerPath(ctx, dut)
		if err != nil {
			return cap, errors.Wrap(err, "failed to find the docking power path")
		}
		testing.ContextLogf(ctx, "docking power path:%s", dockCharger)
		// Retrieve the docking interface.
		dockingPort, err := utils.FindDockingConnectPort(ctx, dut, dockingID)
		if err != nil {
			return cap, errors.Wrap(err, "failed to find the docking port")
		}
		testing.ContextLog(ctx, "Found the docking port: ", dockingPort)
		dockingInterface, err := utils.FindUSBConnectStatus(ctx, dut, dockingPort)
		if err != nil {
			return cap, errors.Wrap(err, "failed to find the docking interface")
		}
		testing.ContextLog(ctx, "Found the docking status: ", dockingInterface)
		cap["Upstream"].(map[string]interface{})["Interface"] = dockingInterface
		// Retrieve the docking power charging voltage and amp.
		var powerInfo []float64
		for _, info := range []string{"voltage_max_design", "current_max"} {
			command := fmt.Sprintf("cat /sys/class/power_supply/%s/%s", dockCharger, info)
			out, err := dut.Conn().CommandContext(ctx, "sh", "-c", command).Output()
			if err != nil {
				return cap, errors.Wrapf(err, "failed to execute command %q", command)
			}
			output, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
			if err != nil {
				return cap, errors.Wrap(err, "convert string to float")
			}
			powerInfo = append(powerInfo, output)
		}
		var baseValue float64 = 1000000
		watts := int(powerInfo[0] / baseValue * powerInfo[1] / baseValue)
		capPowerSupply := cap["Upstream"].(map[string]interface{})["Power supply"].(map[string]interface{})
		capPowerSupply["voltage"] = fmt.Sprintf("%vv", int(powerInfo[0]/baseValue))
		capPowerSupply["amp"] = fmt.Sprintf("%va", int(powerInfo[1]/baseValue))
		capPowerSupply["watts"] = fmt.Sprintf("%dw", watts)
	}
	// Retrieve the lan port speed.
	ethernetID := cap["Downstream"].(map[string]interface{})["Ethernet"].(map[string]interface{})["EthernetID"].(string)
	if ethernetID != "" {
		// Find Dock Ethernet on DUT.
		if err := utils.ControlFixture(ctx, ethernetID, "on"); err != nil {
			return cap, errors.Wrap(err, "failed to connect the Ethernet")
		}
		dockEth, err := utils.FindDockEthernet(ctx, dut, defaultEthernets)
		if err != nil {
			return cap, errors.Wrap(err, "failed to find dock Ethernet")
		}
		testing.ContextLog(ctx, "Found the Dock Ethernet: ", dockEth)
		// GoBigSleepLint: Wait for docking eth working.
		testing.Sleep(ctx, 5*time.Second)
		ethSpeed, err := utils.FindEthernetSpeed(ctx, dut, dockEth)
		if err != nil {
			return cap, errors.Wrap(err, "failed to find the ethernet speed")
		}
		testing.ContextLog(ctx, "Found the Dock Ethernet speed: ", ethSpeed)
		cap["Downstream"].(map[string]interface{})["Ethernet"].(map[string]interface{})["Speed"] = ethSpeed
	}
	// Retrieve the USB Type-A Speed.
	usbTypeADevices := cap["Downstream"].(map[string]interface{})["USB Type A"].(map[string]interface{})
	if len(usbTypeADevices) != 0 {
		for port, device := range usbTypeADevices {
			fixtureID := device.(map[string]interface{})["USBTypeAIDArray"].(string)
			usbSpeed, err := utils.FindDeviceSpeed(ctx, dut, fixtureID)
			if err != nil {
				return cap, errors.Wrapf(err, "failed to find the device speed, fixture ID:%s", fixtureID)
			}
			cap["Downstream"].(map[string]interface{})["USB Type A"].(map[string]interface{})[port].(map[string]interface{})["Speed"] = usbSpeed
		}
	}
	return cap, nil
}
