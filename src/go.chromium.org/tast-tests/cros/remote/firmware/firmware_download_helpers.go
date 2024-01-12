// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// FWType represents the type of firmware we are working on - AP or EC
type FWType string

// FWFilesToFlash represents the name of firmware bin to flash
type FWFilesToFlash struct {
	ECFirmwareFile string
	APFirmwareFile string
	MonitorFile    string
}

var (
	// FirmwarePath is the GCS location for the firmware to be downloaded
	FirmwarePath = testing.RegisterVarString(
		"firmware.firmwarePath",
		"",
		"A variable to store the path information for the fw download")

	// LocalFirmwarePath is the local file location for the downloaded firmware
	LocalFirmwarePath = testing.RegisterVarString(
		"firmware.localFirmwarePath",
		"",
		"A variable to store the local path information for the download firmware")
)

const (
	// FirmwareFileName contains the name of the file to be downloaded from chromeos-image-archive.
	FirmwareFileName = "firmware_from_source.tar.bz2"
	// ECFirmware indicates firmware for EC
	ECFirmware FWType = "EC"
	// APFirmware indicates firmware for AP
	APFirmware FWType = "AP"
	// ECFirmwareFileToFlash is the name of the EC firmware bin to flash
	ECFirmwareFileToFlash string = "ecFirmwareForTest.bin"
	// APFirmwareFileToFlash is the name of the AP firmware bin to flash
	APFirmwareFileToFlash string = "FirmwareForTest.bin"
	// MonitorFileToFlash is the name of the Monitor bin to flash
	MonitorFileToFlash string = "npcx_monitor.bin"
)

