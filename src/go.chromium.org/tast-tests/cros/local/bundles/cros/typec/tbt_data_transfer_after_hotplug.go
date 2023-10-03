// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/typec/setup"
	typec "go.chromium.org/tast-tests/cros/local/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TbtDataTransferAfterHotplug,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "TBT data tarnsfer after hot plug",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Attr:         []string{"group:typec", "group:intel-tbt3-dock-usb"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"test_config.json", "testcert.p12"},
		Vars:         []string{"typec.dutTbtPort", "typec.cSwitchPort", "typec.sdCardName", "typec.dutSsdTbtPort", "typec.ssdName", "typec.domainIP", "ui.signinProfileTestExtensionManifestKey"},
		HardwareDeps: hwdep.D(setup.ThunderboltSupportedDevices()),
		Timeout:      160 * time.Minute,
	})
}

// TbtDataTransferAfterHotplug performs the following:
// - Hot plug TBT Device into DUT with help of cswitch.
// - Performs TBT Device enumeration check.
// - Performs data transfer from DUT to TBT device.
// - Unplug TBT device from DUT and validates TBT device detection.
//
// This test requires the following H/W topology to run.
//
//	DUT ------> C-Switch(device that performs hot plug-unplug)---->TBT SSD, SD card.
func TbtDataTransferAfterHotplug(ctx context.Context, s *testing.State) {

	const (
		// Config file which contains expected values of TBT parameters.
		jsonTestConfig = "test_config.json"
		// SSD TransFile name.
		ssdTransFilename = "file_ogg_tbt_ssd.txt"
		// SD Card TransFile name.
		sdCardTransFilename = "file_ogg_sdcard.txt"
		// TBT SSD data file size.
		ssdDataFileSize = 1024 * 1024 * 1024 * 20
		// SD card data file size.
		sdCardDataFileSize = 1024 * 1024 * 1024 * 5
	)

	// TBT port ID in the DUT.
	tbtPort := s.RequiredVar("typec.dutTbtPort")
	// SSD TBT port ID in the DUT.
	ssdTbtPort := s.RequiredVar("typec.dutSsdTbtPort")
	// cswitch port ID.
	cSwitchON := s.RequiredVar("typec.cSwitchPort")
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("typec.domainIP")
	// SD CardDevice Name
	sdCardDeviceName := s.RequiredVar("typec.sdCardName")
	// TBT SSD Device Name
	ssdDeviceName := s.RequiredVar("typec.ssdName")
	const cSwitchOFF = "0"

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Get to the Chrome login screen.
	cr, err := chrome.New(ctx,
		chrome.DeferLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))
	if err != nil {
		s.Fatal("Failed to start Chrome at login screen: ", err)
	}
	defer cr.Close(ctx)

	if err := typecutils.EnablePeripheralDataAccess(ctx, s.DataPath("testcert.p12")); err != nil {
		s.Fatal("Failed to enable peripheral data access setting: ", err)
	}

	if err := cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	// Read json config file.
	jsonData, err := ioutil.ReadFile(s.DataPath(jsonTestConfig))
	if err != nil {
		s.Fatal("Failed to read response data: ", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		s.Fatal("Failed to read json: ", err)
	}

	// Checking for TBT config data.
	tbtVal, ok := data["TBT"].(map[string]interface{})
	if !ok {
		s.Fatal("Failed to found TBT config data in JSON file")
	}

	// Create C-Switch session that performs hot plug-unplug on TBT device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create session: ", err)
	}

	defer func(ctx context.Context) {
		s.Log("Cleanup")
		if err := cswitch.CloseSession(ctx, sessionID, domainIP); err != nil {
			s.Log("Failed to close session: ", err)
		}
	}(cleanupCtx)

	for i := 1; i <= 10; i++ {
		s.Logf("Hotplug - unplug iteration: %d/10", i)
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchON, domainIP); err != nil {
			s.Fatal("Failed to enable c-switch port: ", err)
		}

		// Verifying Enumeration of TBT Dock station.
		if _, err := cswitch.IsDeviceEnumerated(ctx, tbtVal["device_name"].(string), tbtPort); err != nil {
			s.Fatal("Failed to enumerate TBT device: ", err)
		}

		// Verifying Enumeration of TBT SSD.
		if _, err := cswitch.IsDeviceEnumerated(ctx, tbtVal["device_detection"].(string), ssdTbtPort); err != nil {
			s.Fatal("Failed to enumerate TBT device: ", err)
		}

		tbtGeneration, err := cswitch.Generation(ctx, tbtPort)
		if err != nil {
			s.Fatal("Failed to get TBT genaration: ", err)
		}
		if strings.TrimSpace(tbtGeneration) != tbtVal["generation"].(string) {
			s.Fatalf("Failed to verify the generation, got %s, want %s", tbtGeneration, tbtVal["generation"].(string))
		}

		sourcePath, err := ioutil.TempDir("", "temp")
		if err != nil {
			s.Fatal("Failed to create temp directory: ", err)
		}
		defer os.RemoveAll(sourcePath)

		s.Log("Checking the device path of SSD")
		ssdDevicePath, err := usbutils.StoragePath(ctx, ssdDeviceName)
		if err != nil {
			s.Fatal("Failed to get ssd device path: ", err)
		}

		s.Log("Checking the device path of SD_CARd")
		sdDevicePath, err := usbutils.StoragePath(ctx, sdCardDeviceName)
		if err != nil {
			s.Fatal("Failed to get sd card device path: ", err)
		}

		ssdSourceFilePath := path.Join(sourcePath, ssdTransFilename)
		if err := typec.GenerateFileWithSize(ctx, ssdSourceFilePath, ssdDataFileSize); err != nil {
			s.Fatal("Failed to generate 20gb file : ", err)
		}

		sdCardSourceFilePath := path.Join(sourcePath, sdCardTransFilename)
		if err := typec.GenerateFileWithSize(ctx, sdCardSourceFilePath, sdCardDataFileSize); err != nil {
			s.Fatal("Failed to generate 5gb file : ", err)
		}

		ssdDestinationFilePath := path.Join(ssdDevicePath, ssdTransFilename)
		if err := usbutils.TransferFile(ctx, ssdSourceFilePath, ssdDestinationFilePath, true); err != nil {
			s.Fatalf("Failed to copy data from %s to %s: %v", ssdSourceFilePath, ssdDestinationFilePath, err)
		}
		defer os.Remove(ssdDestinationFilePath)

		sdCardDestinationFilePath := path.Join(sdDevicePath, sdCardTransFilename)
		if err := usbutils.TransferFile(ctx, sdCardSourceFilePath, sdCardDestinationFilePath, true); err != nil {
			s.Fatalf("Failed to copy data from %s to %s: %v", sdCardSourceFilePath, sdCardDestinationFilePath, err)
		}
		defer os.Remove(sdCardDestinationFilePath)

		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
		}

		if _, err := cswitch.IsDeviceEnumerated(ctx, tbtVal["device_name"].(string), tbtPort); err == nil {
			s.Fatal("Failed to hot unplug the TBT device: ", err)
		}
	}
}
