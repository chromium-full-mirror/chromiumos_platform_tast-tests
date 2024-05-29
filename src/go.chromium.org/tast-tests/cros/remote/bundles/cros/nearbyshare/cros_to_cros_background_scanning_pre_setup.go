// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"path/filepath"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	remotenearby "go.chromium.org/tast-tests/cros/remote/cros/nearbyshare"
	"go.chromium.org/tast-tests/cros/services/cros/nearbyservice"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrosToCrosBackgroundScanningPreSetup,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Nearby Device is trying to share notification shows up, clicking the notification initiates onboarding flow",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"hansenmichael@google.com",
		},
		BugComponent: "b:1131838",
		Attr:         []string{"group:cross-device-remote", "cross-device-remote_nearbyshare"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.nearbyservice.NearbyShareService"},
		Vars:         []string{"secondaryTarget"},
		Params: []testing.Param{
			// Stable subset of models.
			{
				Name:              "onboarding_flow_initiated",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanningPreSetup",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				// TODO(b/225966067): Replace with companion DUT HWDep for background scanning.
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.BGScanningStableSkipModels...)),
				},
				ExtraAttr: []string{"cross-device-remote_cq"},
			},

			// Unstable subset of models (sender).
			{
				Name:              "onboarding_flow_initiated_unstable_sender",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanningPreSetup",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				// TODO(b/225966067): Replace with companion DUT HWDep for background scanning.
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.BGScanningStableSkipModels...)),
				},
			},
			// Unstable subset of models (receiver).
			{
				Name:              "onboarding_flow_initiated_unstable_receiver",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanningPreSetup",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				// TODO(b/225966067): Replace with companion DUT HWDep for background scanning.
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.BGScanningUnstableModels...)),
				},
			},
			// Unstable subset of models (both).
			{
				Name:              "onboarding_flow_initiated_unstable_both",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanningPreSetup",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				// TODO(b/225966067): Replace with companion DUT HWDep for background scanning.
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.BGScanningUnstableModels...)),
				},
			},

			// Floss duplicate
			{
				Name:    "onboarding_flow_initiated_floss",
				Fixture: "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanningPreSetupFloss",
				Val:     nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				// TODO(b/225966067): Replace with companion DUT HWDep for background scanning.
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel("babymega", "barla", "blooglet", "dewatt", "dumo", "fennel", "hana", "hayato", "kevin", "krane", "sentry", "soraka", "tomato", "treeya", "treeya360")),
				},
				ExtraAttr: []string{"cross-device-remote_floss"},
			},
		},
	})
}

// CrosToCrosBackgroundScanningPreSetup tests that background scanning initiates onboarding.
func CrosToCrosBackgroundScanningPreSetup(ctx context.Context, s *testing.State) {
	remoteFilePath := s.FixtValue().(*remotenearby.FixtData).RemoteFilePath
	sender := s.FixtValue().(*remotenearby.FixtData).Sender
	receiver := s.FixtValue().(*remotenearby.FixtData).Receiver

	s.Log("Starting sending on DUT1 (Sender)")
	testData := s.Param().(nearbycommon.TestData)
	remoteFile := filepath.Join(remoteFilePath, testData.Filename)
	fileReq := &nearbyservice.CrOSPrepareFileRequest{FileName: remoteFile}
	fileNames, err := sender.PrepareFiles(ctx, fileReq)
	if err != nil {
		s.Fatal("Failed to prepare files for sending on DUT1 (Sender): ", err)
	}
	sendReq := &nearbyservice.CrOSSendFilesRequest{FileNames: fileNames.FileNames}
	_, err = sender.StartSend(ctx, sendReq)
	if err != nil {
		s.Fatal("Failed to start send on DUT1 (Sender): ", err)
	}

	s.Log("Accepting the fast initiation notification on DUT2 (Receiver)")
	acceptFastInitNotificationReq := &nearbyservice.CrOSAcceptFastInitiationNotificationRequest{IsSetupComplete: false}
	_, err = receiver.AcceptFastInitiationNotification(ctx, acceptFastInitNotificationReq)
	if err != nil {
		s.Fatal("Failed to accept fast initiation notification on DUT2 (Receiver): ", err)
	}

	s.Log("Waiting for onboarding to open")
	_, err = receiver.WaitForOnboardingFlow(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to wait for onboarding to open: ", err)
	}
}
