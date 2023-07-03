// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

// DiskType represents the type of the block device.
type DiskType int64

// TODO(dlunev): figure out how to detect bridges
const (
	// UnknownDisk represents a missing or unrecognized block device.
	UnknownDisk = iota
	// EmmcDisk represents an internal eMMC storage.
	EmmcDisk
	// NVMeDisk represents an internal NVMe storage.
	NvmeDisk
	// UfsDisk represents an internal NVMe storage.
	UfsDisk
	// SataDisk represents an internal SATA storage.
	SataDisk
	// UsbDisk represents a USB storage.
	UsbDisk
	// MmcDisk represents a removable mmc card.
	MmcDisk
)

// DiskPowerState represents disk's power state.
type DiskPowerState int64

const (
	//DiskUnknownPowerState represents unknown power state
	DiskUnknownPowerState = iota
	// DiskHighPowerState represents high power state
	DiskHighPowerState
	// DiskLowPowerState represents high power state
	DiskLowPowerState
)

// DiskTypeToString returns string name of a disk type.
func DiskTypeToString(t DiskType) string {
	switch t {
	case UnknownDisk:
		return "unknown"
	case EmmcDisk:
		return "eMMC"
	case NvmeDisk:
		return "NVMe"
	case UfsDisk:
		return "UFS"
	case SataDisk:
		return "SATA"
	case UsbDisk:
		return "usb-storage"
	case MmcDisk:
		return "SD card"
	}
	return "invalid enum type"
}

func getBlockDeviceSize(ctx context.Context, dut *dut.DUT, devPath string) (int, error) {
	blockCountStr, err := RunCmdWithStringOutput(ctx, dut, "blockdev", "--getsz", devPath)
	if err != nil {
		return -1, errors.Wrapf(err, "can't get block count for %q", devPath)
	}

	blockCount, err := strconv.Atoi(blockCountStr)
	if err != nil {
		return -1, errors.Wrapf(err, "can't parse int from %q", blockCountStr)
	}

	return blockCount * 512, nil
}

// GetDiskType returns disk type based on its name.
// TODO(dlunev): this is a crude stub, a more reliable method should be developed.
func GetDiskType(devName string) DiskType {
	if strings.HasPrefix(devName, "mmcblk") {
		return EmmcDisk
	}
	if strings.HasPrefix(devName, "nvme") {
		return NvmeDisk
	}
	if strings.HasPrefix(devName, "sd") {
		return UfsDisk
	}

	return UnknownDisk
}

// Disk structure represents a block device.
type Disk struct {
	DUT  *dut.DUT
	Path string
	Name string
	Size int
	Type DiskType

	nvmePowerConfig *NvmePowerConfig
}

func newDisk(ctx context.Context, dut *dut.DUT, path string) (*Disk, error) {
	pathComponents := strings.Split(path, "/")
	name := pathComponents[len(pathComponents)-1]

	size, err := getBlockDeviceSize(ctx, dut, path)
	if err != nil {
		return nil, errors.Wrap(err, "can't get dev size")
	}

	disk := &Disk{
		DUT:  dut,
		Path: path,
		Name: name,
		Size: size,
		Type: GetDiskType(name),
	}

	if disk.Type == NvmeDisk {
		powerConfig, err := readNvmePowerConfig(ctx, disk)
		if err != nil {
			return nil, errors.Wrap(err, "can't get nvme power config")
		}
		disk.nvmePowerConfig = powerConfig
	}

	return disk, nil
}

func (d *Disk) debugfsEntry() string {
	switch d.Type {
	case EmmcDisk:
		fallthrough
	case MmcDisk:
		return "mmc" + d.Name[len(d.Name)-1:]
	default:
		return "not_implemented_debugfs_entry"
	}
}

