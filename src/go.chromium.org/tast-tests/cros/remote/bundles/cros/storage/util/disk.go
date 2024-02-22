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
	// SDCard represents a removable SD card.
	SDCard
	// EmmcOverNvmeDisk represents an eMMC device that is exposed via an
	// eMMC-NVMe bridge (e.g. BH799)
	EmmcOverNvmeDisk
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
	case SDCard:
		return "SD card"
	case EmmcOverNvmeDisk:
		return "eMMC-NVMe"
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

// Disk structure represents a block device.
type Disk struct {
	DUT                *dut.DUT
	Path               string
	Name               string
	PhysicalDevicePath string
	PhysicalDeviceName string
	Size               int
	Type               DiskType

	nvmePowerConfig *NvmePowerConfig
}

func newDisk(ctx context.Context, dut *dut.DUT, path string, isPartition bool) (*Disk, error) {
	pathComponents := strings.Split(path, "/")
	name := pathComponents[len(pathComponents)-1]
	physDevName := name

	if isPartition {
		physDevName = physDevName[:len(physDevName)-2]
	}

	size, err := getBlockDeviceSize(ctx, dut, path)
	if err != nil {
		return nil, errors.Wrap(err, "can't get dev size")
	}

	disk := &Disk{
		DUT:                dut,
		Path:               path,
		Name:               name,
		PhysicalDevicePath: "/dev/" + physDevName,
		PhysicalDeviceName: physDevName,
		Size:               size,
		Type:               UnknownDisk,
	}

	if err := disk.detectType(ctx); err != nil {
		return nil, errors.Wrap(err, "can't detect disk type")
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

const bh799Ven = 0x1217
const bh799Dev = 0x0002

func (d *Disk) detectType(ctx context.Context) error {
	if strings.HasPrefix(d.PhysicalDeviceName, "mmcblk") {
		sysfsType, err := d.ReadSysfsString(ctx, "device/type")
		if err != nil {
			return errors.Wrap(err, "can't read mmc type value")
		}
		if sysfsType == "SD" {
			d.Type = SDCard
		} else if sysfsType == "MMC" {
			d.Type = EmmcDisk
		}
	} else if strings.HasPrefix(d.PhysicalDeviceName, "nvme") {
		subsysVen, err := d.ReadSysfsHexInt64(ctx, "device/device/subsystem_vendor")
		if err != nil {
			return errors.Wrap(err, "can't read `susbsystem_vendor` value")
		}
		subsysDev, err := d.ReadSysfsHexInt64(ctx, "device/device/subsystem_device")
		if err != nil {
			return errors.Wrap(err, "can't read `subsystem_device` value")
		}

		if subsysVen == bh799Ven && subsysDev == bh799Dev {
			d.Type = EmmcOverNvmeDisk
		} else {
			d.Type = NvmeDisk
		}
	} else if strings.HasPrefix(d.PhysicalDeviceName, "sd") {
		removable, err := d.ReadSysfsInt64(ctx, "removable")
		if err != nil {
			return errors.Wrap(err, "can't read `removable` value")
		}

		if removable == 0 {
			d.Type = UfsDisk
		} else {
			d.Type = UsbDisk
		}
	}

	// If we don't match anything, we leave Type field as is, but report no
	// error. The calling code will check if UnknownDisk is an appropriate
	// status.
	return nil
}

func (d *Disk) debugfsEntry() string {
	switch d.Type {
	case EmmcDisk:
		fallthrough
	case SDCard:
		return "mmc" + d.PhysicalDeviceName[len(d.PhysicalDeviceName)-1:]
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
	case SDCard:
		fallthrough
	case EmmcDisk:
		fallthrough
	case UfsDisk:
		return d.powerStateFromSysfs(ctx)
	default:
		return DiskUnknownPowerState, errors.New("Power State check not implemented")
	}
}

// IsInternal returns true if the disk is an internal storage.
func (d *Disk) IsInternal() bool {
	return d.Type != UnknownDisk && d.Type != UsbDisk && d.Type != SDCard
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
	path := "/sys/block/" + d.PhysicalDeviceName + "/" + relativePath
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

// GetExpectedTrimValueFunc returns comparison function for expected after-trim
// value of the device.
func (d *Disk) GetExpectedTrimValueFunc(ctx context.Context) (TrimComparisonFunc, error) {
	switch d.Type {
	case NvmeDisk:
		fallthrough
	case EmmcOverNvmeDisk:
		return ExpectedTrimValueNvme(ctx, d.DUT, d.PhysicalDevicePath)
	case EmmcDisk:
		return ExpectedTrimValueEmmc(ctx, d.DUT, d.PhysicalDevicePath)
	case UfsDisk:
		return ExpectedTrimValueUfs(ctx, d.DUT, d.PhysicalDevicePath)
	default:
		return nil, nil
	}
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

	var result []string
	for _, dev := range strings.Fields(sysBlockLs) {
		if strings.HasPrefix(dev, "zram") || strings.Contains(dev, "boot") {
			// Skip zram devices for they fail size check and are
			// not physical devices anyway.
			continue
		}
		result = append(result, dev)
	}

	return result, nil
}

// GetRemovableSD returns the disk structure representing a removable SD card.
func GetRemovableSD(ctx context.Context, dut *dut.DUT) (*Disk, error) {
	blockDevNames, err := getBlockDevList(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "can't list block devices")
	}

	for _, bdev := range blockDevNames {
		devPath := filepath.Join("/dev", bdev)
		disk, err := newDisk(ctx, dut, devPath, false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to process block device %q", devPath)
		}
		if disk.Type == SDCard {
			return disk, nil
		}
	}

	return nil, errors.New("can not detect SD card")
}

// GetStandbyRootfs returns disk structure representing the non-active rootfs
// partition of the internal storage device.
func GetStandbyRootfs(ctx context.Context, dut *dut.DUT) (*Disk, error) {
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

	disk, err := newDisk(ctx, dut, partitionPath, true)
	if err != nil {
		return nil, err
	}

	if !disk.IsInternal() {
		return nil, errors.New("stand-by rootfs can be used only on internal device")
	}

	return disk, nil
}

// GetInternalStorage returns the disk structure representing the internal
// storage device.
func GetInternalStorage(ctx context.Context, dut *dut.DUT) (*Disk, error) {
	blockDevNames, err := getBlockDevList(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "can't list block devices")
	}

	for _, bdev := range blockDevNames {
		devPath := filepath.Join("/dev", bdev)
		disk, err := newDisk(ctx, dut, devPath, false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to process block device %q", devPath)
		}
		if disk.IsInternal() {
			return disk, nil
		}
	}

	return nil, errors.New("can not detect internal storage device")
}
