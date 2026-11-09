// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// usbdevfsCtrlTransfer matches Linux uapi struct usbdevfs_ctrltransfer in
// <linux/usbdevice_fs.h>.
type usbdevfsCtrlTransfer struct {
	bRequestType uint8
	bRequest     uint8
	wValue       uint16
	wIndex       uint16
	wLength      uint16
	timeout      uint32
	data         unsafe.Pointer
}

const (
	sysfsNetRoot       = "/sys/class/net"
	r8152DriverName    = "r8152"
	r8152ReqTypeRead   = 0xc0
	r8152ReqTypeWrite  = 0x40
	r8152ReqRegs       = 0x05
	r8152PLARCR        = 0xc010
	r8152MCUTypePLA    = 0x0100
	r8152ByteEnDword   = 0x00ff
	r8152CtrlTimeoutMS = 1000

	r8152RCRAPM = uint32(0x02) // Accept physical match (unicast).
	r8152RCRAM  = uint32(0x04) // Accept multicast.
	r8152RCRAB  = uint32(0x08) // Accept broadcast.

	// usbdevfsControlIoctl encodes _IOWR('U', 0, struct usbdevfs_ctrltransfer).
	usbdevfsControlIoctl = (uintptr(3) << 30) | (unsafe.Sizeof(usbdevfsCtrlTransfer{}) << 16) | (uintptr('U') << 8)
)

// disableR8152Broadcast disables hardware broadcast reception on connected,
// routed r8152 Ethernet interfaces.
func disableR8152Broadcast(ctx context.Context) (CleanupCallback, error) {
	return Nested(ctx, "disable r8152 broadcast", func(s *Setup) error {
		manager, err := shill.NewManager(ctx)
		if err != nil {
			return errors.Wrap(err, "creating shill manager")
		}
		svcs, props, err := manager.ServicesByTechnology(ctx, shill.TechnologyEthernet)
		if err != nil {
			return errors.Wrap(err, "listing ethernet services")
		}
		for i, svc := range svcs {
			if connected, err := props[i].GetBool(shillconst.ServicePropertyIsConnected); err != nil || !connected {
				continue
			}
			netCfg, err := svc.GetNetworkConfig(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return errors.Wrap(err, "getting ethernet network config")
				}
				testing.ContextLogf(ctx, "Skipping Ethernet service %s without complete network config: %v", svc.ObjectPath(), err)
				continue
			}
			if !netCfg.HasIPv4Address() || netCfg.IPv4Gateway == nil || netCfg.IPv4Gateway.IsUnspecified() {
				continue
			}
			ifname, err := svc.GetDeviceInterface(ctx)
			if err != nil {
				return errors.Wrapf(err, "getting interface for %s", svc.ObjectPath())
			}
			s.Add(disableInterfaceR8152Broadcast(ctx, ifname))
		}
		return nil
	})
}

func disableInterfaceR8152Broadcast(ctx context.Context, ifname string) (CleanupCallback, error) {
	changed, err := setR8152Broadcast(ctx, ifname, false)
	if err != nil || !changed {
		return nil, err
	}
	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Re-enabling r8152 broadcast on network interface %q", ifname)
		// Other power setups can briefly trigger USB dongle re-enumeration
		// (see b/262254989), so poll until the interface is back.
		return testing.Poll(ctx, func(ctx context.Context) error {
			_, err := setR8152Broadcast(ctx, ifname, true)
			return err
		}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second})
	}, nil
}

// setR8152Broadcast sets the hardware broadcast accept bit (RCR_AB) in the
// Realtek r8152/r8153/r8156 PLA_RCR register for ifname, and reports whether
// PLA_RCR was modified. Non-r8152 interfaces return (false, nil). Kernel
// rx-mode updates, link flaps, or suspend/resume reset RCR_AB to enabled.
func setR8152Broadcast(ctx context.Context, ifname string, enable bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	devNode, multicastOff, ok, err := r8152USBDev(ifname)
	if err != nil || !ok {
		return false, err
	}

	fd, err := unix.Open(devNode, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, errors.Wrapf(err, "open %s for %q", devNode, ifname)
	}
	defer unix.Close(fd)

	var before uint32
	if !enable && multicastOff {
		// Wait for r8152's async _rtl8152_set_rx_mode work item to finish
		// clearing RCR_AM before modifying PLA_RCR so it cannot overwrite our
		// RCR_AB update.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			rcr, err := r8152ReadPLARCR(ctx, fd)
			if err != nil {
				return testing.PollBreak(err)
			}
			before = rcr
			if rcr&r8152RCRAM != 0 {
				return errors.Errorf("RCR_AM still set (PLA_RCR=0x%08x)", rcr)
			}
			return nil
		}, &testing.PollOptions{Timeout: 2 * time.Second, Interval: 50 * time.Millisecond}); err != nil {
			return false, errors.Wrapf(err, "wait for r8152 multicast sync on %q (%s)", ifname, devNode)
		}
	} else {
		before, err = r8152ReadPLARCR(ctx, fd)
		if err != nil {
			return false, errors.Wrapf(err, "read PLA_RCR on %q (%s)", ifname, devNode)
		}
	}

	if before&r8152RCRAPM == 0 {
		if enable {
			// Rx is currently disabled (e.g. carrier down); the r8152 driver
			// unconditionally sets RCR_AB | RCR_APM on the next link-up.
			return false, nil
		}
		return false, errors.Errorf("r8152 unicast Rx not active on %q (%s): PLA_RCR=0x%08x", ifname, devNode, before)
	}

	target := before &^ r8152RCRAB
	if enable {
		target = before | r8152RCRAB
	}
	if target == before {
		return false, nil
	}

	if err := r8152WritePLARCR(ctx, fd, target); err != nil {
		return false, errors.Wrapf(err, "write PLA_RCR on %q (%s)", ifname, devNode)
	}
	after, err := r8152ReadPLARCR(ctx, fd)
	if err != nil {
		return false, errors.Wrapf(err, "verify PLA_RCR on %q (%s)", ifname, devNode)
	}
	if (after&r8152RCRAB != 0) != enable || after&r8152RCRAPM == 0 {
		return false, errors.Errorf("PLA_RCR readback on %q (%s) got 0x%08x, want 0x%08x", ifname, devNode, after, target)
	}
	testing.ContextLogf(ctx, "Updated r8152 broadcast on %q (%s): PLA_RCR 0x%08x -> 0x%08x", ifname, devNode, before, after)
	return true, nil
}

