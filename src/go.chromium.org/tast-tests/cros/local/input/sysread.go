// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"golang.org/x/sys/unix"
)

// sysread makes an sysread system call against fd.
func sysread(fd int, data, req uintptr) (uintptr, error) {
	readCnt, _, errno := unix.Syscall(unix.SYS_READ, uintptr(fd), data, uintptr(req))
	if errno != 0 {
		return 0, errno
	}
	return readCnt, nil
}
