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

// Disk structure represents a block device.
type Disk struct {
	Path string
	Name string
	Size int
	Type DiskType
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

// ReadDebugfsString reads string from the debugfs of the device.
func (d *Disk) ReadDebugfsString(ctx context.Context, dut *dut.DUT, relativePath string) (string, error) {
	path := "/sys/kernel/debug/" + d.debugfsEntry() + "/" + relativePath
	data, err := RunCmdWithStringOutput(ctx, dut, "cat", path)
	if err != nil {
		return "", errors.Wrap(err, "failed to read sysfs path: "+path)
	}
	return data, nil
}

// ReadDebugfsHexInt64 reads hex int from the debugfs of the device.
func (d *Disk) ReadDebugfsHexInt64(ctx context.Context, dut *dut.DUT, relativePath string) (int64, error) {
	strVal, err := d.ReadDebugfsString(ctx, dut, relativePath)
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
func (d *Disk) ReadSysfsString(ctx context.Context, dut *dut.DUT, relativePath string) (string, error) {
	path := "/sys/block/" + d.Name + "/" + relativePath
	data, err := RunCmdWithStringOutput(ctx, dut, "cat", path)
	if err != nil {
		return "", errors.Wrap(err, "failed to read sysfs path: "+path)
	}
	return data, nil
}

// ReadSysfsHexInt64 reads hex int from the sysfs of the device.
func (d *Disk) ReadSysfsHexInt64(ctx context.Context, dut *dut.DUT, relativePath string) (int64, error) {
	strVal, err := d.ReadSysfsString(ctx, dut, relativePath)
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
func (d *Disk) ReadSysfsInt64(ctx context.Context, dut *dut.DUT, relativePath string) (int64, error) {
	strVal, err := d.ReadSysfsString(ctx, dut, relativePath)
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

// GetRemovableMmc returns the disk structure representing the removable MMC
// TODO(dlunev): This can pick up wrong device. It will get fixed, but ok for
// now to start working on tests.
func GetRemovableMmc(ctx context.Context, dut *dut.DUT) (*Disk, error) {
	mmcName, err := getRemovableMmcName(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "can't find mmc card")
	}

	mmcPath := filepath.Join("/dev", mmcName)
	mmcSize, err := getBlockDeviceSize(ctx, dut, mmcPath)
	if err != nil {
		return nil, errors.Wrap(err, "can't get mmc card size")
	}

	return &Disk{Path: mmcPath, Name: mmcName, Size: mmcSize, Type: MmcDisk}, nil
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
	rootDevSize, err := getBlockDeviceSize(ctx, dut, rootDevPath)
	if err != nil {
		return nil, errors.Wrap(err, "can't get rootdev size")
	}

	return &Disk{
		Path: rootDevPath,
		Name: rootDevName,
		Size: rootDevSize,
		Type: GetDiskType(rootDevName)}, nil
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
	partitionSize, err := getBlockDeviceSize(ctx, dut, partitionPath)
	if err != nil {
		return nil, errors.Wrap(err, "can't get standby rootfs partition size")
	}

	return &Disk{
		Path: partitionPath,
		Name: partitionName,
		Size: partitionSize,
		Type: GetDiskType(partitionName)}, nil
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
	var internalDevType DiskType

	internalDevType = UnknownDisk

	for _, bdev := range blockDevNames {
		if bdev != rootDevName && GetDiskType(bdev) != UnknownDisk {
			internalDevName = bdev
			break
		}

	}

	internalDevPath := filepath.Join("/dev", internalDevName)
	internalDevSize, err := getBlockDeviceSize(ctx, dut, internalDevPath)
	if err != nil {
		return nil, errors.Wrap(err, "can't get rootdev size")
	}

	return &Disk{
		Path: internalDevPath,
		Name: internalDevName,
		Size: internalDevSize,
		Type: internalDevType}, nil
}
