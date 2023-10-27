// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package phonehub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"

	cdcommon "go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/phonehub"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const statusAreaOverflowButtonTrayClassName = "StatusAreaOverflowButtonTray"

func init() {
	testing.AddTest(&testing.Test{
		Func:         DownloadRecentPhoto,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Exercises toggling the Recent Photos feature and downloading a photo from a connected phone",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"jasonsun@chromium.org",
		},
		BugComponent: "b:1131837",
		Attr:         []string{"group:cross-device", "cross-device_phonehub"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Fixture:           "crossdeviceOnboardedAllFeaturesRerun",
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(cdcommon.UnstableModels...)),
			},
			{
				Name:              "unstable",
				Fixture:           "crossdeviceOnboardedAllFeaturesRerun",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cdcommon.UnstableModels...)),
			},
			{
				Name:      "floss",
				Fixture:   "crossdeviceOnboardedAllFeaturesFlossRerun",
				ExtraAttr: []string{"cross-device_floss"},
			},
		},
	})
}

// DownloadRecentPhoto exercises toggling the Recent Photos feature and downloading a photo from a connected phone.
func DownloadRecentPhoto(ctx context.Context, s *testing.State) {
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice
	chrome := s.FixtValue().(*crossdevice.FixtData).Chrome
	tconn := s.FixtValue().(*crossdevice.FixtData).TestConn
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)
	ui := uiauto.New(tconn)

	photoName, err := androidDevice.TakePhoto(ctx)
	if err != nil {
		s.Fatal("Failed to take a photo on the Android phone: ", err)
	}
	androidFilePath := filepath.Join(crossdevice.AndroidPhotosPath, photoName)
	defer androidDevice.RemoveMediaFile(ctx, androidFilePath)

	// Re-open Phone Hub to expose controls to download recent photos.
	if err := phonehub.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to re-open Phone Hub to download Recent Photos: ", err)
	}

	// Download the newly taken photo to Tote.
	if err := phonehub.DownloadMostRecentPhoto(ctx, tconn); err != nil {
		s.Fatal("Failed to download the most recent photo: ", err)
	}

	// Locate and click on the holding space tray.
	if err := ui.LeftClick(holdingspace.FindTray())(ctx); err != nil {
		// In tablet mode, the holding space may not be visible until the status tray is expanded.
		if err := ui.LeftClick(nodewith.ClassName(statusAreaOverflowButtonTrayClassName))(ctx); err != nil {
			s.Fatal("Failed to locate and click the holding space and status overflow tray: ", err)
		}

		// Look for the holding space tray again.
		if err := ui.LeftClick(holdingspace.FindTray())(ctx); err != nil {
			s.Fatal("Failed to locate and click the holding space tray after expanding the status tray: ", err)
		}
	}

	// Locate recently downloaded photo in the holding space tray.
	if err := ui.Exists(holdingspace.FindDownloadChip().Name(photoName).First())(ctx); err != nil {
		s.Fatal("Expected photo ", photoName, " is not displayed in the holding space tray: ", err)
	}

	// Verify the downloaded file content.
	sourceFileSizeBytes, err := androidDevice.FileSize(ctx, androidFilePath)
	if err != nil {
		s.Fatal("Failed to read source photo size: ", err)
	}
	downloadsPath, err := cryptohome.DownloadsPath(ctx, chrome.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	crosPhotoFilePath := filepath.Join(downloadsPath, photoName)
	if err := waitUntilDownloadComplete(ctx, crosPhotoFilePath, sourceFileSizeBytes); err != nil {
		s.Fatal("Photo download cannot be completed: ", err)
	}
	if err := comparePhotoHashes(ctx, crosPhotoFilePath, androidDevice); err != nil {
		s.Fatal("Failed to verify hash of the downloaded photo: ", err)
	}

	// Hide Phone Hub and disable Recent Photos from the Settings page.
	if err := phonehub.Hide(ctx, tconn); err != nil {
		s.Fatal("Failed to open Phone Hub: ", err)
	}
}

// waitUntilDownloadComplete waits for the target photo to be fully downloaded to the CrOS device's download directory.
func waitUntilDownloadComplete(ctx context.Context, crosPhotoFilePath string, sourceFileSizeBytes int64) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		fi, err := os.Stat(crosPhotoFilePath)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get the size of the downloaded photo on CrOS"))
		}
		if currentSizeBytes := fi.Size(); currentSizeBytes != sourceFileSizeBytes {
			return errors.Errorf("Photo download not complete yet: expected %d bytes, downloaded %d bytes", sourceFileSizeBytes, currentSizeBytes)
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return errors.Wrap(err, "failed to wait for photo download to complete")
	}
	return nil
}

// comparePhotoHashes verifies that the hash of the downloaded photo matches the hash of the source photo on the Android device.
func comparePhotoHashes(ctx context.Context, crosPhotoFilePath string, androidDevice *crossdevice.AndroidDevice) error {
	photoName := filepath.Base(crosPhotoFilePath)
	androidFilePath := filepath.Join(crossdevice.AndroidPhotosPath, photoName)
	androidHash, err := androidDevice.SHA256Sum(ctx, androidFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to compute hash of the source photo on the Android device")
	}

	crosHash, err := hashFile(ctx, crosPhotoFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to compute hash of the downloaded photo on the CrOS device")
	}

	if androidHash != crosHash {
		return errors.Errorf("Hash mismatch for downloaded photo %s: expected=%s, actual=%s", photoName, androidHash, crosHash)
	}

	return nil
}

// hashFile computes the SHA256 checksum of the given file.
func hashFile(ctx context.Context, filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open file %s", filePath)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errors.Wrapf(err, "failed to copy %s file contents to the hasher", filePath)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
