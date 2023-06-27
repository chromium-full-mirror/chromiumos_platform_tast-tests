// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"bytes"
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// Guest is an interface to a generic guest OS, be it a container or VM.
// It can be used by tests that do not use the implementation details of any
// specific type of guest OS.
type Guest interface {
	Command(ctx context.Context, vshArgs ...string) *testexec.Cmd
	ReadFile(ctx context.Context, filePath string) (string, error)
	CheckFileContent(ctx context.Context, filePath, testString string) error
}

// containerCommand returns a testexec.Cmd with a vsh command that will run in
// the specified container.
func containerCommand(ctx context.Context, vmName, containerName, ownerID string, vshArgs ...string) *testexec.Cmd {
	args := append([]string{"--vm_name=" + vmName,
		"--target_container=" + containerName,
		"--owner_id=" + ownerID,
		"--"},
		vshArgs...)
	cmd := testexec.CommandContext(ctx, "vsh", args...)
	// Add an empty buffer for stdin to force allocating a pipe. vsh uses
	// epoll internally and generates a warning (EPERM) if stdin is /dev/null.
	cmd.Stdin = &bytes.Buffer{}
	return cmd
}

// readFile reads the content of file using command cat and returns it as a string.
func readFile(ctx context.Context, guest Guest, filePath string) (content string, err error) {
	cmd := guest.Command(ctx, "cat", filePath)
	result, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "failed to cat the content of %s", filePath)
	}

	return string(result), nil
}

// checkFileContent checks that the content of the specified file equals to the given string.
// Returns error if fail to read content or the contest does not equal to the given string.
func checkFileContent(ctx context.Context, guest Guest, filePath, testString string) error {
	content, err := guest.ReadFile(ctx, filePath)
	if err != nil {
		return errors.Wrapf(err, "failed to cat the result %s", filePath)
	}
	if content != testString {
		return errors.Wrapf(err, "want %s, got %q", testString, content)
	}
	return nil
}
