// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filesystem

import (
	"context"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// InodeFlags returns the inode flags of the given file.
func InodeFlags(path string) (int, error) {
	fd, err := unix.Open(path, unix.O_RDONLY, 0 /* perm */)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to open an existing file %s", path)
	}
	defer unix.Close(fd)
	return unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
}

// SetInodeFlags sets the given inode flags to the specified file.
// NOTE: The users of this function need to be careful not to remove the
// unremovable flags. To avoid it, it is recommended to follow the convention to
// get and set flags (crbug/1105400#c7). A typical use case will look like:
//
//	oldFlags, err := filesystem.InodeFlags(path)
//	if err != nil {
//	    ... // error handling
//	}
//	if err := filesystem.SetInodeFlags(path, flags|oldFlags); err != nil {
//	    ... // error handling
//	}
func SetInodeFlags(path string, flags int) error {
	fd, err := unix.Open(path, unix.O_RDONLY, 0 /* perm */)
	if err != nil {
		return errors.Wrapf(err, "failed to open an existing file %s", path)
	}
	defer unix.Close(fd)
	return unix.IoctlSetPointerInt(fd, unix.FS_IOC_SETFLAGS, flags)
}

// QuotaProjectID returns the quota project ID of the specified file.
func QuotaProjectID(ctx context.Context, path string) (int64, error) {
	// Output looks like:
	// " 1003 ---------E----e----- <path>"
	out, err := testexec.CommandContext(ctx, "lsattr", "-p", path).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.Split(strings.TrimSpace(string(out)), " ")[0], 10, 64)
}

// SetQuotaProjectID sets the quota project ID on the specified file.
func SetQuotaProjectID(ctx context.Context, path string, projectID int64) error {
	return testexec.CommandContext(ctx, "chattr", "-p", strconv.FormatInt(projectID, 10), path).Run(testexec.DumpLogOnError)
}
