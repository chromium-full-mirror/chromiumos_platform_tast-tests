// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbutils

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// USBDevice represents information of a connected USB device.
type USBDevice struct {
	// Class represents class that the connected device falls into. (Example: Mass storage, Wireless, etc).
	Class string
	// Driver represents driver that drives the connected device. (Example: hub, btusb, etc).
	Driver string
	// Speed represents the speed of connected device. (Example: 480M, 5000M, 1.5M, etc).
	Speed string
}

// re will parse Class, Driver and Speed of USB devices from 'lsusb -t' command output.
// Sample output of 'lsusb -t' command is as below:
/*
/:  Bus 04.Port 1: Dev 1, Class=root_hub, Driver=xhci_hcd/4p, 10000M
/:  Bus 03.Port 1: Dev 1, Class=root_hub, Driver=xhci_hcd/12p, 480M
    |__ Port 2: Dev 2, If 0, Class=Mass Storage, Driver=usb-storage, 5000M
    |__ Port 2: Dev 2, If 0, Class=Vendor Specific Class, Driver=asix, 480M
    |__ Port 5: Dev 3, If 0, Class=Video, Driver=uvcvideo, 480M
    |__ Port 5: Dev 3, If 1, Class=Video, Driver=uvcvideo, 480M
    |__ Port 10: Dev 4, If 0, Class=Wireless, Driver=btusb, 12M
    |__ Port 10: Dev 4, If 1, Class=Wireless, Driver=btusb, 12M
/:  Bus 02.Port 1: Dev 1, Class=root_hub, Driver=xhci_hcd/4p, 10000M
/:  Bus 01.Port 1: Dev 1, Class=root_hub, Driver=xhci_hcd/1p, 480M
*/
var re = regexp.MustCompile(`Class=([a-zA-Z_\s]+), Driver=([a-zA-Z0-9_\-\/\s]+), ([a-zA-Z0-9_\/.]+)`)

// ListDevicesInfo returns the class, driver and speed for all the USB devices.
//
// For local-side dut parameter must be nil.
// Example:
// usbDeviceInfo, err := usbutils.ListDevicesInfo(ctx, nil){
// ...
//
// For remote-side dut parameter must be non-nil.
// Example:
// dut := s.DUT()
// usbDeviceInfo, err := usbutils.ListDevicesInfo(ctx, dut){
// ...
func ListDevicesInfo(ctx context.Context, dut *dut.DUT) ([]USBDevice, error) {
	var out []byte
	var err error
	if dut != nil {
		out, err = dut.Conn().CommandContext(ctx, "lsusb", "-t").Output()
	} else {
		out, err = testexec.CommandContext(ctx, "lsusb", "-t").Output()
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to run lsusb command")
	}
	lsusbOut := string(out)
	var res []USBDevice
	sc := bufio.NewScanner(strings.NewReader(lsusbOut))
	for sc.Scan() {
		match := re.FindStringSubmatch(sc.Text())
		if match == nil {
			continue
		}
		res = append(res, USBDevice{
			Class:  match[1],
			Driver: match[2],
			Speed:  match[3],
		})
	}
	return res, nil
}

// NumberOfUSBDevicesConnected returns number of all USB devices connected with given devClassName, usbSpeed.
func NumberOfUSBDevicesConnected(deviceInfoList []USBDevice, devClassName, usbSpeed string) int {
	var speedSlice []string
	for _, dev := range deviceInfoList {
		if dev.Class == devClassName {
			devSpeed := dev.Speed
			if devSpeed == usbSpeed {
				speedSlice = append(speedSlice, devSpeed)
			}
		}
	}
	numberOfDevicesConnected := len(speedSlice)
	return numberOfDevicesConnected
}

// calculateHashOfFile checks the checksum for the input file.
func calculateHashOfFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return []byte{}, errors.Wrap(err, "failed to open files")
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return []byte{}, errors.Wrap(err, "failed to calculate the hash of the files")
	}

	return h.Sum(nil), nil
}

// StoragePath returns storage path of the given device name.
func StoragePath(ctx context.Context, deviceName string) (string, error) {
	const mediaRemovable = "/media/removable/"
	var storageFullPath string

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		fileInfos, err := ioutil.ReadDir(mediaRemovable)
		if err != nil {
			return errors.Wrap(err, "failed to read contents of media removable directory")
		}

		found := false
		for _, fileInfo := range fileInfos {
			if strings.Contains(fileInfo.Name(), deviceName) {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("failed to find device %s", deviceName)
		}
		storageFullPath = filepath.Join(mediaRemovable, deviceName)
		return nil
	}, &testing.PollOptions{Timeout: 45 * time.Second, Interval: 15 * time.Second}); err != nil {
		return "", errors.Wrap(err, "failed to find storage device")
	}
	return storageFullPath, nil
}

// TransferFile performs copying the file from source to destination
// and validates it by comparing the hash of both the files. If the
// bidirectional param is set to true, it copies the data from destination to source and validates the hash again.
func TransferFile(ctx context.Context, src, dst string, bidirection bool) error {
	localHash, err := calculateHashOfFile(src)
	if err != nil {
		return errors.Wrap(err, "failed to calculate hash of the source file")
	}

	testing.ContextLogf(ctx, "Transferring file from %s to %s", src, dst)
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return errors.Wrap(err, "failed to get file info")
	}
	if !sourceFileStat.Mode().IsRegular() {
		return errors.Errorf("%s is not a regular file", src)
	}
	source, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "failed to open file")
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return errors.Wrap(err, "failed to copy")
	}
	destHash, err := calculateHashOfFile(dst)
	if err != nil {
		return errors.Wrap(err, "failed to calculate hash of the destination file")
	}
	if !bytes.Equal(localHash, destHash) {
		return errors.Wrapf(err, "the hash doesn't match (destHash path: %q)", destHash)
	}

	if bidirection {
		testing.ContextLogf(ctx, "Transferring file from %s to %s", dst, src)
		destFileStat, err := os.Stat(dst)
		if err != nil {
			return errors.Wrap(err, "failed to get file info")
		}
		if !destFileStat.Mode().IsRegular() {
			return errors.Errorf("%s is not a regular file", dst)
		}
		destination, err := os.Open(dst)
		if err != nil {
			return errors.Wrap(err, "failed to open file")
		}
		defer destination.Close()

		source, err := os.Create(src)
		if err != nil {
			return errors.Wrap(err, "failed to create file")
		}
		defer source.Close()

		if _, err := io.Copy(source, destination); err != nil {
			return errors.Wrap(err, "failed to copy")
		}
		locHash, err := calculateHashOfFile(src)
		if err != nil {
			return errors.Wrap(err, "failed to calculate hash of the destination file")
		}
		if !bytes.Equal(locHash, destHash) {
			return errors.Wrapf(err, "the hash doesn't match (destHash path: %q)", destHash)
		}
	}
	return nil
}
