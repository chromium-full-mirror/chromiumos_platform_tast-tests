// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

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

// FWTargets represents the firmware targets to flash
type FWTargets struct {
	APTarget string
	ECTarget string
}

// configData is used to read the contents of config.yaml file from the firmware tarball
type configData struct {
	ChromeOS struct {
		Configs []struct {
			Firmware struct {
				BuildTargets struct {
					// AP image name, if missing fallback to ImageName
					Coreboot string `yaml:"coreboot"`
					EC       string `yaml:"ec"`
					ZephyrEC string `yaml:"zephyr-ec"`
				} `yaml:"build-targets"`
				ImageName string `yaml:"image-name"`
			} `yaml:"firmware"`
			Name     string `yaml:"name"`
			Identity struct {
				SKUID int `yaml:"sku-id"`
			} `yaml:"identity"`
		}
	}
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
	// configPath is the path of the config file from the DUT to find the downloaded firmware binary names that
	// should be used.
	configPath = "/usr/share/chromeos-config/yaml/config.yaml"
	// defaultTarSuffix is the default firmware tarball suffix
	defaultTarSuffix = ".tar.bz2"
)

// VerifyFwIDs will show in logs the current firmware version and compare it to expected ones if they are provided.
func VerifyFwIDs(ctx context.Context, h *Helper, exROVersion, exRWVersion string) error {
	currentROID, currentRWID, err := h.Reporter.GetFWRORWVersion(ctx)
	if err != nil {
		return err
	}
	if exROVersion != currentROID && !strings.Contains(exROVersion, currentROID) {
		return errors.Errorf("got %s RO version, but expected %s", currentROID, exROVersion)
	}
	if exRWVersion != currentRWID && !strings.Contains(exRWVersion, currentRWID) {
		return errors.Errorf("got %s RW version, but expected %s", currentRWID, exRWVersion)
	}
	return nil
}

// VerifyECFwIDs will compare the current EC firmware versions to expected ones.
func VerifyECFwIDs(ctx context.Context, h *Helper, exROVersion, exRWVersion string) error {
	currentROID, currentRWID, err := NewECTool(h.DUT, ECToolNameMain).RORWVersion(ctx)
	if err != nil {
		return err
	}
	if exROVersion != currentROID && !strings.Contains(exROVersion, currentROID) {
		return errors.Errorf("got %s RO version, but expected %s", currentROID, exROVersion)
	}
	if exRWVersion != currentRWID && !strings.Contains(exRWVersion, currentRWID) {
		return errors.Errorf("got %s RW version, but expected %s", currentRWID, exRWVersion)
	}
	return nil
}

// DownloadFirmwareFiles will extract the AP and EC bin files from the cloud storage.
// TODO: Currently DownloadFirmwareFile and DownloadRequiredFirmwareFiles perform similar
// actions. As soon as DownloadFirmwareFile is proven to work in the lab environment, these two
// functions should be accommodated together so that we can remove one of them.
func DownloadFirmwareFiles(ctx context.Context, cs *testing.CloudStorage, h *Helper, tmpDir, gcsFirmwareFilePath, fileName string, fwTargets *FWTargets) (*FWFilesToFlash, error) {
	testing.ContextLogf(ctx, "Downloading firmware image from the path: %s", gcsFirmwareFilePath)
	// In case a file name is not provided, the default const FiemwareFileName will be used.
	// Also checks if the gcsFirmwareFilePath includes firmware file name and resets
	// the firmware filename in that case.
	// The downloaded file in tmpDir will always be named as the const FirmwareFileName.
	if fileName == "" && !strings.HasSuffix(gcsFirmwareFilePath, defaultTarSuffix) {
		fileName = FirmwareFileName
	} else if strings.HasSuffix(gcsFirmwareFilePath, defaultTarSuffix) {
		fileName = ""
	}
	// Ensuring that the file path has a 'gs://' preFix.
	if !strings.HasPrefix(gcsFirmwareFilePath, "gs://") {
		gcsFirmwareFilePath = "gs://" + gcsFirmwareFilePath
	}
	// Find a devserver that works.
	for _, devserver := range cs.Devservers() {
		testing.ContextLogf(ctx, "Trying devserver at %q", devserver)
		if err := h.ServoProxy.RunCommand(ctx, false, "curl", "-f", "--connect-timeout", "3", fmt.Sprintf("%s/check_health", devserver)); err != nil {
			testing.ContextLogf(ctx, "Devserver %q not healthy: %v", devserver, err)
			continue
		}

		stagingURL := fmt.Sprintf("%s/stage?archive_url=%s&files=%s", devserver, gcsFirmwareFilePath, fileName)
		testing.ContextLogf(ctx, "Staging image %q", stagingURL)
		if err := h.ServoProxy.RunCommand(ctx, false, "curl", "-fL", stagingURL); err != nil {
			testing.ContextLogf(ctx, "Failed to stage file at %q: %v", stagingURL, err)
			continue
		}

		ecFilenamePool, ecMonitorFileNamePool := getFileNamePools(ctx, fwTargets.ECTarget, ECFirmware)
		apFileNamePool, _ := getFileNamePools(ctx, fwTargets.APTarget, APFirmware)
		// No need of the Prefix 'gs://' for the extraction.
		gcsFirmwareFilePath = strings.TrimPrefix(gcsFirmwareFilePath, "gs://")
		// To do the extraction, gcsFirmwareFilePath should point to the tar file to be untared.
		gcsFirmwareFilePath = filepath.Join(gcsFirmwareFilePath, fileName)
		monitorBin := extractFirmwareFile(ctx, h, devserver, gcsFirmwareFilePath, tmpDir, MonitorFileToFlash, ecMonitorFileNamePool)
		apBin := extractFirmwareFile(ctx, h, devserver, gcsFirmwareFilePath, tmpDir, APFirmwareFileToFlash, apFileNamePool)
		ecBin := extractFirmwareFile(ctx, h, devserver, gcsFirmwareFilePath, tmpDir, ECFirmwareFileToFlash, ecFilenamePool)
		// Both apBin and ecBin should be found.
		if apBin == "" || ecBin == "" {
			return nil, errors.Errorf("unable to extract bin files, ap file exists: %v, ec file exists: %v", apBin != "", ecBin != "")
		}
		return &FWFilesToFlash{ECFirmwareFile: ecBin, MonitorFile: monitorBin, APFirmwareFile: apBin}, nil
	}
	return nil, errors.New("unable to download file")
}

