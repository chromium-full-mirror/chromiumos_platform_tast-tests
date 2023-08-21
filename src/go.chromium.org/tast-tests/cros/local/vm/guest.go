// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cpb "chromiumos/system_api/vm_cicerone_proto"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// Guest is an interface to a generic guest OS, be it a container or VM.
// It can be used by tests that do not use the implementation details of any
// specific type of guest OS.
type Guest interface {
	Command(ctx context.Context, vshArgs ...string) *testexec.Cmd
	ReadFile(ctx context.Context, filePath string) (string, error)
	CheckFileContent(ctx context.Context, filePath, testString string) error
	PushFile(ctx context.Context, localPath, containerPath string) error
	GetFile(ctx context.Context, containerPath, localPath string) error
	GetFileList(ctx context.Context, path string) (fileList []string, err error)
	Cleanup(ctx context.Context, path string) error
	WriteFile(ctx context.Context, path, contents string) error
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

// sftpCommand executes an SFTP command to perform a file transfer with the guest.
// sftpCmd is any sftp command to be batch executed by sftp "-b" option.
func sftpCommand(ctx context.Context, contextID int64, sftpVsockPort uint32, sftpCmd string) error {
	// Create temp dir to store sftp command and vsock adapter script.
	// The adapter script is required to use sftp over vsock.
	// Though we can also pipe the commands to sftp via stdin, errors are not reflected on the
	// exit code of the sftp process. The exit code of "sftp -b" honors errors.
	dir, err := os.MkdirTemp("", "tast_vm_sftp_")
	if err != nil {
		return errors.Wrap(err, "failed to create temp dir for sftp")
	}
	defer os.RemoveAll(dir)

	sftpAdapter := fmt.Sprintf(`#!/bin/sh
exec socat stdio vsock-connect:%d:%d
`, contextID, sftpVsockPort)

	sftpAdapterFile := filepath.Join(dir, "sftp_adapter")
	cmdFile := filepath.Join(dir, "cmd")
	if err := os.WriteFile(sftpAdapterFile, []byte(sftpAdapter), 0755); err != nil {
		return errors.Wrap(err, "failed to write sftp adapter script")
	}
	if err := os.WriteFile(cmdFile, []byte(sftpCmd), 0644); err != nil {
		return errors.Wrap(err, "failed to write sftp command to temp file")
	}

	sftpArgs := []string{
		"-b", cmdFile,
		"-r",
		"-S", sftpAdapterFile,
		"container", // This is ignored by the sftp adapter script
	}
	cmd := testexec.CommandContext(ctx, "sftp", sftpArgs...)
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return errors.Wrapf(err, "failed to execute %q with sftp command %q", strings.Join(cmd.Args, " "), sftpCmd)
	}
	return nil
}

// sftpVsockPort gets the vsock port where openssh-sftp-server is running for a guest.
func sftpVsockPort(ctx context.Context, containerName string, vm *VM) (uint32, error) {
	resp := &cpb.GetGarconSessionInfoResponse{}
	if err := dbusutil.CallProtoMethod(ctx, vm.Concierge.ciceroneObj, ciceroneInterface+".GetGarconSessionInfo",
		&cpb.GetGarconSessionInfoRequest{
			VmName:        vm.name,
			ContainerName: containerName,
			OwnerId:       vm.Concierge.ownerID,
		}, resp); err != nil {
		return 0, err
	}

	return resp.SftpVsockPort, nil
}

// pushFile copies a local file to the guest's filesystem.
func pushFile(ctx context.Context, containerName string, vm *VM, localPath, containerPath string) error {
	port, err := sftpVsockPort(ctx, containerName, vm)
	if err != nil {
		return errors.Wrap(err, "failed to get SFTP vsock port")
	}

	testing.ContextLogf(ctx, "Copying local file %v to guest %v", localPath, containerPath)
	// Double quotes in sftp keeps spaces and invalidate special characters like * or ?.
	// Golang %q escapes " and \ and sftp unescape them correctly.
	// To handle a leading -, "--" is added after the command.
	putCmd := fmt.Sprintf("put -- %q %q", localPath, containerPath)
	return sftpCommand(ctx, vm.ContextID, port, putCmd)
}

// getFile copies a remote file from the guest's filesystem.
func getFile(ctx context.Context, containerName string, vm *VM, containerPath, localPath string) error {
	port, err := sftpVsockPort(ctx, containerName, vm)
	if err != nil {
		return errors.Wrap(err, "failed to get SFTP vsock port")
	}

	testing.ContextLogf(ctx, "Copying file %v from guest %v", localPath, containerPath)
	// Double quotes in sftp keeps spaces and invalidate special characters like * or ?.
	// Golang %q escapes " and \ and sftp unescape them correctly.
	// To handle a leading -, "--" is added after the command.
	getCmd := fmt.Sprintf("get -- %q %q", containerPath, localPath)
	return sftpCommand(ctx, vm.ContextID, port, getCmd)
}

// getFileList returns a list of the files in the given path in the container.
func getFileList(ctx context.Context, guest Guest, path string) (fileList []string, err error) {
	// Get files in the path in guest.
	result, err := guest.Command(ctx, "ls", "-1", path).Output()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to run 'ls %s' in guest", path)
	}
	fileList = strings.Split(string(result), "\n")

	// Delete the last empty item if it is there.
	if len(fileList) > 0 && fileList[len(fileList)-1] == "" {
		fileList = fileList[:len(fileList)-1]
	}

	return fileList, nil
}

// cleanup removes all the files under the specific path.
func cleanup(ctx context.Context, guest Guest, path string) error {
	list, err := guest.GetFileList(ctx, path)
	if err != nil {
		return errors.Wrapf(err, "failed to get file list of %s in guest: ", path)
	}
	for _, file := range list {
		if err := guest.Command(ctx, "rm", "-rf", file).Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "failed to delete %s in %s", file, path)
		}
	}
	return nil
}

// writeFile creates a file in the container using echo.
func writeFile(ctx context.Context, guest Guest, filePath, fileContent string) error {
	if err := guest.Command(ctx, "mkdir", "-p", filepath.Dir(filePath)).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to create the directory containing the file %v in guest", filePath)
	}
	if err := guest.Command(ctx, "sh", "-c", fmt.Sprintf("echo -n %s > %s", shutil.Escape(fileContent), filePath)).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to write file %v in guest", filePath)
	}
	return nil
}
