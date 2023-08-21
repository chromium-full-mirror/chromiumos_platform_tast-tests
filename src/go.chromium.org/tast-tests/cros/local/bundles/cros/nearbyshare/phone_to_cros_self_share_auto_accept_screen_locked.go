// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android"
	"go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbyfixture"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbysnippet"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbytestutils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhoneToCrosSelfShareAutoAcceptScreenLocked,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that we can successfully send files from Android to CrOS on the same Gaia without accepting when the Chromebook is locked",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// "ChromeOS > Software > System Services > Cross Device > Nearby Share"
		BugComponent: "b:1131838",
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
				ExtraAttr:         []string{"group:cross-device", "cross-device_nearbyshare"},
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
				ExtraAttr:         []string{"group:cross-device", "cross-device_nearbyshare"},
				ExtraData:         []string{"big_txt.zip"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},

			// Android Nearby prod tests
			{
				Name:    "dataoffline_allcontacts_jpg11kb_prod",
				Fixture: "nearbyShareDataUsageOfflineSelfShareProd",
				Val: nearbycommon.TestData{
					Filename:        "small_jpg.zip",
					TransferTimeout: nearbycommon.SmallFileTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
					MimeType:        nearbycommon.MimeTypeJpeg,
				},
				ExtraAttr: []string{"group:cross-device", "cross-device_nearbyshare-prod"},
				ExtraData: []string{"small_jpg.zip"},
				Timeout:   nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},

			// Android Nearby Dev tests
			{
				Name:    "dataoffline_allcontacts_jpg11kb_dev",
				Fixture: "nearbyShareDataUsageOfflineSelfShareDev",
				Val: nearbycommon.TestData{
					Filename:        "small_jpg.zip",
					TransferTimeout: nearbycommon.SmallFileTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
					MimeType:        nearbycommon.MimeTypeJpeg,
				},
				ExtraAttr: []string{"group:cross-device", "cross-device_nearbyshare-dev"},
				ExtraData: []string{"small_jpg.zip"},
				Timeout:   nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},

			// Floss duplicates
			{
				Name:    "dataoffline_allcontacts_png5kb_floss",
				Fixture: "nearbyShareDataUsageOfflineSelfShareFloss",
				Val: nearbycommon.TestData{
					Filename:        "small_png.zip",
					TransferTimeout: nearbycommon.SmallFileTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				},
				ExtraData:         []string{"small_png.zip"},
				ExtraAttr:         []string{"group:cross-device", "cross-device_nearbyshare", "cross-device_floss"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:    "dataonline_allcontacts_txt30mb_floss",
				Fixture: "nearbyShareDataUsageOnlineSelfShareFloss",
				Val: nearbycommon.TestData{
					Filename:        "big_txt.zip",
					TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
				},
				ExtraData:         []string{"big_txt.zip"},
				ExtraAttr:         []string{"group:cross-device", "cross-device_nearbyshare", "cross-device_floss"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
		},
	})
}

// PhoneToCrosSelfShareAutoAcceptScreenLocked tests file sharing with the same Gaia for an Android device as sender and CrOS device as receiver.
func PhoneToCrosSelfShareAutoAcceptScreenLocked(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*nearbyfixture.FixtData).Chrome
	tconn := s.FixtValue().(*nearbyfixture.FixtData).TestConn
	crosDisplayName := s.FixtValue().(*nearbyfixture.FixtData).CrOSDeviceName
	crosUsername := s.FixtValue().(*nearbyfixture.FixtData).CrOSUsername
	crosPassword := s.FixtValue().(*nearbyfixture.FixtData).CrOSPassword
	androidDevice := s.FixtValue().(*nearbyfixture.FixtData).AndroidDevice
	androidDisplayName := s.FixtValue().(*nearbyfixture.FixtData).AndroidDeviceName

	// Open a keyboard device.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open keyboard device: ", err)
	}
	defer kb.Close(ctx)

	// Extract the test file to the staging directory on the Android device.
	testData := s.Param().(nearbycommon.TestData)
	testDataZip := s.DataPath(testData.Filename)
	testFile, err := nearbytestutils.ExtractAndroidTestFile(ctx, testDataZip, androidDevice)
	if err != nil {
		s.Fatal("Failed to extract test data files: ", err)
	}

	s.Log("Locking the ChromeOS screen")
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen on ChromeOS: ", err)
	}

	s.Log("Waiting for locked screen state to be verified")
	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
		s.Fatalf("Waiting for screen to be locked failed: %v (last status %+v)", err, st)
	}

	s.Log("Starting sending on the Android device")
	testTimeout := testData.TestTimeout
	mimetype := testData.MimeType
	if err := androidDevice.SendFile(ctx, androidDisplayName, crosDisplayName, testFile, mimetype, testTimeout); err != nil {
		s.Fatal("Failed to start sending on Android: ", err)
	}

	s.Log("Unlocking the screen with password")
	if err := lockscreen.UnlockWithPassword(ctx, tconn, crosUsername, crosPassword, kb, 10*time.Second, 30*time.Second); err != nil {
		s.Fatal("Failed to unlock the screen: ", err)
	}

	// Defer cancelling the share on the Android side if it does not succeed.
	var shareCompleted bool
	defer func() {
		if !shareCompleted {
			s.Log("Cancelling sending")
			if err := screenshot.CaptureChrome(ctx, cr, filepath.Join(s.OutDir(), "after_sharing.png")); err != nil {
				s.Log("Failed to capture a screenshot before cancelling sending")
			}
			if err := androidDevice.CancelSendingFile(ctx); err != nil {
				s.Error("Failed to cancel sending after the share failed: ", err)
			}
			if err := androidDevice.AwaitSharingStopped(ctx, testTimeout); err != nil {
				s.Error("Failed waiting for the Android device to signal that sharing has finished: ", err)
			}
		}
	}()

	s.Log("Waiting for receiving-complete notification on CrOS receiver")
	if err := nearbyshare.WaitForReceivingCompleteNotification(ctx, tconn, androidDisplayName, nearbycommon.DetectShareTargetTimeout); err != nil {
		s.Fatal("Failed waiting for notification to indicate sharing has completed on CrOS: ", err)
	}

	shareCompleted = true

	s.Log("Comparing Android and CrOS file hashes")
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	if err := nearbytestutils.FileHashComparison(ctx, []string{testFile}, downloadsPath, filepath.Join(android.DownloadDir, nearbysnippet.SendDir), androidDevice); err != nil {
		s.Fatal("Failed file hash comparison: ", err)
	}
	s.Log("Share completed and file hashes match on both sides")
}
