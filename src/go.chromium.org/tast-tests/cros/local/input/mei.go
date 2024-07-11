// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"context"
	"os"
	"unsafe"
)

const maxBufSize = 1<<31 - 1

// SendMEIConnectClientCmd sends MEI_CONNECT_CLIENT_COMMAND to the device.
func SendMEIConnectClientCmd(ctx context.Context, meidev *os.File, buf *[16]byte) error {
	ioctlMeiConnectClient := iowr('H', 1, 16)
	return ioctl(int(meidev.Fd()), ioctlMeiConnectClient, uintptr(unsafe.Pointer(buf)))
}

// SysRead reads the data from the device.
func SysRead(ctx context.Context, meidev *os.File, buf *[maxBufSize]byte) (uintptr, error) {
	readCnt, err := sysread(int(meidev.Fd()), uintptr(unsafe.Pointer(buf)), uintptr(unsafe.Sizeof(buf)))
	if err != nil {
		return 0, err
	}
	return readCnt, nil
}
