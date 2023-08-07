// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

const (
	fakeContainerName = "penguin"
)

// BruschettaVM implements vm.Guest
type BruschettaVM struct {
	VM *VM
}

// Command runs a command in a Bruschetta VM.
func (bru *BruschettaVM) Command(ctx context.Context, vshArgs ...string) *testexec.Cmd {
	return containerCommand(ctx, bru.VM.Name(), fakeContainerName, bru.VM.Concierge.GetOwnerID(), vshArgs...)
}

// ReadFile reads the content of file using command cat and returns it as a string.
func (bru *BruschettaVM) ReadFile(ctx context.Context, filePath string) (content string, err error) {
	return readFile(ctx, bru, filePath)
}

// PushFile copies a local file to the container's filesystem.
func (bru *BruschettaVM) PushFile(ctx context.Context, localPath, containerPath string) error {
	return pushFile(ctx, fakeContainerName, bru.VM, localPath, containerPath)
}

// GetFile copies a remote file from the container's filesystem.
func (bru *BruschettaVM) GetFile(ctx context.Context, containerPath, localPath string) error {
	return getFile(ctx, fakeContainerName, bru.VM, containerPath, localPath)
}

// CheckFileContent checks that the content of the specified file equals to the given string.
// Returns error if fail to read content or the contest does not equal to the given string.
func (bru *BruschettaVM) CheckFileContent(ctx context.Context, filePath, testString string) error {
	return checkFileContent(ctx, bru, filePath, testString)
}

// GetFileList returns a list of the files in the given path in the container.
func (bru *BruschettaVM) GetFileList(ctx context.Context, path string) (fileList []string, err error) {
	return getFileList(ctx, bru, path)
}

// Cleanup removes all the files under the specific path.
func (bru *BruschettaVM) Cleanup(ctx context.Context, path string) error {
	return cleanup(ctx, bru, path)
}
