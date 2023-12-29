// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/usbdevice"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         USBCopy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify it is possible to copy files to and from a mass storage USB device",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"ashpakov@google.com",
		},
		Attr: []string{
			"group:mainline",
			"informational",
		},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{{
			Name: "exfat",
			Val:  "exfat",
		}, {
			Name: "ext4",
			Val:  "ext4",
		}, {
			Name: "vfat",
			Val:  "vfat",
		}, {
			Name: "ntfs",
			Val:  "ntfs",
		}},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-ac496e62-db9a-4ae2-9c4a-d322f6d62b84",
			},
		},
	})
}

func USBCopy(ctx context.Context, s *testing.State) {
	const (
		deviceLabel          = "virtual-usb"
		defaultDir           = "/media/removable/" + deviceLabel
		defaultUsbLabel      = "USB Drive"
		mountDirPrefix       = "mass_storage_"
		pasteToUsbFile       = "paste-to-usb.txt"
		pasteToUsbContents   = "pasteToUsb"
		pasteFromUsbFile     = "paste-from-usb.txt"
		pasteFromUsbContents = "pasteFromUsb"
	)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	fs := s.Param().(string)
	cleanupMs, err := setupMassStorage(ctx, deviceLabel, fs)
	if err != nil {
		s.Fatal("Unable to setup the mass storage device: ", err)
	}
	defer cleanupMs(cleanupCtx)

	if err := waitUntilMounted(ctx, defaultDir); err != nil {
		s.Fatal("Timed out waiting for ChromeOS to mount the mass storage device: ", err)
	}
	if fs == "ext4" {
		// Change the ownership of the mountDir so that test user could modify it.
		if err := os.Chown(defaultDir, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
			s.Fatal("Failed to chown: ", err)
		}
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}

	testFiles := map[string]string{
		filepath.Join(downloadsPath, pasteToUsbFile): pasteToUsbContents,
		filepath.Join(defaultDir, pasteFromUsbFile):  pasteFromUsbContents,
	}
	for path, contents := range testFiles {
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			s.Fatalf("Failed to create file %q: %s", path, err)
		}
		defer os.Remove(path)
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "USBCopy")

	if err := uiauto.Combine("Paste a test file from the downloads directory to the USB",
		files.OpenDownloads(),
		files.CopyFileToClipboard(pasteToUsbFile),
		action.IfFailThen(
			files.WithTimeout(10*time.Second).OpenPath(filesapp.FilesTitlePrefix+deviceLabel, deviceLabel),
			files.WithTimeout(10*time.Second).OpenPath(filesapp.FilesTitlePrefix+defaultUsbLabel, defaultUsbLabel), // Sometimes device label is not recognised and the default one is used.
		),
		files.PasteFileFromClipboard(kb),
		files.WithTimeout(10*time.Second).WaitForFile(pasteToUsbFile),
	)(ctx); err != nil {
		s.Fatalf("Failed to copy %q to the usb: %v", pasteToUsbFile, err)
	}
	defer os.Remove(filepath.Join(defaultDir, pasteToUsbFile))

	if err := uiauto.Combine("Paste a test file from the USB to the downloads directory",
		files.CopyFileToClipboard(pasteFromUsbFile),
		files.OpenDownloads(),
		files.PasteFileFromClipboard(kb),
		files.WithTimeout(10*time.Second).WaitForFile(pasteFromUsbFile),
	)(ctx); err != nil {
		s.Fatalf("Failed to copy %q to the downloads: %v", pasteFromUsbFile, err)
	}
	defer os.Remove(filepath.Join(downloadsPath, pasteFromUsbFile))

	testFiles[filepath.Join(defaultDir, pasteToUsbFile)] = pasteToUsbContents
	testFiles[filepath.Join(downloadsPath, pasteFromUsbFile)] = pasteFromUsbContents
	for path, original := range testFiles {
		result, err := os.ReadFile(path)
		if err != nil {
			s.Errorf("Failed to read %s: %v", path, err)
		}
		if string(result) != original {
			s.Errorf("Unexpected file contents at %q: got %q; want %q", path, result, original)
		}
	}
}

func setupMassStorage(ctx context.Context, label, fs string) (action.Action, error) {
	ms := usbdevice.NewUSBMassStorage()
	sizeInMb := uint64(10)
	if err := ms.Init(ctx, sizeInMb); err != nil {
		return nil, errors.Wrap(err, "failed to initialise the mass storage device")
	}
	if err := ms.PlugIn(ctx, false); err != nil {
		return nil, errors.Wrap(err, "failed to plug in the mass storage device")
	}
	if fs == "ext4" {
		if err := ms.FormatFileSystem(ctx, "mkfs.ext4", "-L", label); err != nil {
			return nil, errors.Wrap(err, "failed to format the mass storage device in ext4")
		}
	} else if fs == "exfat" {
		if err := ms.FormatFileSystem(ctx, "mkfs.exfat", "-n", label); err != nil {
			return nil, errors.Wrap(err, "failed to format the mass storage device in exfat")
		}
	} else if fs == "ntfs" {
		if err := ms.FormatFileSystem(ctx, "mkfs.ntfs", "--fast", "-L", label); err != nil {
			return nil, errors.Wrap(err, "failed to format the mass storage device in ntfs")
		}
	} else if fs == "vfat" {
		if err := ms.FormatFileSystem(ctx, "mkfs.vfat", "-n", label); err != nil {
			return nil, errors.Wrap(err, "failed to format the mass storage device in vfat")
		}
	} else {
		return nil, errors.Errorf("unexpected file system %s", fs)
	}
	return ms.CleanUp, nil
}

func waitUntilMounted(ctx context.Context, mountDir string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, stderr, err := testexec.CommandContext(ctx, "mountpoint", mountDir).SeparatedOutput(); err != nil {
			return errors.Wrapf(err, "failed to find a mountpoint: %q", string(stderr))
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 3 * time.Second}); err != nil {
		return errors.Wrapf(err, "timed out waiting for a mountpoint %q", mountDir)
	}
	return nil
}
