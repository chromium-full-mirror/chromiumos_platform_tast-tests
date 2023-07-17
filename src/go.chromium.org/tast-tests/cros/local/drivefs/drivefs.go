// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package drivefs

import (
	"context"
	"os"
	"path"
	"strings"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/filesystem"
	"go.chromium.org/tast/core/errors"
)

const (
	driveFsCommandLineArgsFileName = "command_line_args"

	driveFsXattrAvailableOffline = "user.drive.available_offline"
	driveFsXattrID               = "user.drive.id"
	driveFsXattrPinned           = "user.drive.pinned"
	driveFsXattrUncommitted      = "user.drive.uncommitted"
)

// DriveFs is a helper object for working with `drivefs` instances run within
// the Files App.
type DriveFs struct {
	user             string
	mountPath        string
	homeDir          string
	persistableToken string
	cliArgs          string
}

// NewDriveFs waits for `drivefs` to mount and then creates a new `DriveFs`
// instance to work with it.
func NewDriveFs(ctx context.Context, user string) (*DriveFs, error) {
	mountPath, err := WaitForDriveFs(ctx, user)
	if err != nil {
		return nil, err
	}
	homeDir, err := cryptohome.UserPath(ctx, user)
	if err != nil {
		return nil, err
	}
	persistableToken := PersistableToken(mountPath)
	if len(persistableToken) == 0 {
		return nil, errors.New("Failed to obtain persistable token from mount: " + mountPath)
	}
	return &DriveFs{
		user:             user,
		mountPath:        mountPath,
		homeDir:          homeDir,
		persistableToken: persistableToken,
	}, nil
}

// ConfigPath returns a path to `elem...` within the config directory for
// `drivefs`.
func (dfs *DriveFs) ConfigPath(elem ...string) string {
	return ConfigPath(dfs.homeDir, dfs.persistableToken, elem...)
}

// MyDrivePath returns a path to `elem...` within the "My Drive" aka "root"
// directory within `drivefs`.
func (dfs *DriveFs) MyDrivePath(elem ...string) string {
	return path.Join(append([]string{dfs.mountPath, "root"}, elem...)...)
}

// MountPath returns a path to `elem...` within the virtual file system provided
// by `drivefs`.
func (dfs *DriveFs) MountPath(elem ...string) string {
	return path.Join(append([]string{dfs.mountPath}, elem...)...)
}

// WriteCommandLineFlags writes a new set of command line flags for `drivefs`
// to use.
//
// Note: `Restart` should be called so `drivefs` picks up the new flags.
func (dfs *DriveFs) WriteCommandLineFlags(flags string) error {
	dfs.cliArgs = flags
	return os.WriteFile(dfs.ConfigPath(driveFsCommandLineArgsFileName), []byte(flags), 0644)
}

// ClearCommandLineFlags clears any previously set command line flags for
// `drivefs.
//
// Note: `Restart` should be called so `drivefs` can be restarted without flags.
func (dfs *DriveFs) ClearCommandLineFlags() error {
	dfs.cliArgs = ""
	return os.Remove(dfs.ConfigPath(driveFsCommandLineArgsFileName))
}

// Restart terminates the running instance of `drivefs` and waits for it to
// remount.
func (dfs *DriveFs) Restart(ctx context.Context) error {
	// Kill DriveFS, cros-disks will ensure another starts up.
	if err := testexec.CommandContext(ctx, "pkill", "-HUP", "drivefs").Run(); err != nil {
		// pkill exits with code 1 if it could find no matching process (see: man 1 pkill).
		// This is OK, as cros-disks will start one shortly.
		if ws, ok := testexec.GetWaitStatus(err); !ok || !ws.Exited() || ws.ExitStatus() != 1 {
			return errors.Wrap(err, "failed to kill drivefs processes")
		}
	}
	_, err := WaitForDriveFs(ctx, dfs.user)
	return err
}

// ClearLocalData mimics the user pressing the Clear local data button on
// chrome://drive-internals and ensures the feature flags are restored.
func (dfs *DriveFs) ClearLocalData(ctx context.Context, cr *chrome.Chrome) error {
	cachedCliArgs := dfs.cliArgs
	pageConn, err := cr.NewConn(ctx, "chrome://drive-internals")
	if err != nil {
		return errors.Wrap(err, "failed to open chrome://drive-internals")
	}
	defer pageConn.Close()

	if err := pageConn.Eval(ctx, "chrome.send('resetDriveFileSystem');", nil); err != nil {
		return errors.Wrap(err, "failed to clear local data")
	}
	if err := pageConn.WaitForExpr(ctx, "$('reset-status-text').textContent === 'success'"); err != nil {
		return errors.Wrap(err, "failed to wait for local data to clear successfully")
	}
	if _, err = WaitForDriveFs(ctx, dfs.user); err != nil {
		return errors.Wrap(err, "failed to wait for drivefs to startup again")
	}
	// Only restore command line arguments if they exist.
	if len(cachedCliArgs) > 0 {
		return dfs.WriteCommandLineFlags(cachedCliArgs)
	}
	return nil
}

