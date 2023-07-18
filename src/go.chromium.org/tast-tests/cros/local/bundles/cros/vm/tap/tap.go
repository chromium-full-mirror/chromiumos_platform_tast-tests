// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tap provides the util functions to get tap device from patchpanel.
package tap

import (
	"context"
	"net"
	"unsafe"

	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

type ifreq struct {
	name  [unix.IFNAMSIZ]byte
	flags int16
}

type tapDevice struct {
	Fd      int
	Addr    net.IP
	Gateway net.IP
}

// GetTap gets a tap device from patchpanel client
func GetTap(ctx context.Context, pc *patchpanel.Client, cid uint32) (device tapDevice, cleanup func(context.Context), err error) {
	resp, err := pc.NotifyTerminaVMStartup(ctx, cid)
	if err != nil {
		err = errors.Wrap(err, "failed to send NotifyTerminaVMStartup request to patchpanel")
		return
	}

	shutdown := func(cleanupCtx context.Context) {
		if err := pc.NotifyTerminaVMShutdown(cleanupCtx, cid); err != nil {
			testing.ContextLog(ctx, "Failed to notify termina shutdown: ", err)
		}
	}

	fd, err := openTapDevice(resp.TapDeviceIfname)
	if err != nil {
		shutdown(ctx)
		err = errors.Wrap(err, "failed to open Tap device")
		return
	}

	device = tapDevice{
		Fd:      fd,
		Addr:    resp.Ipv4Address,
		Gateway: resp.GatewayIpv4Address,
	}

	cleanup = func(cleanupCtx context.Context) {
		shutdown(cleanupCtx)
		unix.Close(fd)
	}

	return
}

func openTapDevice(ifname string) (int, error) {
	const path = "/dev/net/tun"

	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to open Tap device: %v", ifname)
	}

	if len(ifname) > unix.IFNAMSIZ-1 {
		unix.Close(fd)
		return 0, errors.Wrapf(err, "too long Ifname: %s", ifname)
	}

	ifr := ifreq{}
	copy(ifr.name[:], ifname)
	ifr.flags = unix.IFF_TAP | unix.IFF_NO_PI | unix.IFF_VNET_HDR
	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		unix.TUNSETIFF,
		uintptr(unsafe.Pointer(&ifr)),
	); errno != 0 {
		unix.Close(fd)
		return 0, errors.Errorf("failed to set network interface: %s", errno.Error())
	}

	return fd, nil
}
