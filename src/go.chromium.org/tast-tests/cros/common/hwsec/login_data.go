// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func (h *CmdHelper) pathExists(ctx context.Context, path string) bool {
	_, err := h.cmdRunner.Run(ctx, "stat", path)
	return err == nil
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
		"--acls",    // save the ACLs to the archive
		"--create",  // create a new archive
		"--gzip",    // filter the archive through gzip
		"--selinux", // save the SELinux context to the archive
		"--xattrs",  // save the user/root xattrs to the archive
		"--file",    // write to the file specified in the next argument
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
//   - /mnt/stateful_partition/unencrypted/tpm2-simulator/NVChip (if includeTpm is set to true).
//   - /var/lib/device_management (for install-time attributes, starting from R119)
func (h *CmdHelper) SaveLoginData(ctx context.Context, archivePath string, includeTpm bool) error {
	if err := h.stopDaemons(ctx, includeTpm); err != nil {
		return err
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()
	defer h.ensureDaemons(cleanupCtx, includeTpm)

	paths := []string{
		"/home/.shadow",
		"/home/chronos",
	}
	if includeTpm {
		paths = append(paths, "/mnt/stateful_partition/unencrypted/tpm2-simulator/NVChip")
	}

	// Starting from R119, we started storing install_attributes.pb inside /var/lib/device_management/
	if h.pathExists(ctx, "/var/lib/device_management") {
		paths = append(paths, "/var/lib/device_management")
	}

	// Skip packing the "mount" directories, since the file systems it's
	// used for don't allow taking snapshots. E.g., ext4 fscrypt complains
	// "Required key not available" when trying to read encrypted files.
	ignorePaths := []string{
		"/home/.shadow/*/mount",
	}
	if err := h.compressData(ctx, archivePath, paths, ignorePaths); err != nil {
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

	// Remove the `/home/.shadow` first to prevent any unexpected file remaining.
	if err := h.RemoveAll(ctx, "/home/.shadow"); err != nil {
		return errors.Wrap(err, "failed to remove old /home/.shadow data")
	}
	// Clean up `/home/chronos` as well (note that deleting this directory itself would fail).
	if err := h.RemoveAll(ctx, "/home/chronos/*"); err != nil {
		return errors.Wrap(err, "failed to remove old /home/chronos/* data")
	}
	if err := h.RemoveAll(ctx, "/home/chronos/.*"); err != nil {
		return errors.Wrap(err, "failed to remove old /home/chronos/.* data")
	}
	// Clean up `/var/lib/device_management` if exists
	if h.pathExists(ctx, "/var/lib/device_management") {
		if err := h.RemoveAll(ctx, "/var/lib/device_management/*"); err != nil {
			return errors.Wrap(err, "failed to remove old /var/lib/device_management data")
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
