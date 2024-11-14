// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbyfixture"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbytestutils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PhoneToCrosBackgroundScanningDisabled,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that \"Nearby Device is trying to share\" notification shows up, clicking the notification enables high-vis mode and the receive flow is successful",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"hansenmichael@google.com",
		},
		BugComponent: "b:1131838",
		Attr:         []string{"group:cross-device", "cross-device_nearbyshare"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "no_notification_shown",
				Fixture: "nearbyShareDataUsageOfflineNoOneBackgroundScanningEnabled",
				Val: nearbycommon.TestData{
					Filename:        "small_jpg.zip",
					TransferTimeout: nearbycommon.SmallFileTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
					MimeType:        nearbycommon.MimeTypePng,
				},
				ExtraData:         []string{"small_jpg.zip"},
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				ExtraHardwareDeps: hwdep.D(hwdep.BackgroundScanning(), hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraSoftwareDeps: []string{"gaia"},
			},

			// Subtests for unstable devices, so they can be omitted from reporting.
			{
				Name:    "no_notification_shown_unstable",
				Fixture: "nearbyShareDataUsageOfflineNoOneBackgroundScanningEnabled",
				Val: nearbycommon.TestData{
					Filename:        "small_jpg.zip",
					TransferTimeout: nearbycommon.SmallFileTransferTimeout,
					TestTimeout:     nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
					MimeType:        nearbycommon.MimeTypePng,
				},
				ExtraData:         []string{"small_jpg.zip"},
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				ExtraHardwareDeps: hwdep.D(hwdep.BackgroundScanning(), hwdep.Model(crossdevice.UnstableModels...)),
			},
		},
	})
}

// PhoneToCrosBackgroundScanningDisabled tests that the background scanning notification does not appear if background scanning is toggled off with an Android device as sender and CrOS device as receiver.
func PhoneToCrosBackgroundScanningDisabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*nearbyfixture.FixtData).Chrome
	tconn := s.FixtValue().(*nearbyfixture.FixtData).TestConn
	crosDisplayName := s.FixtValue().(*nearbyfixture.FixtData).CrOSDeviceName
	androidDevice := s.FixtValue().(*nearbyfixture.FixtData).AndroidDevice
	androidDisplayName := s.FixtValue().(*nearbyfixture.FixtData).AndroidDeviceName

	if err := nearbyshare.ToggleNearbyDeviceIsSharingNotification(ctx, tconn, cr /*setChecked=*/, false); err != nil {
		s.Fatal("Failed to toggle background scanning notification: ", err)
	}

	// Extract the test file to the staging directory on the Android device.
	testData := s.Param().(nearbycommon.TestData)
	testDataZip := s.DataPath(testData.Filename)
	testFile, err := nearbytestutils.ExtractAndroidTestFile(ctx, testDataZip, androidDevice)
	if err != nil {
		s.Fatal("Failed to extract test data files: ", err)
	}

	s.Log("Starting sending on the Android device")
	testTimeout := testData.TestTimeout
	mimetype := testData.MimeType
	if err := androidDevice.SendFile(ctx, androidDisplayName, crosDisplayName, testFile, mimetype, testTimeout); err != nil {
		s.Fatal("Failed to start sending on Android: ", err)
	}
	defer androidDevice.AwaitSharingStopped(ctx, 10*time.Second)
	defer androidDevice.CancelSendingFile(ctx)

	s.Logf("Waiting for %v seconds to ensure no background scanning notification is displayed", nearbycommon.DetectShareTargetTimeout.Seconds())
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	start := time.Now()
	if err := testing.Poll(ctx,
		func(ctx context.Context) error {
			if exists, err := nearbyshare.FastInitiationNotificationExists(ctx, tconn); err != nil {
				return testing.PollBreak(err)
			} else if exists {
				return testing.PollBreak(errors.New("background scanning notification found unexpectedly"))
			}
			if time.Since(start) >= nearbycommon.DetectShareTargetTimeout {
				// Timeout is reached and element was not found.
				return nil
			}
			return errors.Errorf("still waiting for the node for %.1fs", (nearbycommon.DetectShareTargetTimeout - time.Since(start)).Seconds())
		},
		nil,
	); err != nil {
		s.Fatal("Failed to ensure that no background scanning notification is displayed: ", err)
	}
}
