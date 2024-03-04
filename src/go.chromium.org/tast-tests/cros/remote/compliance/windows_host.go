// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package compliance

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
)

// powerShellCmd wraps execution on windows devices in powershell since it's the closest thing
// to a linux environment that we have.
func powerShellCmd(dir string, args []string) string {
	cmd := "powershell -Command " + strings.Join(args, " ")
	if dir != "" {
		cmd = fmt.Sprintf("cd %q; %s", dir, cmd)
	}
	return cmd
}

// winPlatform represents a system with a Windows platform for controlling compliance device.
var winPlatform = &ssh.Platform{BuildShellCommand: powerShellCmd}

// WindowsHost provides methods for controlling a Windows host machine that a compliance tester
// is connected to.
type WindowsHost struct {
	host *ssh.Conn
}

// NewWindowsHost initializes a compliance tester host for a windows machine.
func NewWindowsHost(ctx context.Context, hostname string, dut *dut.DUT, user string) (*WindowsHost, error) {
	// Connect to compliance host.
	sshOptions := &ssh.Options{
		KeyFile:        dut.KeyFile(),
		KeyDir:         dut.KeyDir(),
		ConnectTimeout: 10 * time.Second,
		User:           user,
		Hostname:       hostname,
		Platform:       winPlatform,
	}

	conn, err := ssh.New(ctx, sshOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to compliance host")
	}

	return &WindowsHost{
		host: conn,
	}, nil
}

// Run runs a command on the host and waits for it to complete.
func (w *WindowsHost) Run(ctx context.Context, cmd string, args ...string) error {
	if out, err := w.host.CommandContext(ctx, cmd, args...).CombinedOutput(); err != nil {
		return errors.Wrapf(err, "failed to run command %q: %s", cmd, string(out))
	}
	return nil
}

// CreateTempDirectory creates a temporary directory on the host.
func (w *WindowsHost) CreateTempDirectory(ctx context.Context) (string, func(context.Context) error, error) {
	out, err := w.host.CommandContext(ctx, "Join-Path $Env:Temp ([System.IO.Path]::GetRandomFileName())").CombinedOutput()
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to create temporary file name: %s", string(out))
	}

	path := strings.TrimSpace(string(out))
	if out, err := w.host.CommandContext(ctx, "mkdir", path).CombinedOutput(); err != nil {
		return "", nil, errors.Wrapf(err, "failed to create temporary directory: %q, %s", path, string(out))
	}

	remove := func(ctx context.Context) error {
		if err := w.Run(ctx, "Remove-Item", path, "-Force", "-Recurse"); err != nil {
			return errors.Wrapf(err, "failed to remove tempo directory: %q", path)
		}
		return nil
	}
	return path, remove, nil
}

// GetFile copies a file or directory from the host to the local machine.
// dst is the destination directory.
func (w *WindowsHost) GetFile(ctx context.Context, src, dst string) error {
	dir, base, err := w.SplitPath(ctx, src)
	if err != nil {
		return errors.Wrap(err, "failed to split src path")
	}

	tarCmd := w.host.CommandContext(ctx, "tar", "-cvf", "-", "-C", dir, base)
	p, err := tarCmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "failed to get stdout pipe")
	}

	if err := tarCmd.Start(); err != nil {
		return errors.Wrap(err, "failed to read tar file contents")
	}
	defer tarCmd.Wait()
	defer tarCmd.Abort()

	// Now read the pipe we just created.
	untarCmd := exec.CommandContext(ctx, "/bin/tar", "-C", dst, "-xvf", "-")
	untarCmd.Stdin = p
	if _, err := untarCmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, "running local tar failed")
	}

	return nil
}

// SplitPath splits a path on the host into it's directory and base filename.
//
// It's easier to move this to a separate function as the default path libraries will use the
// local machines path separator when parsing filepaths.
func (w *WindowsHost) SplitPath(ctx context.Context, path string) (string, string, error) {
	out, err := w.host.CommandContext(ctx, "Split-Path", path, "-Parent").Output()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get directory from path")
	}
	dir := strings.TrimSpace(string(out))

	out, err = w.host.CommandContext(ctx, "Split-Path", path, "-Leaf").Output()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get base from path")
	}
	return dir, strings.TrimSpace(string(out)), nil
}

// Close releases any resources held open by the host.
func (w *WindowsHost) Close(ctx context.Context) error {
	return w.host.Close(ctx)
}
