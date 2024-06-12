// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"bufio"
	"bytes"
	"context"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/avl"
	"go.chromium.org/tast/core/errors"
	"golang.org/x/exp/maps"
)

const storageInfoPath = "/mnt/stateful_partition/encrypted/var/log/storage_info.txt"

const storageComponentType = "storage"

// WriteAVLInfo fetches AVL info (model, firmware) from the disk and writes it
// to the provided output directory.
// Only eMMC, NVMe, and UFS disks are supported: others will return an error.
func WriteAVLInfo(ctx context.Context, disk *Disk, outDir string) error {
	storageInfo, err := RunCmdWithOutput(ctx, disk.DUT, "cat", storageInfoPath)
	if err != nil {
		return errors.Wrap(err, "could not read storage info from the device")
	}
	avlInfo, err := avlInfoFromStorageInfo(storageInfo, disk.Type)
	if err != nil {
		return err
	}
	return avl.SaveInfo(ctx, outDir, avlInfo)
}

func avlInfoFromStorageInfo(storageInfo []byte, diskType DiskType) (avl.Info, error) {
	var patterns map[string]*regexp.Regexp
	var format func(map[string]string) avl.Info
	switch diskType {
	case EmmcDisk:
		patterns = map[string]*regexp.Regexp{
			"manfid":   regexp.MustCompile(`\s*manfid\s+\|\s(?P<manfid>\S+).*`),
			"name":     regexp.MustCompile(`\s*name\s+\|\s(?P<param>\S+).*`),
			"firmware": regexp.MustCompile(`\s*fwrev\s+\|\s(?P<firmware>\S+).*`),
		}
		format = func(matched map[string]string) avl.Info {
			return avl.Info{
				PartModel:     matched["manfid"] + " " + matched["name"],
				PartFirmware:  matched["firmware"],
				ComponentType: storageComponentType,
			}
		}
	case EmmcOverNvmeDisk:
		fallthrough
	case NvmeDisk:
		patterns = map[string]*regexp.Regexp{
			"model":    regexp.MustCompile(`\s*Model Number:\s+(?P<model>.+)\s*`),
			"firmware": regexp.MustCompile(`\s*Firmware Version:\s+(?P<firmware>\S+).*`),
		}
		format = func(matched map[string]string) avl.Info {
			return avl.Info{
				PartModel:     matched["model"],
				PartFirmware:  matched["firmware"],
				ComponentType: storageComponentType,
			}
		}
	case UfsDisk:
		patterns = map[string]*regexp.Regexp{
			"model":    regexp.MustCompile(`^Model: (?P<param>\S+)`),
			"vendor":   regexp.MustCompile(`^Vendor: (?P<vendor>\S+)`),
			"firmware": regexp.MustCompile(`^Firmware: (?P<firmware>\S+)`),
		}
		format = func(matched map[string]string) avl.Info {
			return avl.Info{
				PartModel:     matched["vendor"] + " " + matched["model"],
				PartFirmware:  matched["firmware"],
				ComponentType: storageComponentType,
			}
		}
	default:
		return avl.Info{}, errors.Errorf("cannot generate AVL info for disks of type %q", DiskTypeToString(diskType))
	}
	values, err := getValues(storageInfo, patterns)
	if err != nil {
		return avl.Info{}, err
	}
	return format(values), nil
}

func getValues(storageInfo []byte, patterns map[string]*regexp.Regexp) (map[string]string, error) {
	out := make(map[string]string, len(patterns))
	remainingPatterns := maps.Clone(patterns)
	scanner := bufio.NewScanner(bytes.NewReader(storageInfo))
	for scanner.Scan() {
		line := scanner.Text()
		var keysToDelete []string
		for name, pattern := range remainingPatterns {
			match := pattern.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			out[name] = match[1]
			keysToDelete = append(keysToDelete, name)
		}
		for _, key := range keysToDelete {
			delete(remainingPatterns, key)
		}
		if len(remainingPatterns) == 0 {
			return out, nil
		}
	}
	var errs []error
	for key := range remainingPatterns {
		errs = append(errs, errors.Errorf("no value found for key %q in storage_info.txt", key))
	}
	return nil, errors.Join(errs...)
}
