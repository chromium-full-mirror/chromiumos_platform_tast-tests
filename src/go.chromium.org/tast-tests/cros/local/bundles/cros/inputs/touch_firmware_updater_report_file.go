// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var unsupportedReferenceModels = []string{
	// There are no plans to add touch firmware updaters to these reference
	// models.
	"brox",
}

var unstableModels = []string{
	// TODO: b/311252896 - Undo skip after fix.
	"ciri",
	// TODO: b/376055193 - Undo skip after fix.
	"wugtrio",
}

func init() {
	testing.AddTest(&testing.Test{
		Func: TouchFirmwareUpdaterReportFile,
		Desc: "Validates touch firmware information is always available after boot and not empty",
		Contacts: []string{
			"chromeos-tango@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		BugComponent: "b:167253", // ChromeOS > Platform > baseOS > Input > Touchpad
		// Skip form factors that do not have built-in touchpads or touchscreens.
		HardwareDeps: hwdep.D(hwdep.SkipOnFormFactor(hwdep.Chromebit, hwdep.Chromebox),
			// Skip unsupported/unstable models.
			hwdep.SkipOnModel(append(unsupportedReferenceModels, unstableModels...)...)),
		// Skip vms since this test is for functionality that is not available in VMs.
		SoftwareDeps: []string{"chrome", "chrome_internal", "no_qemu"},
	})
}

const (
	// FirmwareReportFilePath is the path to Firmware Report on Devices.
	firmwareReportFilePath = "/run/touch-updater/firmware-versions"
)

func TouchFirmwareUpdaterReportFile(ctx context.Context, s *testing.State) {
	firmwareReportFile, err := os.Open(firmwareReportFilePath)
	if err != nil {
		s.Fatal("Failed to open touch firmware report file: ", err)
	}
	defer firmwareReportFile.Close()

	fileStats, err := firmwareReportFile.Stat()
	if err != nil {
		s.Fatal("Failed to read stats of touch firmware report file: ", err)
	}

	// Check if firmware file is empty.
	if fileStats.Size() == 0 {
		s.Fatal("Failed because touch firmware report file is empty")
	}

	if err := processEntries(firmwareReportFile); err != nil {
		s.Fatal("Failed to process the device entries in the firmware report file: ", err)
	}
}

// Struct to convert to JSON.
type device struct {
	Path           string
	Updater        string `json:"updater"`
	InitialVersion string `json:"initial_version"`
	UpdateStatus   string `json:"update_status"`
}

func processEntries(file *os.File) error {
	// Checks that each device entry in the file is valid, returns any errors.

	deviceMap := make(map[string]device)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var lineText = scanner.Text()

		// Get Device name and creating JSON values from line.
		var splitLine = strings.SplitN(lineText, " ", 2)
		var deviceName = splitLine[0]

		// Convert JSON to struct so that fields can be accessed.
		var deviceInfo device
		deviceInfo.Path = deviceName
		if err := json.Unmarshal([]byte(splitLine[1]), &deviceInfo); err != nil {
			return err
		}

		// Check if there was a previous entry for the device.
		existingInfo, ok := deviceMap[deviceName]
		if ok { // If there is an existing entry for deviceName, replace values.
			if deviceInfo.Updater != "" {
				existingInfo.Updater = deviceInfo.Updater
			}
			if deviceInfo.InitialVersion != "" {
				existingInfo.InitialVersion = deviceInfo.InitialVersion
			}
			if deviceInfo.UpdateStatus != "" {
				existingInfo.UpdateStatus = deviceInfo.UpdateStatus
			}
			deviceMap[deviceName] = existingInfo
		} else { // If there is no existing entry, create a new entry.
			deviceMap[deviceName] = deviceInfo
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	var errs error

	// Check that each device struct is valid.
	for deviceName := range deviceMap {
		if err := validateDevice(deviceMap[deviceName]); err != nil {
			errs = errors.Join(errs, err) // Accumulate errors from every detected device.
		}
	}

	return errs
}

func validateDevice(deviceInfo device) error {
	// deviceInfo should have initialized values for updater, initial_version.
	var errs error

	if len(deviceInfo.Updater) == 0 {
		errs = errors.New("Missing updater field for device at " + deviceInfo.Path)
	}

	if len(deviceInfo.InitialVersion) == 0 {
		errs = errors.Join(errs, errors.New("Missing initial_version field for device at "+deviceInfo.Path+" ; updater: "+deviceInfo.Updater))
	}

	if len(deviceInfo.UpdateStatus) > 0 && deviceInfo.UpdateStatus != "SUCCESS" {
		errs = errors.Join(errs, errors.New("update_status field was \""+deviceInfo.UpdateStatus+"\", not \"SUCCESS\" for device at "+deviceInfo.Path+" ; updater: "+deviceInfo.Updater))
	}

	return errs
}
