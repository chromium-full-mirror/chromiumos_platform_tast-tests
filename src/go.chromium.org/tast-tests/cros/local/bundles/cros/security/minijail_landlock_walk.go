// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MinijailLandlockWalk,
		Desc: "Verifies Landlock only adds expected restrictions",
		Contacts: []string{
			"minijail-dev@google.com",
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Minijail
		BugComponent: "b:1099158",
		SoftwareDeps: []string{"landlock_enabled"},
		Attr:         []string{"group:mainline"},
	})
}

func MinijailLandlockWalk(ctx context.Context, s *testing.State) {
	dirsArray := []string{"/home", "/media"}

	for _, dir := range dirsArray {
		s.Log("Examining: ", dir)
		walkFiles(ctx, s, dir)
	}
}

func walkFiles(ctx context.Context, s *testing.State, dir string) error {
	const (
		minijailPath   = "/sbin/minijail0"
		walkerTestPath = "/usr/local/libexec/tast/helpers/local/cros/security.Minijail.landlock_walk"
		enoentError    = "exit status 2"
	)
	numFiles := 0

	walkErr := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			return nil
		}

		minijailArgs := []string{"-u", "chronos",
			"-g", "chronos-access",
			"-v",
		}
		numFiles++
		args := append(minijailArgs, walkerTestPath, path, "rw")
		err = testexec.CommandContext(ctx, minijailPath, args...).Run()

		// Landlock policy based on src/platform2/login_manager/landlock_policy.cc.
		// TODO(b/294842930): move to a conf file used by login_manager,
		// once libminijail has support.
		landlockArgs := []string{"--fs-default-paths",
			"--fs-path-rx=/",
			"--fs-path-rw=/dev",
			"--fs-path-rw=/home/chronos",
			"--fs-path-rw=/home/user",
			"--fs-path-rw=/media",
			"--fs-path-rw=/mnt",
			"--fs-path-rw=/proc",
			"--fs-path-rw=/run",
			"--fs-path-rw=/sys/fs/cgroup",
			"--fs-path-rw=/tmp",
			"--fs-path-rw=/var/cache",
			"--fs-path-rw=/var/lib",
			"--fs-path-rw=/var/lock",
			"--fs-path-rw=/var/log",
			"--fs-path-rw=/var/spool/support",
			"--fs-path-rw=/var/tmp",
			walkerTestPath, path, "rw"}
		landlockArgs = append(minijailArgs, landlockArgs...)
		errLandlock := testexec.CommandContext(ctx, minijailPath, landlockArgs...).Run()

		if (err == nil) != (errLandlock == nil) {
			// Handle file deleted during the walk.
			if (err != nil && err.Error() == enoentError) || (errLandlock != nil && errLandlock.Error() == enoentError) {
				return nil
			}

			errorMsg := "Could not verify access rights: " + path
			if err != nil {
				errorMsg += " non-Landlock error:" + err.Error()
			}
			if errLandlock != nil {
				errorMsg += " Landlock error: " + errLandlock.Error()
			}
			return errors.New(errorMsg)
		}
		return nil
	})
	s.Logf("Examined %d files", numFiles)

	if walkErr != nil {
		s.Error("Failed walking filesystem: ", walkErr)
	}
	return nil
}
