// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

var defaultBackupDirs = []string{
	"/home/.shadow/",
	"/home/chronos/",
	"/var/lib/device_management/",
	"/var/lib/devicesettings/",
	"/var/lib/tpm_manager/",
	"/var/lib/bootlockbox/",
	"/var/lib/cryptohome/",
	"/var/lib/chaps/",
	"/var/lib/oobe_config_restore/",
	"/var/lib/u2f/",
	"/var/lib/vtpm/",
	"/mnt/stateful_partition/unencrypted/tpm_manager/",
	"/mnt/stateful_partition/unencrypted/preserve/",
}

var defaultBackupFiles = []string{
	"/var/lib/public_mount_salt",
	"/var/lib/system_salt",
}

// Skip packing the "mount" directories, since the file systems it's
// used for don't allow taking snapshots. E.g., ext4 fscrypt complains
// "Required key not available" when trying to read encrypted files.
var defaultIgnorePaths = []string{
	"/home/.shadow/*/mount",
}

const defaultPinWeaverDataDir = "/home/.shadow/low_entropy_creds"
const defaultTpm2SimulatorNVChipFile = "/mnt/stateful_partition/unencrypted/tpm2-simulator/NVChip"

func (h *CmdHelper) pathExistsInTar(ctx context.Context, tar, dataPath string) bool {
	// We want to use relative path here.
	dataPath = strings.TrimPrefix(dataPath, "/")
	_, err := h.cmdRunner.Run(ctx, "/bin/tar", "--list", "--file", tar, dataPath)
	return err == nil
}

func (h *CmdHelper) cleanupExistingDir(ctx context.Context, tar, path string) error {
	if h.pathExistsInTar(ctx, tar, path) {
		// Clean dir content. (note that deleting the directory itself may fail).
		visiblePath := filepath.Join(path, "*")
		if err := h.RemoveAll(ctx, visiblePath); err != nil {
			return errors.Wrapf(err, "failed to remove old %v data", visiblePath)
		}
		hiddenPath := filepath.Join(path, ".*")
		if err := h.RemoveAll(ctx, hiddenPath); err != nil {
			return errors.Wrapf(err, "failed to remove old %v data", hiddenPath)
		}
	}
	return nil
}

func (h *CmdHelper) runCmdOrFailWithOut(ctx context.Context, cmd string, args ...string) error {
	out, err := h.cmdRunner.RunWithCombinedOutput(ctx, cmd, args...)
	if err != nil {
		// Return programs's output on failures. Avoid line breaks in error
		// messages, to keep the Tast logs readable.
		outFlat := strings.Replace(string(out), "\n", " ", -1)
		return errors.Wrap(err, outFlat)
	}
	return nil
}

func (h *CmdHelper) decompressData(ctx context.Context, src string) error {
	// Use the "tar" program as it takes care of recursive unpacking,
	// preserving ownership, permissions and SELinux attributes.
	return h.runCmdOrFailWithOut(ctx, "/bin/tar",
		"--extract",              // extract files from an archive
		"--gzip",                 // filter the archive through gunzip
		"--preserve-permissions", // extract file permissions
		"--same-owner",           // extract file ownership
		"--directory=/",          // unpacks files to the root directory
		"--file",                 // read from the file specified in the next argument
		src)
}

func (h *CmdHelper) compressData(ctx context.Context, dst string, paths, ignorePaths []string) error {
	// Use the "tar" program as it takes care of recursive packing,
	// preserving ownership, permissions and SELinux attributes.
	args := append([]string{
		"--acls",               // save the ACLs to the archive
		"--create",             // create a new archive
		"--gzip",               // filter the archive through gzip
		"--selinux",            // save the SELinux context to the archive
		"--xattrs",             // save the user/root xattrs to the archive
		"--ignore-failed-read", // Ignore the read failure
		"--file",               // write to the file specified in the next argument
		dst})
	for _, p := range ignorePaths {
		// Exclude the specified patterns from archiving.
		args = append(args, "--exclude", p)
	}
	// Specify the input paths to archive.
	args = append(args, paths...)
	return h.runCmdOrFailWithOut(ctx, "/bin/tar", args...)
}

