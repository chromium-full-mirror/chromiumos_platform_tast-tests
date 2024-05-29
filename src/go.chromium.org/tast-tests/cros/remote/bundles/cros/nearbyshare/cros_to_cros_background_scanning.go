// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	remotenearby "go.chromium.org/tast-tests/cros/remote/cros/nearbyshare"
	"go.chromium.org/tast-tests/cros/services/cros/nearbyservice"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrosToCrosBackgroundScanning,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Nearby Device is trying to share notification shows up, clicking the notification enables high-vis mode and the receive flow is successful",
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
			// Stable subset of boards.
			{
				Name:              "dataoffline_hidden_png5kb",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanning",
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

			// Unstable subset of boards (sender).
			{
				Name:              "dataoffline_hidden_png5kb_unstable_sender",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanning",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				// TODO(b/225966067): Replace with companion DUT HWDep for background scanning.
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.BGScanningStableSkipModels...)),
				},
			},
			// Unstable subset of boards (receiver).
			{
				Name:              "dataoffline_hidden_png5kb_unstable_receiver",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanning",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout:           nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				// TODO(b/225966067): Replace with companion DUT HWDep for background scanning.
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.BGScanningUnstableModels...)),
				},
			},
			// Unstable subset of boards (both).
			{
				Name:              "dataoffline_hidden_png5kb_unstable_both",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanning",
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
				Name:    "dataoffline_hidden_png5kb_floss",
				Fixture: "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanningFloss",
				Val:     nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
				// TODO(b/225966067): Replace with companion DUT HWDep for background scanning.
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.BackgroundScanningDisabledModels...)),
				},
				ExtraAttr: []string{"cross-device-remote_floss"},
			},
		},
	})
}

// CrosToCrosBackgroundScanning tests background scanning and file sharing between ChromeOS devices.
func CrosToCrosBackgroundScanning(ctx context.Context, s *testing.State) {
	remoteFilePath := s.FixtValue().(*remotenearby.FixtData).RemoteFilePath
	sender := s.FixtValue().(*remotenearby.FixtData).Sender
	receiver := s.FixtValue().(*remotenearby.FixtData).Receiver
	senderDisplayName := s.FixtValue().(*remotenearby.FixtData).SenderDisplayName
	receiverDisplayName := s.FixtValue().(*remotenearby.FixtData).ReceiverDisplayName

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
	acceptFastInitNotificationReq := &nearbyservice.CrOSAcceptFastInitiationNotificationRequest{IsSetupComplete: true}
	_, err = receiver.AcceptFastInitiationNotification(ctx, acceptFastInitNotificationReq)
	if err != nil {
		s.Fatal("Failed to accept fast initiation notification on DUT2 (Receiver): ", err)
	}

	s.Log("Selecting Receiver's (DUT2) share target on Sender (DUT1)")
	targetReq := &nearbyservice.CrOSSelectShareTargetRequest{ReceiverName: receiverDisplayName, CollectShareToken: true}
	senderShareToken, err := sender.SelectShareTarget(ctx, targetReq)
	if err != nil {
		s.Fatal("Failed to select share target on DUT1 (Sender): ", err)
	}
	s.Log("Accepting the share request on DUT2 (Receiver)")
	transferTimeoutSeconds := int32(testData.TransferTimeout.Seconds())
	receiveReq := &nearbyservice.CrOSReceiveFilesRequest{SenderName: senderDisplayName, TransferTimeoutSeconds: transferTimeoutSeconds}
	receiverShareToken, err := receiver.WaitForSenderAndAcceptShare(ctx, receiveReq)
	if err != nil {
		s.Fatal("Failed to accept share on DUT2 (Receiver): ", err)
	}
	if senderShareToken.ShareToken != receiverShareToken.ShareToken {
		s.Fatalf("Share tokens for sender and receiver do not match. Sender: %s, Receiver: %s", senderShareToken, receiverShareToken)
	}

	// Repeat the file hash check for a few seconds, as we have no indicator on the CrOS side for when the received file has been completely written.
	// TODO(crbug/1173190): Remove polling when we can confirm the transfer status with public test functions.
	s.Log("Comparing file hashes for all transferred files on both DUTs")
	senderSendDir := s.FixtValue().(*remotenearby.FixtData).SenderSendPath
	receiverDownloadsPath := s.FixtValue().(*remotenearby.FixtData).ReceiverDownloadsPath
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		senderFileReq := &nearbyservice.CrOSFileHashRequest{FileNames: fileNames.FileNames, FileDir: senderSendDir}
		senderFileRes, err := sender.FilesHashes(ctx, senderFileReq)
		if err != nil {
			return errors.Wrap(err, "failed to get file hashes on DUT1 (Sender)")
		}
		receiverFileReq := &nearbyservice.CrOSFileHashRequest{FileNames: fileNames.FileNames, FileDir: receiverDownloadsPath}
		receiverFileRes, err := receiver.FilesHashes(ctx, receiverFileReq)
		if err != nil {
			return errors.Wrap(err, "failed to get file hashes on DUT2 (Receiver)")
		}
		if len(senderFileRes.Hashes) != len(receiverFileRes.Hashes) {
			return errors.Wrap(err, "length of hashes don't match")
		}
		for i := range senderFileRes.Hashes {
			if senderFileRes.Hashes[i] != receiverFileRes.Hashes[i] {
				return errors.Wrap(err, "hashes don't match")
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed file hash comparison: ", err)
	}
	s.Log("Share completed and file hashes match on both DUTs")
}