func (d *Disk) powerStateFromSysfs(ctx context.Context) (DiskPowerState, error) {
	state, err := d.ReadSysfsString(ctx, "device/power/runtime_status")
	if err != nil {
		return DiskUnknownPowerState, errors.Wrap(err, "can't get power state from sysfs")
	}

	if state == "suspended" {
		return DiskLowPowerState, nil
	}
	if state == "active" {
		return DiskHighPowerState, nil
	}
	return DiskUnknownPowerState, errors.Errorf("unknown sysfs power state %q", state)
}

// CurrentPowerState returns device's current power state
func (d *Disk) CurrentPowerState(ctx context.Context) (DiskPowerState, error) {
	switch d.Type {
	case NvmeDisk:
		return getNvmePowerState(ctx, d)
	case MmcDisk:
		fallthrough
	case EmmcDisk:
		fallthrough
	case UfsDisk:
		return d.powerStateFromSysfs(ctx)
	default:
		return DiskUnknownPowerState, errors.New("Power State check not implemented")
	}
}

// ReadDebugfsString reads string from the debugfs of the device.
func (d *Disk) ReadDebugfsString(ctx context.Context, relativePath string) (string, error) {
	path := "/sys/kernel/debug/" + d.debugfsEntry() + "/" + relativePath
	data, err := RunCmdWithStringOutput(ctx, d.DUT, "cat", path)
	if err != nil {
		return "", errors.Wrap(err, "failed to read debugfs path: "+path)
	}
	return data, nil
}

// ReadDebugfsHexInt64 reads hex int from the debugfs of the device.
func (d *Disk) ReadDebugfsHexInt64(ctx context.Context, relativePath string) (int64, error) {
	strVal, err := d.ReadDebugfsString(ctx, relativePath)
	if err != nil {
		return 0, err
	}
	if strVal[0:2] == "0x" {
		strVal = strVal[2:]
	}
	value, err := strconv.ParseInt(strVal, 16, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse %q as int64 from %q", strVal, relativePath)
	}
	return value, nil
}

// ReadSysfsString reads string from the sysfs of the device.
func (d *Disk) ReadSysfsString(ctx context.Context, relativePath string) (string, error) {
	path := "/sys/block/" + d.Name + "/" + relativePath
	data, err := RunCmdWithStringOutput(ctx, d.DUT, "cat", path)
	if err != nil {
		return "", errors.Wrap(err, "failed to read sysfs path: "+path)
	}
	return data, nil
}

// ReadSysfsHexInt64 reads hex int from the sysfs of the device.
func (d *Disk) ReadSysfsHexInt64(ctx context.Context, relativePath string) (int64, error) {
	strVal, err := d.ReadSysfsString(ctx, relativePath)
	if err != nil {
		return 0, err
	}
	if strVal[0:2] == "0x" {
		strVal = strVal[2:]
	}
	value, err := strconv.ParseInt(strVal, 16, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse %q as int64 from %q", strVal, relativePath)
	}
	return value, nil
}

// ReadSysfsInt64 reads decimal int from the sysfs of the device.
func (d *Disk) ReadSysfsInt64(ctx context.Context, relativePath string) (int64, error) {
	strVal, err := d.ReadSysfsString(ctx, relativePath)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(strVal, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse %q as int64 from %q", strVal, relativePath)
	}
	return value, nil
}

func getRootDevName(ctx context.Context, dut *dut.DUT) (string, error) {
	rootDevPath, err := RunCmdWithStringOutput(ctx, dut, "rootdev", "-s", "-d")
	if err != nil {
		return "", errors.Wrap(err, "failed to query rootdev")
	}

	rootDevPathComponents := strings.Split(rootDevPath, "/")
	if len(rootDevPathComponents) != 3 {
		return "", errors.Errorf("malformed rootdev path: %q", rootDevPath)
	}

	return rootDevPathComponents[2], nil
}

func getRootDevPartition(ctx context.Context, dut *dut.DUT) (string, error) {
	rootDevPath, err := RunCmdWithStringOutput(ctx, dut, "rootdev", "-s")
	if err != nil {
		return "", errors.Wrap(err, "failed to query rootdev")
	}

	rootDevPathComponents := strings.Split(rootDevPath, "/")
	if len(rootDevPathComponents) != 3 {
		return "", errors.Errorf("malformed rootdev path: %q", rootDevPath)
	}

	return rootDevPathComponents[2], nil
}

