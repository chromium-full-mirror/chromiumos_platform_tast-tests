// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fileutils

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// CopyFilesFromHostToHost copies files/directories from srcHost to dstHost as
// defined in the filesSrcToDst map.
//
// This is done by downloading the files from srcHost to a temporary dir on the
// host running Tast, then uploading it from the test drone host to the dstHost.
// The temporary files on the test runner host is removed before returning.
//
// All paths must be absolute paths. Any trailing slashes will be removed.
//
// The temporary directory is located at "<context_out_dir>/tmp_copy_between_hosts_<start_unix_time>".
func CopyFilesFromHostToHost(ctx context.Context, srcHost, dstHost *ssh.Conn, filesSrcToDst map[string]string) error {
	// Remove all trailing slashes and make sure they are absolute paths.
	cleanedPaths := make(map[string]string)
	for srcPath, dstPath := range filesSrcToDst {
		srcPath = strings.TrimSuffix(srcPath, "/")
		dstPath = strings.TrimSuffix(dstPath, "/")
		if !path.IsAbs(srcPath) {
			return errors.Errorf("copy file from host to host: all source paths must be absolute paths, got %q", srcPath)
		}
		if !path.IsAbs(dstPath) {
			return errors.Errorf("copy file from host to host: all destination paths must be absolute paths, got %q", dstPath)
		}
		cleanedPaths[srcPath] = dstPath
	}
	filesSrcToDst = cleanedPaths

	// Prepare temporary dir on test drone host.
	contextOutDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("copy file from host to host: failed to get context OutDir")
	}
	tmpTastRunnerFileDir := path.Join(contextOutDir, fmt.Sprintf("tmp_copy_between_hosts_%d", time.Now().Unix()))
	if err := os.Mkdir(tmpTastRunnerFileDir, os.ModePerm|os.ModeDir); err != nil {
		return errors.Wrapf(err, "copy file from host to host: failed to create temporary dir %q on Tast runner host", tmpTastRunnerFileDir)
	}
	defer func() {
		// Make sure any created temporary files are removed.
		err := os.RemoveAll(tmpTastRunnerFileDir)
		if err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to remove temporary dir on the Tast runner host at %q: %v", tmpTastRunnerFileDir, err)
		}
	}()

	// Copy all the files and/or dirs from src to the tast runner (local system).
	putFilesMap := make(map[string]string)
	for srcFile, dstFile := range filesSrcToDst {
		// Download file to local dir based in tmp dir.
		localFilePath := path.Join(tmpTastRunnerFileDir, srcFile)
		localFileDir := path.Dir(localFilePath)
		if localFileDir != tmpTastRunnerFileDir {
			// Make required subdirectories.
			if err := os.MkdirAll(localFileDir, os.ModePerm|os.ModeDir); err != nil {
				return errors.Wrapf(err, "copy file from host to host: failed to create temporary dir %q on Tast runner host", localFileDir)
			}
		}
		if err := linuxssh.GetFile(ctx, srcHost, srcFile, localFilePath, linuxssh.DereferenceSymlinks); err != nil {
			return errors.Wrapf(err, "copy file from host to host: failed to download file from src host at %q to the test runner host at %q", srcFile, localFilePath)
		}
		// Append slash to directories so that PutFiles copies them correctly.
		fileInfo, err := os.Stat(localFilePath)
		if err != nil {
			return errors.Wrapf(err, "copy file from host to host: failed to stat file %q downloaded from src host at %q", localFilePath, srcFile)
		}
		if fileInfo.IsDir() {
			putFilesMap[localFilePath] += dstFile + "/"
		} else {
			putFilesMap[localFilePath] = dstFile
		}
	}

	// Copy all the files from tast runner (local system) to the dst host.
	if _, err := linuxssh.PutFiles(ctx, dstHost, putFilesMap, linuxssh.DereferenceSymlinks); err != nil {
		return errors.Wrapf(err, "copy file from host to host: failed to upload files from tast runner host at %q to dst host", tmpTastRunnerFileDir)
	}
	return nil
}
