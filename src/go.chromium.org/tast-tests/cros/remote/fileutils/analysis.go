// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fileutils

import (
	"context"
	"regexp"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
)

// HostFileContentsMatch checks if the file at remoteFilePath on the remote
// host exists and that its contents match using the regex string matchRegex.
//
// Returns true if the file exists and its contents matches. Returns false
// with a nil error if the file does not exist.
func HostFileContentsMatch(ctx context.Context, host *ssh.Conn, remoteFilePath, matchRegex string) (bool, error) {
	// Verify that the file exists.
	fileExists, err := HostFileExists(ctx, host, remoteFilePath)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check for the existence of file %q", remoteFilePath)
	}
	if !fileExists {
		return false, nil
	}

	// Verify that the file contents match.
	matcher, err := regexp.Compile(matchRegex)
	if err != nil {
		return false, errors.Wrapf(err, "failed to compile regex string %q", matchRegex)
	}
	fileContents, err := linuxssh.ReadFile(ctx, host, remoteFilePath)
	return matcher.Match(fileContents), nil
}

// HostTestPath runs "test <testFlag> <remoteFilePath>" on the host and
// returns true if the test passes and false if the test fails.
func HostTestPath(ctx context.Context, host *ssh.Conn, testFlag, remotePath string) (bool, error) {
	if err := host.CommandContext(ctx, "test", testFlag, remotePath).Run(); err != nil {
		if err.Error() == "Process exited with status 1" {
			// Test was successfully evaluated and returned false.
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to run 'test %q %q' on remote host", testFlag, remotePath)
	}
	return true, nil
}

// HostFileExists checks the host for the file at remoteFilePath and returns
// true if remoteFilePath exists and is a regular file.
func HostFileExists(ctx context.Context, host *ssh.Conn, remoteFilePath string) (bool, error) {
	return HostTestPath(ctx, host, "-f", remoteFilePath)
}

// HostDirExists checks the host for the file at remoteFilePath and returns
// true if remoteFilePath exists and is a directory.
func HostDirExists(ctx context.Context, host *ssh.Conn, remoteFilePath string) (bool, error) {
	return HostTestPath(ctx, host, "-d", remoteFilePath)
}

// LocalSha256sum runs the "sha256sum <path>" command on the remote tast runner
// and returns the checksum it outputs.
func LocalSha256sum(ctx context.Context, path string) (string, error) {
	output, err := testexec.CommandContext(ctx, "sha256sum", path).Output()
	if err != nil {
		return "", err
	}
	return strings.Fields(string(output))[0], nil
}

// RemoteSha256sum runs the "sha256sum <path>" command over the provides ssh
// connection and returns the checksum it outputs.
func RemoteSha256sum(ctx context.Context, sshConn *ssh.Conn, path string) (string, error) {
	output, err := sshConn.CommandContext(ctx, "sha256sum", path).Output()
	if err != nil {
		return "", err
	}
	return strings.Fields(string(output))[0], nil
}
