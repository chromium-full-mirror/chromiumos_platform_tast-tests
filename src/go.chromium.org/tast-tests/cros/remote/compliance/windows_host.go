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
	cmd := strings.Join(args, " ")
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
	Host *ssh.Conn
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
		Host: conn,
	}, nil
}

// Run runs a command on the host and waits for it to complete.
func (w *WindowsHost) Run(ctx context.Context, cmd string, args ...string) (string, error) {
	out, err := w.Host.CommandContext(ctx, cmd, args...).CombinedOutput()
	if err != nil {
		return string(out), errors.Wrapf(err, "failed to run command %q: %s", cmd, string(out))
	}
	return string(out), nil
}

// CreateTempDirectory creates a temporary directory on the host.
func (w *WindowsHost) CreateTempDirectory(ctx context.Context) (string, func(context.Context) error, error) {
	out, err := w.Host.CommandContext(ctx, "Join-Path $Env:Temp ([System.IO.Path]::GetRandomFileName())").CombinedOutput()
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to create temporary file name: %s", string(out))
	}

	path := strings.TrimSpace(string(out))
	if out, err := w.Host.CommandContext(ctx, "mkdir", path).CombinedOutput(); err != nil {
		return "", nil, errors.Wrapf(err, "failed to create temporary directory: %q, %s", path, string(out))
	}

	remove := func(ctx context.Context) error {
		if _, err := w.Run(ctx, "Remove-Item", path, "-Force", "-Recurse"); err != nil {
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
	tarCmd := w.Host.CommandContext(ctx, "tar", "-cvf", "-", "-C", fmt.Sprintf("'%s'", dir), base)
	p, err := tarCmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "failed to get stdout pipe")
	}

	// Now read the pipe we just created.
	untarCmd := exec.CommandContext(ctx, "/bin/tar", "-C", dst, "-xvf", "-")
	untarCmd.Stdin = p

	if err := tarCmd.Start(); err != nil {
		return errors.Wrap(err, "failed to read tar file contents")
	}
	defer tarCmd.Wait()
	defer tarCmd.Abort()

	if out, err := untarCmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "running local tar failed: %s", out)
	}

	return nil
}

// SplitPath splits a path on the host into it's directory and base filename.
//
// It's easier to move this to a separate function as the default path libraries will use the
// local machines path separator when parsing filepaths.
func (w *WindowsHost) SplitPath(ctx context.Context, path string) (string, string, error) {
	out, err := w.Host.CommandContext(ctx, "Split-Path", path, "-Parent").Output()
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to get directory from path: %s", out)
	}
	dir := strings.TrimSpace(string(out))

	out, err = w.Host.CommandContext(ctx, "Split-Path", path, "-Leaf").Output()
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to get base from path: %s", out)
	}
	return dir, strings.TrimSpace(string(out)), nil
}

// Close releases any resources held open by the host.
func (w *WindowsHost) Close(ctx context.Context) error {
	return w.Host.Close(ctx)
}

// GetNewestDirectory gets the newest directory in a given directory
func (w *WindowsHost) GetNewestDirectory(ctx context.Context, reportDirectory string) (string, error) {
	dir := w.Host.CommandContext(ctx, `C:\Users\CrOSECMinion\Documents\get_newest_dir.ps1`, reportDirectory)
	out, err := dir.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, string(out))
	}
	return strings.TrimSpace(string(out)), err
}

// GetFileNames retrieves the names of all the files and directories found in a given directory
func (w *WindowsHost) GetFileNames(ctx context.Context, directory string) (string, error) {
	cmd := w.Host.CommandContext(ctx, `C:\Users\CrOSECMinion\Documents\filenames.ps1`, fmt.Sprintf(`'%s'`, directory))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, string(out))
	}
	trimmed := strings.TrimSpace(string(out))
	return trimmed, err
}

// Connected returns true if a usable connection to the WindowsHost is held.
func (w *WindowsHost) Connected(ctx context.Context) bool {
	if w == nil || w.Host == nil {
		return false
	}
	if err := w.Host.Ping(ctx, 3*time.Second); err != nil {
		return false
	}
	return true
}
