// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast-tests/cros/local/shill"
	fmpb "go.chromium.org/tast-tests/cros/services/cros/filemanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			fmpb.RegisterFreezeFUSEServiceServer(srv, &FreezeFUSEService{s})
		},
	})
}

type FreezeFUSEService struct {
	s *testing.ServiceState
}

func (f *FreezeFUSEService) TestMountZipAndSuspend(ctx context.Context, request *fmpb.TestMountZipAndSuspendRequest) (emp *empty.Empty, lastErr error) {
	// Use a shortened context to allow time for required cleanup steps.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	cr, err := chrome.New(
		ctx,
		chrome.GAIALogin(chrome.Creds{User: request.GetUser(), Pass: request.GetPassword()}),
		chrome.ARCDisabled())
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create TestAPIConn for Chrome")
	}

	// This command quickly reproduces freeze timeouts with archives.
	// The PID is assigned to the ui cgroup here to avoid race conditions where
	// find/cat are forked before writing the PID to cgroup.procs.
	// Sync is run before the while loop to speed up the kernel's sync before
	// the stress script starts hammering the filesystem.
	script := "echo $$ > /sys/fs/cgroup/freezer/ui/cgroup.procs;" +
		"sync;" +
		"while true; do find /media/archive -type f; done"

	cmd := testexec.CommandContext(
		ctx,
		"sh",
		"-c",
		script)

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get users Downloads path")
	}

	// Copy the zip file to Downloads folder.
	zipFile := "100000_files_in_one_folder.zip"
	zipPath := path.Join(downloadsPath, zipFile)
	if err := fsutil.CopyFile(request.GetZipDataPath(), zipPath); err != nil {
		return nil, errors.Wrapf(err, "error copying ZIP file to %q", zipPath)
	}
	defer func() {
		if err := os.Remove(zipPath); err != nil {
			lastErr = errors.Wrapf(err, "failed to remove ZIP file %q", zipPath)
			// Log the error now, because this may not the last error.
			testing.ContextLog(cleanupCtx, lastErr)
		}
		if err := cmd.Kill(); err != nil {
			lastErr = errors.Wrap(err, "failed to kill testing script")
			// Log the error now, because this may not the last error.
			testing.ContextLog(cleanupCtx, lastErr)
		}
		cmd.Wait()
		// Restart powerd, otherwise we may get stuck in suspend.
		if err := testexec.CommandContext(cleanupCtx, "restart", "powerd").Run(); err != nil {
			lastErr = errors.Wrap(err, "failed to restart powerd after failed suspend attempt. DUT may get stuck after retry suspend")
		}
	}()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		lastErr = errors.Wrap(err, "could not launch the Files App")
		testing.ContextLog(ctx, lastErr)
		return nil, lastErr
	}
	defer files.Close(cleanupCtx)

	if err := files.OpenDownloads()(ctx); err != nil {
		lastErr = errors.Wrap(err, "could not open Downloads folder")
		testing.ContextLog(ctx, lastErr)
		return nil, lastErr
	}

	// Wait for the zip file to show up in the UI.
	if err := files.WithTimeout(3 * time.Minute).WaitForFile(zipFile)(ctx); err != nil {
		lastErr = errors.Wrap(err, "Waiting for test ZIP file failed")
		testing.ContextLog(ctx, lastErr)
		return nil, lastErr
	}

	if err := files.OpenFile(zipFile)(ctx); err != nil {
		lastErr = errors.Wrap(err, "Opening ZIP file failed")
		testing.ContextLog(ctx, lastErr)
		return nil, lastErr
	}

	node := nodewith.Name("Files - " + zipFile).Role(role.RootWebArea)
	if err := files.WithTimeout(time.Minute).WaitUntilExists(node)(ctx); err != nil {
		lastErr = errors.Wrapf(err, "Mounting ZIP file %q failed", zipFile)
		testing.ContextLog(ctx, lastErr)
		return nil, lastErr
	}

	if err := cmd.Start(); err != nil {
		lastErr = errors.Wrap(err, "Unable to start archive stress script")
		testing.ContextLog(ctx, lastErr)
		return nil, lastErr
	}

	for i := int32(1); i <= request.Iterations; i++ {
		successfulSuspends, err := readSuccessfulSuspends(ctx)
		if err != nil {
			lastErr = errors.Wrap(err, "failed to read successful suspends before suspend attempt")
			testing.ContextLog(ctx, lastErr)
			return nil, lastErr
		}

		// Suspend for 20 seconds since the stress script slows us down. Freeze of userspace happens before setting the
		// RTC to wake the system, so we just need to worry about the up to 20s delay for freeze in the kernel.
		testing.ContextLogf(ctx, "Attempting suspend iteration %d", i)
		if _, err := suspend.Request(ctx, suspend.Delay(0*time.Second), suspend.WithoutRetries(),
			suspend.For(20*time.Second), suspend.Timeout(30*time.Second)); err != nil {
			lastErr = errors.Wrap(err, "DUT failed to properly suspend")
			testing.ContextLog(ctx, lastErr)
			return nil, lastErr
		}

		successfulSuspendsAfter, err := readSuccessfulSuspends(ctx)
		if err != nil {
			lastErr = errors.Wrap(err, "failed to read successful suspends after suspend attempt")
			testing.ContextLog(ctx, lastErr)
			return nil, lastErr
		}
		if successfulSuspendsAfter != successfulSuspends+1 {
			lastErr = errors.Errorf("successful suspends did not increase by 1. Before: %d, After: %d", successfulSuspends, successfulSuspendsAfter)
			testing.ContextLog(ctx, lastErr)
			return nil, lastErr
		}

		if err := shill.WaitForOnlineAfterResume(ctx); err != nil {
			lastErr = errors.Wrap(err, "timed out waiting for the network to connect after resume")
			testing.ContextLog(ctx, lastErr)
			return nil, lastErr
		}

		// GoBigSleepLint: Allow some time for the server to connect to the DUT.
		testing.Sleep(ctx, 5*time.Second)
	}

	return &empty.Empty{}, lastErr
}

var successfulSuspendsRegex = regexp.MustCompile(`success: ([0-9]+)`)

func readSuccessfulSuspends(ctx context.Context) (int, error) {
	out, err := ioutil.ReadFile("/sys/kernel/debug/suspend_stats")
	if err != nil {
		return 0, errors.Wrap(err, "failed to read suspend_stats for the number of successful suspends")
	}
	successfulSuspendsMatch := successfulSuspendsRegex.FindSubmatch([]byte(out))
	if successfulSuspendsMatch == nil {
		return 0, errors.New("failed to parse number of successful suspends from suspend_stats")
	}
	return strconv.Atoi(string(successfulSuspendsMatch[1]))
}