// VerifyFwIDs will show in logs the current firmware version and compare it to expected ones if they are provided.
func VerifyFwIDs(ctx context.Context, h *Helper, exROVersion, exRWVersion string) error {
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
func GetFwVersion(ctx context.Context, h *Helper, param reporters.CrossystemParam) (string, error) {
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
func DownloadFirmwareFile(ctx context.Context, cs *testing.CloudStorage, tmpDir, gcsFirmwareFilePath string) error {
	testing.ContextLogf(ctx, "Downloading firmware image from the path: %s", gcsFirmwareFilePath)

	// Stage the complete path.
	r, err := cs.Open(ctx, fmt.Sprintf("gs://%s", gcsFirmwareFilePath))
	if err != nil {
		return errors.Wrapf(err, "failed to stage file for url %q", gcsFirmwareFilePath)
	}

	// Open tmp file
	fo, err := os.Create(tmpDir + "/" + FirmwareFileName)
	if err != nil {
		return errors.Wrapf(err, "failed to open tmp file %q", tmpDir+"/"+FirmwareFileName)
	}
	// Close file on exit
	defer func() error {
		if err := fo.Close(); err != nil {
			return errors.Wrapf(err, "failed to close tmp file %q", tmpDir+"/"+FirmwareFileName)
		}
		return nil
	}()
	w := bufio.NewWriter(fo)
	if err != nil {
		return errors.Wrapf(err, "failed to stage file for url %q", gcsFirmwareFilePath)
	}
	written, err := io.Copy(w, r)
	if err != nil {
		return errors.Wrap(err, "failed to download the file")
	}
	testing.ContextLogf(ctx, "Downloaded stats for %s: %d", fo.Name(), written)
	return nil
}

// DownloadRequiredFirmwareFiles will extract and download the specified AP and EC .bin files from the firmware tar in the cloud storage
func DownloadRequiredFirmwareFiles(ctx context.Context, h *Helper, cs *testing.CloudStorage, gcsFirmwareFilePath, servoTmpDir, fwidModel string) (*FWFilesToFlash, error) {
	ecFilenamePool, ecMonitorFileNamePool := getFileNamePools(ctx, fwidModel, ECFirmware)
	apFileNamePool, _ := getFileNamePools(ctx, fwidModel, APFirmware)
	var apBin, ecBin, monitorBin string

	// Find a devserver that works from servo host, and download image from there.
	for _, devserver := range cs.Devservers() {
		testing.ContextLogf(ctx, "Trying devserver at %q", devserver)
		if err := h.ServoProxy.RunCommand(ctx, false, "curl", "--connect-timeout", "3", fmt.Sprintf("%s/check_health", devserver)); err != nil {
			testing.ContextLog(ctx, "Devserver not healthy: ", err)
			continue
		}
		artifactsURL := strings.TrimSuffix(cs.BuildArtifactsURL(), "/")
		stagingURL := fmt.Sprintf("%s/stage?archive_url=%s&files=%s", devserver, artifactsURL, gcsFirmwareFilePath)
		testing.ContextLogf(ctx, "Staging image %q", stagingURL)
		if err := h.ServoProxy.RunCommand(ctx, false, "curl", stagingURL); err != nil {
			testing.ContextLogf(ctx, "Failed to stage file at %q: %v", stagingURL, err)
			continue
		}
		testing.ContextLogf(ctx, "Successfully staged from %q", stagingURL)
		monitorBin = extractFirmwareFile(ctx, h, devserver, gcsFirmwareFilePath, servoTmpDir, MonitorFileToFlash, ecMonitorFileNamePool)
		apBin = extractFirmwareFile(ctx, h, devserver, gcsFirmwareFilePath, servoTmpDir, APFirmwareFileToFlash, apFileNamePool)
		ecBin = extractFirmwareFile(ctx, h, devserver, gcsFirmwareFilePath, servoTmpDir, ECFirmwareFileToFlash, ecFilenamePool)
		// Extracted all the required files from this devserver
		return &FWFilesToFlash{ECFirmwareFile: ecBin, MonitorFile: monitorBin, APFirmwareFile: apBin}, nil
	}
	return nil, errors.New("no devservers able to stage firmware image")
}

// UntarUnknownFileName will try to untar the respective fw bin file from the downloaded tar file.
func UntarUnknownFileName(ctx context.Context, tmpDir, fwidModel string, fwType FWType) (string, string, error) {
	// List of possible formats for the binary file found in a downloaded tar file.
	const ecMonitorFileName = "npcx_monitor.bin"
	ecMonitorFile := ""
	filenamePool, ecMonitorFileNamePool := getFileNamePools(ctx, fwidModel, fwType)
	if fwType == ECFirmware {
		// Extract subsidiary binaries for EC
		// Find a monitor binary for NPCX_UUT chip type, if any.
		for _, f := range ecMonitorFileNamePool {
			if err := testexec.CommandContext(ctx, "tar", "-xvf", tmpDir+"/"+FirmwareFileName, "-C", tmpDir, f).Run(ssh.DumpLogOnError); err != nil {
				testing.ContextLogf(ctx, "WARNING! failed to untar the image with the name %q: %v", f, err)
				continue
			}
			ecMonitorFile = f
			testing.ContextLogf(ctx, "Found monitor image with the name %q", f)
			break
		}
	}
	var err error
	for _, filename := range filenamePool {
		if err = testexec.CommandContext(ctx, "tar", "-xvf", tmpDir+"/"+FirmwareFileName, "-C", tmpDir, filename).Run(ssh.DumpLogOnError); err != nil {
			testing.ContextLogf(ctx, "WARNING! failed to untar the image with the name %q: %v", filename, err)
			continue
		}
		return filename, ecMonitorFile, nil
	}
	return "", "", errors.Wrap(err, "failed to untar fw bin file from the downloaded tar file")
}

// getFileNamePools gets the possible file name pools based on the type of firmware
func getFileNamePools(ctx context.Context, fwidModel string, fwType FWType) ([]string, []string) {
	// List of possible formats for the binary file found in a downloaded tar file.
	const ecMonitorFileName = "npcx_monitor.bin"
	var filenamePool []string
	var monitorFileNamePool []string
	if fwType == APFirmware {
		filenamePool = []string{fmt.Sprintf("image-%s.bin", fwidModel), fmt.Sprintf("./image-%s.bin", fwidModel), "image.bin"}
	} else if fwType == ECFirmware {
		filenamePool = []string{fmt.Sprintf("%s/ec.bin", fwidModel), fmt.Sprintf("./%s/ec.bin", fwidModel)}
		// Extract subsidiary binaries for EC
		// Find a monitor binary for NPCX_UUT chip type, if any.
		for _, f := range filenamePool {
			monitorFile := strings.Replace(f, "ec.bin", ecMonitorFileName, 1)
			monitorFileNamePool = append(monitorFileNamePool, monitorFile)
		}
	}
	return filenamePool, monitorFileNamePool
}

// extractFirmwareFile extracts the firmware file based on the specified name pools to the labstation
func extractFirmwareFile(ctx context.Context, h *Helper, devserver, gcsFirmwareFilePath, servoTmpDir, firmwareFileName string, fileNamePool []string) string {
	for _, filename := range fileNamePool {
		testImageURL := fmt.Sprintf("%s/extract/%s?file=%s", devserver, gcsFirmwareFilePath, filename)
		testing.ContextLogf(ctx, "Downloading image file %q", filename)
		if err := h.ServoProxy.RunCommand(ctx, false, "curl", testImageURL, "--output", fmt.Sprintf("%s/%s", servoTmpDir, firmwareFileName)); err != nil {
			testing.ContextLogf(ctx, "Failed to extract image file at %q: %v", testImageURL, err)
			continue
		}
		return firmwareFileName
	}
	return ""
}
