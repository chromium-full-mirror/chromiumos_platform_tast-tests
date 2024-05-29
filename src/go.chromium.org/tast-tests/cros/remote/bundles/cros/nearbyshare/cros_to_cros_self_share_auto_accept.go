// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	remotenearby "go.chromium.org/tast-tests/cros/remote/cros/nearbyshare"
	"go.chromium.org/tast-tests/cros/services/cros/nearbyservice"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrosToCrosSelfShareAutoAccept,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks we can successfully send files from one Cros device to another on the same Gaia without accepting",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1131838",
		Attr:         []string{"group:cross-device-remote", "cross-device-remote_nearbyshare"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.nearbyservice.NearbyShareService"},
		Params: []testing.Param{
			// Stable subset of boards.
			{
				Name:              "dataoffline_allcontacts_png5kb",
				Fixture:           "nearbyShareRemoteDataUsageOfflineSelfShare",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:    "dataonline_allcontacts_txt30mb",
				Fixture: "nearbyShareRemoteDataUsageOnlineSelfShare",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},

			// Floss duplicates
			{
				Name:              "dataoffline_allcontacts_png5kb_floss",
				Fixture:           "nearbyShareRemoteDataUsageOfflineSelfShareFloss",
				ExtraAttr:         []string{"cross-device-remote_floss"},
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:      "dataonline_allcontacts_txt30mb_floss",
				Fixture:   "nearbyShareRemoteDataUsageOnlineSelfShareFloss",
				ExtraAttr: []string{"cross-device-remote_floss"},
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
		},
	})
}

// CrosToCrosSelfShareAutoAccept tests file sharing between ChromeOS devices where the same user is logged in on both devices.
func CrosToCrosSelfShareAutoAccept(ctx context.Context, s *testing.State) {
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

	s.Log("Selecting Receiver's (DUT2) share target on Sender (DUT1)")
	targetReq := &nearbyservice.CrOSSelectShareTargetRequest{ReceiverName: receiverDisplayName, CollectShareToken: false}
	_, err = sender.SelectShareTarget(ctx, targetReq)
	if err != nil {
		s.Fatal("Failed to select share target on DUT1 (Sender): ", err)
	}

	s.Log("Waiting for transfer complete notification on Receiver (DUT2) (skipping Accept notification with auto-accept)")
	transferTimeoutSeconds := int32(testData.TransferTimeout.Seconds())
	receiveReq := &nearbyservice.CrOSReceiveFilesRequest{SenderName: senderDisplayName, TransferTimeoutSeconds: transferTimeoutSeconds}
	_, err = receiver.WaitForIncomingShareCompletionNotification(ctx, receiveReq)
	if err != nil {
		s.Fatal("Failed to see transfer complete notification on DUT2 (Receiver): ", err)
	}

	s.Log("Comparing file hashes for all transferred files on both DUTs")
	senderSendDir := s.FixtValue().(*remotenearby.FixtData).SenderSendPath
	senderFileReq := &nearbyservice.CrOSFileHashRequest{FileNames: fileNames.FileNames, FileDir: senderSendDir}
	senderFileRes, err := sender.FilesHashes(ctx, senderFileReq)
	if err != nil {
		s.Fatal("Failed to get file hashes on DUT1 (Sender): ", err)
	}
	receiverDownloadsPath := s.FixtValue().(*remotenearby.FixtData).ReceiverDownloadsPath
	receiverFileReq := &nearbyservice.CrOSFileHashRequest{FileNames: fileNames.FileNames, FileDir: receiverDownloadsPath}
	receiverFileRes, err := receiver.FilesHashes(ctx, receiverFileReq)
	if err != nil {
		s.Fatal("Failed to get file hashes on DUT2 (Receiver): ", err)
	}
	if len(senderFileRes.Hashes) != len(receiverFileRes.Hashes) {
		s.Fatal("Length of file hashes don't match")
	}
	for i := range senderFileRes.Hashes {
		if senderFileRes.Hashes[i] != receiverFileRes.Hashes[i] {
			s.Fatalf("Hashes don't match. Wanted: %s, Got: %s", senderFileRes.Hashes[i], receiverFileRes.Hashes[i])
		}
	}
	s.Log("Share completed and file hashes match on both DUTs")
}
