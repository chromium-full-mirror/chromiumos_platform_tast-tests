// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"bytes"
	"context"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	// Pre-requisite: Connect Type-A USB 3.0 pendrive to the DUT.
	testing.AddTest(&testing.Test{
		Func:         CopyFiles,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Copy files between Downloads and USB (and vice versa)",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:intel-usb-set1"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Vars:         []string{"typec.usbDetectionName"},
		Fixture:      "chromeLoggedIn",
		Timeout:      7 * time.Minute,
	})
}

func CopyFiles(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	const (
		GB       = 1024 * 1024 * 1024 // 1 GigaByte size.
		fileName = "usb_sample_file.txt"
		// mediaRemovable is removable media path.
		mediaRemovable     = "/media/removable/"
		usbDeviceClassName = "Mass Storage"
		usbSpeed           = "5000M"
	)

	// Verify USB pendrive speed.
	usbDevicesList, err := usbutils.ListDevicesInfo(ctx, nil)
	if err != nil {
		s.Fatal("Failed to get USB devices list: ", err)
	}

	got := usbutils.NumberOfUSBDevicesConnected(usbDevicesList, usbDeviceClassName, usbSpeed)
	if want := 1; got < want {
		s.Fatalf("Unexpected number of USB devices connected: got: %d, want: %d", got, want)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	// Download file path.
	downloadFilePath := path.Join(downloadsPath, fileName)
	defer os.RemoveAll(downloadFilePath)

	// Create a file with size.
	file, err := os.Create(downloadFilePath)
	if err != nil {
		s.Fatal("Failed to create file: ", err)
	}
	if err := file.Truncate(int64(1 * GB)); err != nil {
		s.Fatal("Failed to truncate file with size: ", err)
	}

	usbDeviceName := s.RequiredVar("typec.usbDetectionName")
	usbFilePath := path.Join(mediaRemovable, usbDeviceName, fileName)
	defer os.RemoveAll(usbFilePath)

	localHash, err := typecutils.FileChecksum(downloadFilePath)
	if err != nil {
		s.Error("Failed to calculate hash of the source file: ", err)
	}

	// Transferring file from source to destination.
	s.Logf("Transferring file from %s to %s", downloadFilePath, usbFilePath)
	if err := typecutils.CopyFile(downloadFilePath, usbFilePath); err != nil {
		s.Fatal("Failed to copy file: ", err)
	}

	if err := os.Remove(downloadFilePath); err != nil {
		s.Fatal("Failed to remove file: ", err)
	}

	destHash, err := typecutils.FileChecksum(usbFilePath)
	if err != nil {
		s.Error("Failed to calculate hash of the destination file: ", err)
	}

	if !bytes.Equal(localHash, destHash) {
		s.Errorf("The hash doesn't match want: %q, got: %q", localHash, destHash)
	}

	// Transferring file vice-versa.
	s.Logf("Transferring file from %s to %s", usbFilePath, downloadFilePath)
	if err := typecutils.CopyFile(usbFilePath, downloadFilePath); err != nil {
		s.Fatal("Failed to copy file: ", err)
	}

	srcHash, err := typecutils.FileChecksum(downloadFilePath)
	if err != nil {
		s.Error("Failed to calculate hash of the destination file: ", err)
	}

	if !bytes.Equal(srcHash, destHash) {
		s.Errorf("The hash doesn't match want: %q, got: %q", destHash, srcHash)
	}

}
