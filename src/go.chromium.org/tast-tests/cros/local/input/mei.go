// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"context"
	"os"
	"unsafe"
)

// SendMEIConnectClientCmd sends MEI_CONNECT_CLIENT_COMMAND to the device.
func SendMEIConnectClientCmd(ctx context.Context, meidev *os.File, buf *[]byte) error {
	ioctlMeiConnectClient := iowr('H', 1, 16)
	return ioctl(int(meidev.Fd()), ioctlMeiConnectClient, uintptr(unsafe.Pointer(&(*buf)[0])))
}

// SysRead reads the data from the device.
func SysRead(ctx context.Context, meidev *os.File, buf *[]byte) (uintptr, error) {
	readCnt, err := sysread(int(meidev.Fd()), uintptr(unsafe.Pointer(&(*buf)[0])), uintptr(unsafe.Sizeof((*buf))))
	if err != nil {
		return 0, err
	}
	return readCnt, nil
}