// SaveLoginData creates the compressed file of login data:
//
//   - /home/.shadow
//   - /home/chronos
//   - /mnt/stateful_partition/unencrypted/tpm2-simulator (if includeTpm is set to true).
//   - /var/lib/device_management (for install-time attributes, starting from R119)
func (h *CmdHelper) SaveLoginData(ctx context.Context, archivePath string, includeTpm bool) error {
	if err := h.stopDaemons(ctx, includeTpm); err != nil {
		return err
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()
	defer h.ensureDaemons(cleanupCtx, includeTpm)

	var paths = []string{}
	paths = append(paths, defaultBackupDirs...)
	paths = append(paths, defaultBackupFiles...)
	if includeTpm {
		// There are other NVChip* snapshot files for CrossVersionPinWeaver test's need, so backup the entire tpm2-simulator/ directory.
		paths = append(paths, "/mnt/stateful_partition/unencrypted/tpm2-simulator/")
	}

	if err := h.compressData(ctx, archivePath, paths, defaultIgnorePaths); err != nil {
		return errors.Wrap(err, "failed to compress the cryptohome data")
	}
	return nil
}

// LoadLoginData loads the login data from compressed file.
func (h *CmdHelper) LoadLoginData(ctx context.Context, archivePath string, includeTpm, resumeDaemons bool) error {
	if err := h.stopDaemons(ctx, includeTpm); err != nil {
		return err
	}
	var cancel context.CancelFunc
	if resumeDaemons {
		cleanupCtx := ctx
		ctx, cancel = ctxutil.Shorten(ctx, 20*time.Second)
		defer cancel()
		defer h.ensureDaemons(cleanupCtx, includeTpm)
	}

	for _, path := range defaultBackupDirs {
		if err := h.cleanupExistingDir(ctx, archivePath, path); err != nil {
			return errors.Wrapf(err, "failed to cleanup %v data", path)
		}
	}

	for _, path := range defaultBackupFiles {
		if err := h.RemoveAll(ctx, path); err != nil {
			return errors.Wrapf(err, "failed to remove %v", path)
		}
	}

	if err := h.decompressData(ctx, archivePath); err != nil {
		return errors.Wrap(err, "failed to decompress the cryptohome data")
	}

	// Run `restorecon` to make sure SELinux attributes are correct after the decompression.
	if _, err := h.cmdRunner.Run(ctx, "restorecon", "-r", "/home/.shadow"); err != nil {
		return errors.Wrap(err, "failed to restore selinux attributes")
	}
	return nil
}

func (h *CmdHelper) stopDaemons(ctx context.Context, includeTpm bool) error {
	if err := h.daemonController.TryStop(ctx, UIDaemon); err != nil {
		return errors.Wrap(err, "failed to try to stop UI")
	}
	if err := h.daemonController.TryStopDaemons(ctx, HighLevelTPMDaemons); err != nil {
		return errors.Wrap(err, "failed to try to stop high-level TPM daemons")
	}
	if err := h.daemonController.TryStopDaemons(ctx, LowLevelTPMDaemons); err != nil {
		return errors.Wrap(err, "failed to try to stop low-level TPM daemons")
	}
	if !includeTpm {
		return nil
	}
	if err := h.daemonController.TryStop(ctx, TPM2SimulatorDaemon); err != nil {
		return errors.Wrap(err, "failed to try to stop tpm2-simulator")
	}
	return nil
}

func (h *CmdHelper) ensureDaemons(ctx context.Context, includeTpm bool) {
	if includeTpm {
		if err := h.daemonController.Ensure(ctx, TPM2SimulatorDaemon); err != nil {
			testing.ContextLog(ctx, "Failed to ensure tpm2-simulator: ", err)
		}
	}
	if err := h.daemonController.EnsureDaemons(ctx, LowLevelTPMDaemons); err != nil {
		testing.ContextLog(ctx, "Failed to ensure low-level TPM daemons: ", err)
	}
	if err := h.daemonController.EnsureDaemons(ctx, HighLevelTPMDaemons); err != nil {
		testing.ContextLog(ctx, "Failed to ensure high-level TPM daemons: ", err)
	}
	if err := h.daemonController.Ensure(ctx, UIDaemon); err != nil {
		testing.ContextLog(ctx, "Failed to ensure UI: ", err)
	}
}

// CapturePinWeaverAndTpmSnapShot takes a snapshot of the on-disk PinWeaver hash tree
// (/home/.shadow/low_entropy_creds folder) and the Tpm NVChip data
// (/mnt/stateful_partition/unencrypted/tpm2-simulator/NVChip), which includes PinWeaver's server side root hashlog entries
func (h *CmdHelper) CapturePinWeaverAndTpmSnapShot(ctx context.Context, index int) error {
	if err := h.stopDaemons(ctx, true /* includeTpm */); err != nil {
		return err
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()
	defer h.ensureDaemons(cleanupCtx, true /* includeTpm */)

	srcPWHashTreeDir := defaultPinWeaverDataDir
	dstPWHashTreeDir := srcPWHashTreeDir + "." + strconv.Itoa(index)
	if err := os.RemoveAll(dstPWHashTreeDir); err != nil {
		errors.Wrap(err, "failed to remove existing /home/.shadow/low_entropy_creds directory")
	}
	if err := fsutil.CopyDir(srcPWHashTreeDir, dstPWHashTreeDir); err != nil {
		errors.Wrap(err, "failed to take snapshot of the PinWeaver on-disk hash tree")
	}

	srcTpmNVChipFile := defaultTpm2SimulatorNVChipFile
	dstTpmNVChipFile := srcTpmNVChipFile + "." + strconv.Itoa(index)
	if err := fsutil.CopyFile(srcTpmNVChipFile, dstTpmNVChipFile); err != nil {
		errors.Wrap(err, "failed to take snapshot of the tpm2-simulator TPM state")
	}

	return nil
}

// RestorePinWeaverHashTreeSnapShot restore the on-disk PinWeaver hash tree data
// (/home/.shadow/low_entropy_creds folder) from a previous snapshot.
func (h *CmdHelper) RestorePinWeaverHashTreeSnapShot(ctx context.Context, index int) error {
	dstPWHashTreeDir := defaultPinWeaverDataDir
	srcPWHashTreeDir := dstPWHashTreeDir + "." + strconv.Itoa(index)
	if err := os.RemoveAll(dstPWHashTreeDir); err != nil {
		errors.Wrap(err, "failed to remove existing /home/.shadow/low_entropy_creds directory")
	}
	if err := fsutil.CopyDir(srcPWHashTreeDir, dstPWHashTreeDir); err != nil {
		errors.Wrap(err, "failed to restore the PinWeaver on-disk hash tree from snapshot")
	}
	return nil
}

// RestoreTpmNVChipSnapShot restore the Tpm NVChip data
// (/mnt/stateful_partition/unencrypted/tpm2-simulator/NVChip) from a previous snapshot.
func (h *CmdHelper) RestoreTpmNVChipSnapShot(ctx context.Context, index int) error {
	if err := h.stopDaemons(ctx, true /* includeTpm */); err != nil {
		return err
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()
	defer h.ensureDaemons(cleanupCtx, true /* includeTpm */)

	dstTpmNVChipFile := defaultTpm2SimulatorNVChipFile
	srcTpmNVChipFile := dstTpmNVChipFile + "." + strconv.Itoa(index)
	if err := fsutil.CopyFile(srcTpmNVChipFile, dstTpmNVChipFile); err != nil {
		errors.Wrap(err, "failed to restore the tpm2-simulator TPM state from snapshot")
	}
	return nil
}