func r8152USBDev(ifname string) (devNode string, multicastOff, ok bool, err error) {
	if ifname == "" || ifname == "." || ifname == ".." || strings.ContainsRune(ifname, '/') {
		return "", false, false, errors.Errorf("invalid interface name %q", ifname)
	}
	ifaceDir := filepath.Join(sysfsNetRoot, ifname)
	if _, err := os.Stat(ifaceDir); err != nil {
		return "", false, false, errors.Wrapf(err, "stat sysfs interface %q", ifname)
	}

	devSym := filepath.Join(ifaceDir, "device")
	driver, err := os.Readlink(filepath.Join(devSym, "driver"))
	if errors.Is(err, os.ErrNotExist) {
		// Re-stat ifaceDir in case the USB NIC disconnected between Stat and Readlink.
		if _, statErr := os.Stat(ifaceDir); statErr != nil {
			return "", false, false, errors.Wrapf(statErr, "stat sysfs interface %q", ifname)
		}
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, errors.Wrapf(err, "read driver symlink for %q", ifname)
	}
	if filepath.Base(driver) != r8152DriverName {
		return "", false, false, nil
	}

	flags, err := readSysfsUint(filepath.Join(ifaceDir, "flags"), 0)
	if err != nil {
		return "", false, false, errors.Wrapf(err, "read interface flags for %q", ifname)
	}
	const rxMulticastFlags = uint32(unix.IFF_MULTICAST | unix.IFF_ALLMULTI | unix.IFF_PROMISC)
	multicastOff = flags&rxMulticastFlags == 0

	intfDir, err := filepath.EvalSymlinks(devSym)
	if err != nil {
		return "", false, false, errors.Wrapf(err, "resolve sysfs device path for %q", ifname)
	}
	usbDevDir := filepath.Dir(intfDir)
	busNum, err := readSysfsUint(filepath.Join(usbDevDir, "busnum"), 10)
	if err != nil {
		return "", false, false, errors.Wrapf(err, "read USB busnum for %q", ifname)
	}
	devNum, err := readSysfsUint(filepath.Join(usbDevDir, "devnum"), 10)
	if err != nil {
		return "", false, false, errors.Wrapf(err, "read USB devnum for %q", ifname)
	}
	return fmt.Sprintf("/dev/bus/usb/%03d/%03d", busNum, devNum), multicastOff, true, nil
}

func r8152ControlPLA(ctx context.Context, fd int, reqType uint8, index uint16, buf *[4]byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timeoutMS := uint32(r8152CtrlTimeoutMS)
	if deadline, ok := ctx.Deadline(); ok {
		rem := time.Until(deadline)
		if rem <= 0 {
			return context.DeadlineExceeded
		}
		if ms := rem.Milliseconds(); ms < r8152CtrlTimeoutMS {
			if ms < 1 {
				// In Linux usbfs, timeout=0 means infinite (MAX_SCHEDULE_TIMEOUT).
				ms = 1
			}
			timeoutMS = uint32(ms)
		}
	}
	ctrl := usbdevfsCtrlTransfer{
		bRequestType: reqType,
		bRequest:     r8152ReqRegs,
		wValue:       r8152PLARCR,
		wIndex:       index,
		wLength:      uint16(len(buf)),
		timeout:      timeoutMS,
		data:         unsafe.Pointer(&buf[0]),
	}
	n, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), usbdevfsControlIoctl, uintptr(unsafe.Pointer(&ctrl)))
	runtime.KeepAlive(buf)
	runtime.KeepAlive(&ctrl)
	if errno != 0 {
		return errors.Wrap(errno, "PLA_RCR control transfer")
	}
	if n != uintptr(len(buf)) {
		return errors.Errorf("short PLA_RCR transfer: %d bytes, want %d", n, len(buf))
	}
	return nil
}

func r8152ReadPLARCR(ctx context.Context, fd int) (uint32, error) {
	var buf [4]byte
	if err := r8152ControlPLA(ctx, fd, r8152ReqTypeRead, r8152MCUTypePLA, &buf); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

func r8152WritePLARCR(ctx context.Context, fd int, val uint32) error {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], val)
	return r8152ControlPLA(ctx, fd, r8152ReqTypeWrite, r8152MCUTypePLA|r8152ByteEnDword, &buf)
}

func readSysfsUint(path string, base int) (uint32, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), base, 32)
	return uint32(v), err
}