// UntarUnknownFileName will try to untar the respective fw bin file from the downloaded tar file.
func UntarUnknownFileName(ctx context.Context, tmpDir, fwidModel string, fwType FWType) (string, string, error) {
	// List of possible formats for the binary file found in a downloaded tar file.
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

// ReadFirmwareTargets finds the firmware binary name that should be used from 'config.yaml' on the DUT.
func ReadFirmwareTargets(ctx context.Context, conn *ssh.Conn, model, fwidModel string) (*FWTargets, error) {
	apTarget := fwidModel
	ecTarget := fwidModel

	out, err := conn.CommandContext(ctx, "crosid").Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run crosid")
	}
	re, err := regexp.Compile(`^SKU='([^']*)'`)
	if err != nil {
		return nil, errors.Wrap(err, "sku regex failed")
	}
	m := re.FindStringSubmatch(string(out))
	sku := -1
	if m != nil {
		if m[1] != "none" {
			sku, err = strconv.Atoi(m[1])
			if err != nil {
				return nil, errors.Wrapf(err, "parse of SKU %q failed", m[1])
			}
		}
		testing.ContextLogf(ctx, "DUT sku = %d", sku)
	}

	out, err = conn.CommandContext(ctx, "cat", configPath).Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run 'cat' command")
	}
	config := bytes.NewReader(out)
	parser := yaml.NewDecoder(config)
	configYaml := configData{}
	if err := parser.Decode(&configYaml); err != nil {
		return nil, errors.Wrap(err, "failed to parse config.yaml")
	}
	for _, config := range configYaml.ChromeOS.Configs {
		if config.Name == model {
			if sku >= 0 && config.Identity.SKUID >= 0 && config.Identity.SKUID != sku {
				continue
			}
			thisAPName := config.Firmware.BuildTargets.Coreboot
			// AP image name, if missing fallback to ImageName
			if thisAPName == "" {
				thisAPName = config.Firmware.ImageName
			}
			thisAPName = strings.TrimSpace(thisAPName)
			if thisAPName != "" {
				apTarget = thisAPName
			}
			var thisECName string
			// Bizarrely, zephyr builders use the coreboot name for the ec.bin file.
			if config.Firmware.BuildTargets.ZephyrEC != "" {
				thisECName = thisAPName
			} else {
				thisECName = config.Firmware.BuildTargets.EC
			}
			thisECName = strings.TrimSpace(thisECName)
			if thisECName != "" {
				ecTarget = thisECName
			}
		}
	}
	return &FWTargets{APTarget: apTarget, ECTarget: ecTarget}, nil
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
		testing.ContextLogf(ctx, "Trying to download image file %q", filename)

		if httpCode, err := h.ServoProxy.OutputCommand(ctx, false, "curl", "-sIL", "-w", "%{http_code}", "-o", "/dev/null", testImageURL); err != nil {
			testing.ContextLogf(ctx, "Failed to get HTTP code for %s: %v", filename, err)
			continue
		} else if string(httpCode) != "200" {
			testing.ContextLogf(ctx, "HTTP code is: %s, skip the file name: %s", string(httpCode), filename)
			continue
		}

		if err := h.ServoProxy.RunCommand(ctx, false, "curl", "-s", "-S", "-fL", testImageURL, "--output", fmt.Sprintf("%s/%s", servoTmpDir, firmwareFileName)); err != nil {
			testing.ContextLogf(ctx, "Failed to extract image file at %q: %v", testImageURL, err)
			continue
		}
		return firmwareFileName
	}
	return ""
}