func getBlockDevList(ctx context.Context, dut *dut.DUT) ([]string, error) {
	sysBlockLs, err := RunCmdWithStringOutput(ctx, dut, "ls", "/sys/block")
	if err != nil {
		return nil, errors.Wrap(err, "failed to list block devices")
	}

	return strings.Fields(sysBlockLs), nil
}

func getRemovableMmcName(ctx context.Context, dut *dut.DUT) (string, error) {
	rootDevName, err := getRootDevName(ctx, dut)
	if err != nil {
		return "", errors.Wrap(err, "can't get rootdev")
	}

	blockDevNames, err := getBlockDevList(ctx, dut)
	if err != nil {
		return "", errors.Wrap(err, "can't list block devices")
	}

	for _, bdev := range blockDevNames {
		if bdev == rootDevName {
			continue
		}

		if strings.HasPrefix(bdev, "mmcblk") {
			return bdev, nil
		}
	}

	return "", errors.New("can't find mmc card")
}

// GetRemovableMmc returns the disk structure representing the removable MMC
// TODO(dlunev): This can pick up wrong device. It will get fixed, but ok for
// now to start working on tests.
func GetRemovableMmc(ctx context.Context, dut *dut.DUT) (*Disk, error) {
	mmcName, err := getRemovableMmcName(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "can't find mmc card")
	}

	mmcPath := filepath.Join("/dev", mmcName)

	return newDisk(ctx, dut, mmcPath)
}

// GetInternalStorageFromInternalBoot returns the disk structure representing
// the internal device for the system boot from the device.
// TODO(dlunev): This can pick up wrong device. It will get fixed, but ok for
// now to start working on tests.
func GetInternalStorageFromInternalBoot(ctx context.Context, dut *dut.DUT) (*Disk, error) {
	rootDevName, err := getRootDevName(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "can't get rootdev")
	}

	rootDevPath := filepath.Join("/dev", rootDevName)

	return newDisk(ctx, dut, rootDevPath)
}

// GetStandbyRootfsFromInternalBoot returns disk structure representing
// the non-active rootfs partition for the system booth from the device.
// TODO(dlunev): This can pick up wrong device. It will get fixed, but ok for
// now to start working on tests.
func GetStandbyRootfsFromInternalBoot(ctx context.Context, dut *dut.DUT) (*Disk, error) {
	partitionName, err := getRootDevPartition(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "can't get rootfs partition")
	}

	partitionIndex := partitionName[len(partitionName)-1:]
	if partitionIndex != "3" && partitionIndex != "5" {
		return nil, errors.Errorf("invalid index of root parition: %s", partitionIndex)
	}
	spareRootMap := map[string]string{"3": "5", "5": "3"}
	partitionName = partitionName[:len(partitionName)-1]
	partitionName = partitionName + spareRootMap[partitionIndex]

	partitionPath := filepath.Join("/dev", partitionName)

	return newDisk(ctx, dut, partitionPath)
}

// GetInternalStorageFromRemovableBoot returns the disk structure representing
// the internal device for the system boot from the removable storage.
// TODO(dlunev): This can pick up wrong device. It will get fixed, but ok for
// now to start working on tests.
func GetInternalStorageFromRemovableBoot(ctx context.Context, dut *dut.DUT) (*Disk, error) {
	rootDevName, err := getRootDevName(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "can't get rootdev")
	}

	blockDevNames, err := getBlockDevList(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "can't list block devices")
	}

	var internalDevName string
	for _, bdev := range blockDevNames {
		if bdev != rootDevName && GetDiskType(bdev) != UnknownDisk {
			internalDevName = bdev
			break
		}

	}

	internalDevPath := filepath.Join("/dev", internalDevName)

	return newDisk(ctx, dut, internalDevPath)
}
