// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type urlDetails struct {
	fileurl  string
	filename string
	filesize int64
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Download,
		Desc:         "Downloads a file from internet using WiFi",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		TestBedDeps:  []string{tbdep.WifiStateNormal},
		Fixture:      "chromeLoggedIn",
		Attr:         []string{"group:intel-wlan"},
		Vars:         []string{"wifissid", "wifipassword", "iterations", "wifi.usbDetectionName"},
		Params: []testing.Param{{
			Name: "quick",
			Val: urlDetails{
				fileurl:  "https://dl.google.com/dl/android/cts/android-cts-verifier-9.0_r15-linux_x86-x86.zip",
				filename: "android-cts-verifier-9.0_r15-linux_x86-x86.zip",
				filesize: 16490022,
			},
			Timeout: 5 * time.Minute,
		}, {
			Name: "stress",
			Val: urlDetails{
				fileurl:  "https://dl.google.com/dl/android/cts/android-cts-media-1.2.zip",
				filename: "android-cts-media-1.2.zip",
				filesize: 2314239713,
			},
			Timeout: 20 * time.Minute,
		}},
	})
}

// parseIntVar checks for the passed var, else returns default int value.
func parseIntVar(s *testing.State, name string, defaultValue int) int {
	str, ok := s.Var(name)
	if !ok {
		return defaultValue
	}

	val, err := strconv.Atoi(str)
	if err != nil {
		s.Errorf("Failed to parse integer variable %v: %v", name, err)
	}
	return val
}

// Download downloads a file over wifi.
func Download(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	defaultIterations := 1
	downloadURL := s.Param().(urlDetails).fileurl
	fileName := s.Param().(urlDetails).filename
	fileSize := s.Param().(urlDetails).filesize
	iterations := parseIntVar(s, "iterations", defaultIterations)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	ssid := s.RequiredVar("wifissid")
	wifiPwd := s.RequiredVar("wifipassword")

	var wifi *shill.WifiManager
	if wifi, err = shill.NewWifiManager(ctx, nil); err != nil {
		s.Fatal("Failed to create shill Wi-Fi manager: ", err)
	}
	// Ensure wi-fi is enabled.
	if err := wifi.Enable(ctx, true); err != nil {
		s.Fatal("Failed to enable Wi-Fi: ", err)
	}
	s.Log("Wi-Fi is enabled")
	if err := wifi.ConnectAP(ctx, ssid, wifiPwd); err != nil {
		s.Fatalf("Failed to connect Wi-Fi AP %s: %v", ssid, err)
	}
	s.Logf("Wi-Fi AP %s is connected", ssid)
	var destinationFilePath string
	usbDeviceName, verifyPendrive := s.Var("wifi.usbDetectionName")
	if verifyPendrive {
		// Verify USB pendrive speed.
		// For local-side dut parameter must be nil
		usbDevicesList, err := usbutils.ListDevicesInfo(ctx, nil)
		if err != nil {
			s.Fatal("Failed to get USB devices list: ", err)
		}
		const (
			mediaRemovable     = "/media/removable/"
			usbDeviceClassName = "Mass Storage"
			usbSpeed           = "5000M"
		)

		got := usbutils.NumberOfUSBDevicesConnected(usbDevicesList, usbDeviceClassName, usbSpeed)
		if want := 1; got < want {
			s.Fatalf("Unexpected number of USB devices connected: got %d, want %d", got, want)
		}
		destinationFilePath = path.Join(mediaRemovable, usbDeviceName)
		if _, err := os.Stat(destinationFilePath); err != nil {
			s.Fatal("Invalid USB pendrive path: ", err)
		}
	} else {
		downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to get users Download path: ", err)
		}
		destinationFilePath = downloadsPath
	}
	for i := 1; i <= iterations; i++ {
		s.Run(ctx, strconv.Itoa(i), func(ctx context.Context, s *testing.State) {
			if err := downloadFile(ctx, downloadURL, filepath.Join(destinationFilePath, fileName), fileSize); err != nil {
				s.Fatal("Failed to download file over WiFi: ", err)
			}
			defer os.Remove(filepath.Join(destinationFilePath, fileName))
			if err := testFile(ctx, fileName, tconn, verifyPendrive, usbDeviceName); err != nil {
				s.Fatal("Failed to downlaod file over WiFi: ", err)
			}
		},
		)
	}
}

// downloadFile downloads the given file from the URL.
func downloadFile(ctx context.Context, url, downloadPath string, fileSize int64) error {
	testing.ContextLogf(ctx, "Downloading %s to %s", url, downloadPath)
	dest, err := os.Create(downloadPath)
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}
	defer dest.Close()

	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "failed to download file")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to download: %s", resp.Status)
	}
	if _, err := io.Copy(dest, resp.Body); err != nil {
		return errors.Wrap(err, "failed to copy file")
	}
	fi, err := dest.Stat()
	if err != nil {
		return errors.Wrap(err, "failed to get file size")
	}
	if fileSize != fi.Size() {
		return errors.Wrapf(err, "failed to match file size want %d, got %d", fileSize, fi.Size())
	}
	return nil
}

// testFile verifies if the downloaded file is present in Downloads path.
func testFile(ctx context.Context, fileName string, tconn *chrome.TestConn, usbDevice bool, usbDeviceName string) error {
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch Files App")
	}
	defer files.Close(ctx)

	if usbDevice {
		if err := files.OpenUSBDriveWithName(usbDeviceName)(ctx); err != nil {
			return errors.Wrap(err, "failed to open USB drive")
		}
	} else {
		if err := files.OpenDownloads()(ctx); err != nil {
			return errors.Wrap(err, "failed to open the downloads folder")
		}
	}
	return files.FileExists(fileName)(ctx)
}
