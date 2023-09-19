// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils contains functionality shared by tests that
// exercise firmware.
package utils

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const (
	// firmwareFileName contains the name of the file to be downloaded from chromeos-image-archive.
	firmwareFileName = "firmware_from_source.tar.bz2"
)

// VerifyFwIDs will show in logs the current firmware version and compare it to expected ones if they are provided.
func VerifyFwIDs(ctx context.Context, h *firmware.Helper, exROVersion, exRWVersion string) error {
	currentROID, err := GetFwVersion(ctx, h, reporters.CrossystemParamRoFwid)
	if err != nil {
		return err
	}
	if exROVersion != currentROID && !strings.Contains(exROVersion, currentROID) {
		return errors.Errorf("got %s RO version, but expected %s", currentROID, exROVersion)
	}
	currentRWID, err := GetFwVersion(ctx, h, reporters.CrossystemParamFwid)
	if err != nil {
		return err
	}
	if exRWVersion != currentRWID && !strings.Contains(exRWVersion, currentRWID) {
		return errors.Errorf("got %s RW version, but expected %s", currentRWID, exRWVersion)
	}
	return nil
}

// GetFwVersion accepts 'crossystem' params (i.e., CrossystemParamFwid & CrossystemParamRoFwid),
// splits the outputs from them and only returns the version numbers.
func GetFwVersion(ctx context.Context, h *firmware.Helper, param reporters.CrossystemParam) (string, error) {
	fwid, err := h.Reporter.CrossystemParam(ctx, param)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get only the fw id from crossystem: %v", param)
	}
	splitout := strings.Split(fwid, ".")
	if len(splitout) < 4 {
		return "", errors.Wrapf(err, "got invalid fw id from crossystem: %v", fwid)
	}
	onlyID := splitout[1] + "." + splitout[2] + "." + splitout[3]
	return onlyID, err
}

// DownloadFirmwareFile will download a tar file from cloud and save to a temporary directory,
// based on the shipped firmware version passed in for test.
func DownloadFirmwareFile(ctx context.Context, s *testing.State, tmpDir, firmwareFilePath string) error {
	testing.ContextLogf(ctx, "Downloading firmware image from the path: %s", firmwareFilePath)

	// Stage the complete path.
	r, err := s.CloudStorage().Open(ctx, fmt.Sprintf("gs://%s", firmwareFilePath))
	if err != nil {
		return errors.Wrapf(err, "failed to stage file for url %q", firmwareFilePath)
	}

	// Open tmp file
	fo, err := os.Create(tmpDir + "/" + firmwareFileName)
	if err != nil {
		return errors.Wrapf(err, "failed to open tmp file %q", tmpDir+"/"+firmwareFileName)
	}
	// Close file on exit
	defer func() error {
		if err := fo.Close(); err != nil {
			return errors.Wrapf(err, "failed to close tmp file %q", tmpDir+"/"+firmwareFileName)
		}
		return nil
	}()
	w := bufio.NewWriter(fo)
	if err != nil {
		return errors.Wrapf(err, "failed to stage file for url %q", firmwareFilePath)
	}
	written, err := io.Copy(w, r)
	if err != nil {
		return errors.Wrap(err, "failed to download the file")
	}
	testing.ContextLogf(ctx, "Downloaded stats for %s: %d", fo.Name(), written)
	return nil
}

// UntarUnknownFileName will try to untar the respective fw bin file from the downloaded tar file.
func UntarUnknownFileName(ctx context.Context, tmpDir, fwidModel string) (string, error) {
	// List of possible formats for the binary file found in a downloaded tar file.
	filenamePool := []string{fmt.Sprintf("image-%s.bin", fwidModel), fmt.Sprintf("./image-%s.bin", fwidModel), "image.bin"}
	var err error
	for _, filename := range filenamePool {
		if err = testexec.CommandContext(ctx, "tar", "-xvf", tmpDir+"/"+firmwareFileName, "-C", tmpDir, filename).Run(ssh.DumpLogOnError); err != nil {
			testing.ContextLogf(ctx, "WARNING! failed to untar the image with the name %q: %v", filename, err)
			continue
		}
		return filename, nil
	}
	return "", errors.Wrap(err, "failed to untar fw bin file from the downloaded tar file")
}
