// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package audioutils provides the util functions needed for the vm audio tests
package audioutils

import (
	"context"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	cgroupPath string = "/sys/fs/cgroup/cpu/termina/tasks"
)

// Config includes all the params needed to setup crosvm for vm audio tests.
type Config struct {
	CrosvmArgs []string
}

// RunCrosvm runs crosvm.
func RunCrosvm(ctx context.Context, kernelPath, kernelLogPath string, kernelArgs []string, config Config) error {
	crosvmCmd, cleanupFunc, err := crosvmCmd(ctx, kernelPath, kernelLogPath, kernelArgs, config)
	if err != nil {
		return errors.Wrap(err, "failed to get crosvm cmd")
	}
	defer cleanupFunc()

	testing.ContextLog(ctx, "Launching crosvm")
	if err = crosvmCmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to run crosvm")
	}

	return nil
}

// crosvmCmd setups the crosvm command for audio device testing.
func crosvmCmd(ctx context.Context, kernelPath, kernelLogPath string, kernelArgs []string, config Config) (crosvmCmd *testexec.Cmd, cleanupFunc func() error, retErr error) {
	cleanupFunc = func() error { return nil }
	defer func() {
		if retErr != nil {
			if err := cleanupFunc(); err != nil {
				testing.ContextLog(ctx, "Failed to cleanup: ", err)
			}
			cleanupFunc = nil
		}
	}()

	kernelParams := []string{
		"root=/dev/root",
		"rootfstype=virtiofs",
		"rw",
	}
	kernelParams = append(kernelParams, kernelArgs...)

	crosvmArgs := []string{"crosvm", "run"}
	crosvmArgs = append(crosvmArgs, config.CrosvmArgs...)
	crosvmArgs = append(crosvmArgs,
		"-p", "\""+strings.Join(kernelParams, " ")+"\"",
		"--serial", fmt.Sprintf("type=file,num=1,console=true,path=%s", kernelLogPath),
		"--shared-dir", "/:/dev/root:type=fs:cache=always",
		kernelPath)

	// Add the shell process id to the control group
	cmdStr := []string{"echo $$ >", cgroupPath, "&&"}
	// Set the rtprio limit of the shell process to unlimited.
	cmdStr = append(cmdStr, "prlimit", "--pid", "$$", "--rtprio=unlimited", "&&")
	cmdStr = append(cmdStr, crosvmArgs...)
	testing.ContextLog(ctx, "cmd: ", cmdStr)
	crosvmCmd = testexec.CommandContext(ctx, "sh", []string{"-c", strings.Join(cmdStr, " ")}...)

	// Same effect as calling `newgrp cras` before `crosvm` in shell
	// This is needed to access /run/cras/.cras_socket (legacy socket)
	crasGrp, err := user.LookupGroup("cras")
	if err != nil {
		return nil, cleanupFunc, errors.Wrap(err, "failed to find group id for cras")
	}
	crasGrpID, err := strconv.ParseUint(crasGrp.Gid, 10, 32)
	if err != nil {
		return nil, cleanupFunc, errors.Wrap(err, "failed to convert cras grp id to integer")
	}
	crosvmCmd.Cred(syscall.Credential{
		Uid:         0,
		Gid:         0,
		Groups:      []uint32{uint32(crasGrpID)},
		NoSetGroups: false,
	})

	return crosvmCmd, cleanupFunc, nil
}
