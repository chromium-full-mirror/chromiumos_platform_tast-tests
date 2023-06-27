// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/android"
	"go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbyfixture"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbytestutils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrosToPhoneSelfShareAutoAccept,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that we can successfully send files between contacts from CrOS to Android on the same Gaia without accepting",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// "ChromeOS > Software > System Services > Cross Device > Nearby Share"
		BugComponent: "b:1131838",
		Attr:         []string{"group:cross-device", "cross-device_nearbyshare"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			// Stable subset of boards.
			{
				Name:    "dataoffline_allcontacts_png5kb",
				Fixture: "nearbyShareDataUsageOfflineSelfShare",
				Val: nearbycommon.TestData{
					Filename:        "small_png.zip",
					TransferTimeout: nearbycommon.SmallFileTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				},
				ExtraData:         []string{"small_png.zip"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:    "dataonline_allcontacts_txt30mb",
				Fixture: "nearbyShareDataUsageOnlineSelfShare",
				Val: nearbycommon.TestData{
					Filename:        "big_txt.zip",
					TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
				},
				ExtraData:         []string{"big_txt.zip"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},

			// Floss duplicates
			{
				Name:      "dataoffline_allcontacts_png5kb_floss",
				Fixture:   "nearbyShareDataUsageOfflineSelfShareFloss",
				ExtraAttr: []string{"cross-device_floss"},
				Val: nearbycommon.TestData{
					Filename:        "small_png.zip",
					TransferTimeout: nearbycommon.SmallFileTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				},
				ExtraData:         []string{"small_png.zip"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:      "dataonline_allcontacts_txt30mb_floss",
				Fixture:   "nearbyShareDataUsageOnlineSelfShareFloss",
				ExtraAttr: []string{"cross-device_floss"},
				Val: nearbycommon.TestData{
					Filename:        "big_txt.zip",
					TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
				},
				ExtraData:         []string{"big_txt.zip"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
		},
	})
}

// CrosToPhoneSelfShareAutoAccept tests self sharing with a CrOS device as sender and Android device as receiver.
func CrosToPhoneSelfShareAutoAccept(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*nearbyfixture.FixtData).Chrome
	tconn := s.FixtValue().(*nearbyfixture.FixtData).TestConn
	crosDisplayName := s.FixtValue().(*nearbyfixture.FixtData).CrOSDeviceName
	sendDir := s.FixtValue().(*nearbyfixture.FixtData).CrOSSendPath
	androidDevice := s.FixtValue().(*nearbyfixture.FixtData).AndroidDevice
	androidDisplayName := s.FixtValue().(*nearbyfixture.FixtData).AndroidDeviceName

	// Extract the test file(s) to ~/MyFiles/Downloads/nearby_test_files.
	testData := s.Param().(nearbycommon.TestData)
	testDataZip := s.DataPath(testData.Filename)
	filenames, err := nearbytestutils.ExtractCrosTestFiles(ctx, cr, testDataZip)
	if err != nil {
		s.Fatal("Failed to extract test data files: ", err)
	}

	// Get the full paths of the test files to pass to chrome://nearby.
	var testFiles []string
	for _, f := range filenames {
		testFiles = append(testFiles, filepath.Join(sendDir, f))
	}

	s.Log("Starting in-contacts receiving on the Android device")
	testTimeout := testData.TestTimeout
	if err := androidDevice.ReceiveFile(ctx, crosDisplayName, androidDisplayName, false /*isHighVisibility*/, testTimeout); err != nil {
		s.Fatal("Failed to start receiving on Android: ", err)
	}
	// Defer cancelling receiving if something goes wrong.
	var shareCompleted bool
	defer func() {
		if !shareCompleted {
			s.Log("Cancelling receiving")
			if err := screenshot.CaptureChrome(ctx, cr, filepath.Join(s.OutDir(), "after_sharing.png")); err != nil {
				s.Log("Failed to capture a screenshot before cancelling receiving")
			}
			if err := androidDevice.CancelReceivingFile(ctx); err != nil {
				s.Error("Failed to cancel receiving after the share failed: ", err)
			}
			if err := androidDevice.AwaitSharingStopped(ctx, testTimeout); err != nil {
				s.Error("Failed waiting for the Android device to signal that sharing has finished: ", err)
			}
		}
	}()

	s.Log("Starting sending on the CrOS device")
	sender, err := nearbyshare.StartSendFiles(ctx, cr, testFiles)
	if err != nil {
		s.Fatal("Failed to set up control over the send surface: ", err)
	}
	defer sender.Close(ctx)
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	s.Log("Waiting for CrOS sender to detect Android receiver")
	if err := sender.SelectShareTarget(ctx, androidDisplayName, nearbycommon.DetectShareTargetTimeout); err != nil {
		s.Fatal("CrOS device failed to select Android device as a receiver and start the transfer: ", err)
	}

	// Instead of waiting for the confirmation page, we instead wait for the transfer complete notification. Time out is
	// the combination of the Phone recognizing the transfer and the transfer time itself.
	s.Log("Ignoring confirmation page since transfer should be auto accepted. Waiting for the Android receiver to signal that sharing has completed")
	if err := androidDevice.AwaitSharingStopped(ctx, nearbycommon.DetectShareTargetTimeout+testData.TransferTimeout); err != nil {
		s.Fatal("Failed waiting for the Android device to signal that sharing has finished: ", err)
	}
	shareCompleted = true

	// Hash the file on both sides and confirm they match. Android receives shares in its default downloads directory.
	if err := nearbytestutils.FileHashComparison(ctx, filenames, sendDir, android.DownloadDir, androidDevice); err != nil {
		s.Fatal("Failed file hash comparison: ", err)
	}
	s.Log("Share completed and file hashes match on both sides")
}
