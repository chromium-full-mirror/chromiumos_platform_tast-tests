// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

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
		Func:         CrosToCrosHighVis,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks we can successfully send files from one Cros device to another",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1131838",
		Attr:         []string{"group:cross-device-remote", "cross-device-remote_nearbyshare"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.nearbyservice.NearbyShareService"},
		Vars:         []string{"secondaryTarget"},
		Params: []testing.Param{
			// Stable subset of boards.
			{
				Name:              "dataoffline_allcontacts_png5kb",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOne",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:              "dataoffline_allcontacts_jpg11kb",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOne",
				Val:               nearbycommon.TestData{Filename: "small_jpg.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:      "dataonline_noone_txt30mb",
				Fixture:   "nearbyShareRemoteDataUsageOnlineNoOne",
				ExtraAttr: []string{"cross-device-remote_cq"},
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_webrtc_and_wlan",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWebRTCAndWLAN",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_webrtc",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWebRTCOnly",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_wlan",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWLANOnly",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},

			// Unstable subset of boards (sender).
			{
				Name:              "dataoffline_allcontacts_png5kb_unstable_sender",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOne",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:              "dataoffline_allcontacts_jpg11kb_unstable_sender",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOne",
				Val:               nearbycommon.TestData{Filename: "small_jpg.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_unstable_sender",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOne",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_webrtc_and_wlan_unstable_sender",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWebRTCAndWLAN",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_webrtc_unstable_sender",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWebRTCOnly",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_wlan_unstable_sender",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWLANOnly",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			// Unstable subset of boards (receiver).
			{
				Name:              "dataoffline_allcontacts_png5kb_unstable_receiver",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOne",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:              "dataoffline_allcontacts_jpg11kb_unstable_receiver",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOne",
				Val:               nearbycommon.TestData{Filename: "small_jpg.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_unstable_receiver",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOne",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_webrtc_and_wlan_unstable_receiver",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWebRTCAndWLAN",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_webrtc_unstable_receiver",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWebRTCOnly",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_wlan_unstable_receiver",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWLANOnly",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			// Unstable subset of boards (both).
			{
				Name:              "dataoffline_allcontacts_png5kb_unstable_both",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOne",
				Val:               nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:              "dataoffline_allcontacts_jpg11kb_unstable_both",
				Fixture:           "nearbyShareRemoteDataUsageOfflineNoOne",
				Val:               nearbycommon.TestData{Filename: "small_jpg.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_unstable_both",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOne",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_webrtc_and_wlan_unstable_both",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWebRTCAndWLAN",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_webrtc_unstable_both",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWebRTCOnly",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:    "dataonline_noone_txt30mb_wlan_unstable_both",
				Fixture: "nearbyShareRemoteDataUsageOnlineNoOneWLANOnly",
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				ExtraHardwareDepsForAll: map[string]hwdep.Deps{
					// Companion DUT 1 dependency.
					"cd1": hwdep.D(hwdep.Model(crossdevice.UnstableModels...)),
				},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},

			// Floss duplicates
			{
				Name:      "dataoffline_allcontacts_png5kb_floss",
				Fixture:   "nearbyShareRemoteDataUsageOfflineNoOneFloss",
				ExtraAttr: []string{"cross-device-remote_floss"},
				Val:       nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout:   nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:      "dataoffline_allcontacts_jpg11kb_floss",
				Fixture:   "nearbyShareRemoteDataUsageOfflineNoOneFloss",
				ExtraAttr: []string{"cross-device-remote_floss"},
				Val:       nearbycommon.TestData{Filename: "small_jpg.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout:   nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:      "dataonline_noone_txt30mb_floss",
				Fixture:   "nearbyShareRemoteDataUsageOnlineNoOneFloss",
				ExtraAttr: []string{"cross-device-remote_floss"},
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:      "dataonline_noone_txt30mb_webrtc_and_wlan_floss",
				Fixture:   "nearbyShareRemoteDataUsageOnlineNoOneWebRTCAndWLANFloss",
				ExtraAttr: []string{"cross-device-remote_floss"},
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:      "dataonline_noone_txt30mb_webrtc_floss",
				Fixture:   "nearbyShareRemoteDataUsageOnlineNoOneWebRTCOnlyFloss",
				ExtraAttr: []string{"cross-device-remote_floss"},
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},
			{
				Name:      "dataonline_noone_txt30mb_wlan_floss",
				Fixture:   "nearbyShareRemoteDataUsageOnlineNoOneWLANOnlyFloss",
				ExtraAttr: []string{"cross-device-remote_floss"},
				Val: nearbycommon.TestData{
					Filename: "big_txt.zip", TransferTimeout: nearbycommon.LargeFileOnlineTransferTimeout},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.LargeFileOnlineTransferTimeout,
			},

			// BLE V2 enabled tests
			// TODO(b/333602803): Remove tests after BLE V2 is launched.
			{
				Name:    "ble_v2",
				Fixture: "nearbyShareNoOneBleV2",
				Val:     nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				Timeout: nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
			{
				Name:      "ble_v2_floss",
				Fixture:   "nearbyShareNoOneBleV2Floss",
				Val:       nearbycommon.TestData{Filename: "small_png.zip", TransferTimeout: nearbycommon.SmallFileTransferTimeout},
				ExtraAttr: []string{"cross-device-remote_floss"},
				Timeout:   nearbycommon.DetectionTimeout + nearbycommon.SmallFileTransferTimeout,
			},
		},
	})
}

// CrosToCrosHighVis tests file sharing between ChromeOS devices.
func CrosToCrosHighVis(ctx context.Context, s *testing.State) {
	remoteFilePath := s.FixtValue().(*remotenearby.FixtData).RemoteFilePath
	sender := s.FixtValue().(*remotenearby.FixtData).Sender
	receiver := s.FixtValue().(*remotenearby.FixtData).Receiver
	senderDisplayName := s.FixtValue().(*remotenearby.FixtData).SenderDisplayName
	receiverDisplayName := s.FixtValue().(*remotenearby.FixtData).ReceiverDisplayName

	s.Log("Starting receiving on DUT2 (Receiver)")
	if _, err := receiver.StartReceiving(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start receiving on DUT2 (Receiver): ", err)
	}

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