// SaveLogsOnError saves off DriveFS logs on failure. See `SaveDriveLogsOnError`.
func (dfs *DriveFs) SaveLogsOnError(ctx context.Context, hasError func() bool) {
	if !hasError() {
		return
	}
	saveDriveLogs(ctx, dfs.homeDir, dfs.persistableToken)
}

func (dfs *DriveFs) ensureDriveFsPath(path string) error {
	if strings.HasPrefix(path, dfs.mountPath) {
		return nil
	}
	return errors.New("path is not in drivefs: " + path)
}

// File wraps an `os.File` with additional helper functions.
type File struct {
	*os.File
	name string
}

// NewFile constructs a new, unopened `drivefs.File`.
func (dfs *DriveFs) NewFile(name string) (*File, error) {
	if err := dfs.ensureDriveFsPath(name); err != nil {
		return nil, err
	}
	return &File{
		File: nil,
		name: name,
	}, nil
}

// Open opens a `drivefs.File`, see `os.Open`.
func (file *File) Open() (err error) {
	if file.File != nil {
		return errors.New("file already opened")
	}
	file.File, err = os.Open(file.Name())
	return err
}

// Create creates a new `drivefs.File`, see `os.Create`.
func (file *File) Create() (err error) {
	if file.File != nil {
		return errors.New("file already opened")
	}
	file.File, err = os.Create(file.Name())
	return err
}

// Close closes a `drivefs.File` if it has been opened, see `os.Close`.
func (file *File) Close() error {
	if file.File == nil {
		return nil
	}
	if err := file.File.Close(); err != nil {
		return err
	}
	file.File = nil
	return nil
}

// Name returns the name of the `drivefs.File`.
//
// This overrides the Name() from `os.File`.
func (file *File) Name() string {
	return file.name
}

// IsPinned returns `true` if the file is pinned in DriveFS.
//
// Note: This doesn't indicate if the file data has been downloaded, just if it
// _should_ be downloaded.
func (file *File) IsPinned() (bool, error) {
	pinned := false
	err := filesystem.GetXattr(file.Name(), driveFsXattrPinned, &pinned)
	return pinned, err
}

// SetPinned pins or unpins a file in DriveFS.
//
// Note: Pinning a file only marks it for download. Unpinning the file will
// free it for eviction, but it won't be evicted until necessary.
func (file *File) SetPinned(pinned bool) error {
	return filesystem.SetXattr(file.Name(), driveFsXattrPinned, pinned)
}

// IsUncommitted returns `true` if the file has uncommitted/unuploaded data.
func (file *File) IsUncommitted() (bool, error) {
	uncommitted := true
	err := filesystem.GetXattr(file.Name(), driveFsXattrUncommitted, &uncommitted)
	return uncommitted, err
}

// IsAvailableOffline returns `true` if the file has all its data available locally.
func (file *File) IsAvailableOffline() (bool, error) {
	availableOffline := false
	err := filesystem.GetXattr(file.Name(), driveFsXattrAvailableOffline, &availableOffline)
	return availableOffline, err
}

// ItemID returns the item ID of the file, if it has been created on the cloud.
//
// Note: Unuploaded files will have a `local-` prefixed ID. This ID will be
// replaced with a cloud ID once uploaded.
func (file *File) ItemID() (string, error) {
	itemID := ""
	err := filesystem.GetXattr(file.Name(), driveFsXattrID, &itemID)
	return itemID, err
}

// CloudIDCreatedAction returns an action that fails until a cloud ID is created.
func (file *File) CloudIDCreatedAction() action.Action {
	return action.Named("await file id creation", func(ctx context.Context) error {
		id, err := file.ItemID()
		if err != nil {
			return err
		}
		if strings.HasPrefix(id, "local-") {
			return errors.New("file has local ID: " + id)
		}
		return nil
	})
}

// UploadedAction returns an action that fails until the file is uploaded.
func (file *File) UploadedAction() action.Action {
	return action.Named("await file upload", func(ctx context.Context) error {
		uncommitted, err := file.IsUncommitted()
		if err != nil {
			return err
		}
		if uncommitted {
			return errors.New("file is uncommitted")
		}
		return nil
	})
}

// ExistsAction returns an action that fails until the file exists locally.
func (file *File) ExistsAction() action.Action {
	return action.Named("await file existence", func(ctx context.Context) error {
		_, err := os.Stat(file.Name())
		return err
	})
}

// PinnedAction returns an action that fails if the `file` does not have the
// `expected` pinned xattr.
func (file *File) PinnedAction(expected bool) action.Action {
	return action.Named("await file to be pinned", func(ctx context.Context) error {
		pinned, err := file.IsPinned()
		if err != nil {
			return err
		}
		if pinned != expected {
			return errors.New("file is not in expected pinned state")
		}
		return nil
	})
}

// AvailableOfflineAction returns an action that fails if the `file` does not
// have the `expected` available_offline xattr.
func (file *File) AvailableOfflineAction(expected bool) action.Action {
	return action.Named("await file to be toggled available offline", func(ctx context.Context) error {
		availableOffline, err := file.IsAvailableOffline()
		if err != nil {
			return err
		}
		if availableOffline != expected {
			return errors.New("file is not in expected available offline state")
		}
		return nil
	})
}
