// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import (
	"context"
	"encoding/binary"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fp "go.chromium.org/tast-tests/cros/common/fingerprint"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	generatedImagesSubDirectory = "images"
	versionStringLenBytes       = 32
	rollbackSizeBytes           = 4
)

// TestImageType specifies the test image variant.
type TestImageType int

// TestImageData represents a firmware test image
type TestImageData struct {
	// Path is the absolute path to the firmware file on the DUT
	Path string
	// ROVersion is the RO version string
	ROVersion string
	// RWVersion is the RW version string
	RWVersion string
}

// FMAPSection describes a firmware map section.
type FMAPSection string

const (
	// ROFirmwareID is the read-only firmware ID.
	ROFirmwareID FMAPSection = "RO_FRID"
	// RWFirmwareID is the read-write firmware ID.
	RWFirmwareID FMAPSection = "RW_FWID"
	// RWRollbackVersion is the read-write rollback version.
	RWRollbackVersion FMAPSection = "RW_RBVER"
	// RWFirmware is the read-write section.
	RWFirmware FMAPSection = "EC_RW"
	// SignatureRW is the signature section.
	SignatureRW FMAPSection = "SIG_RW"
)

const (
	// TestImageTypeOriginal is the original firmware on the DUT.
	TestImageTypeOriginal TestImageType = iota
	// TestImageTypeDev is a dev-key signed version of the firmware.
	TestImageTypeDev
	// TestImageTypeCorruptFirstByte is a variant of the original firmware with the first byte changed.
	TestImageTypeCorruptFirstByte
	// TestImageTypeCorruptLastByte is a variant of the original firmware with the last byte changed.
	TestImageTypeCorruptLastByte
	// TestImageTypeDevRollbackZero is a dev-key signed version of the firmware with rollback set to zero.
	TestImageTypeDevRollbackZero
	// TestImageTypeDevRollbackOne is a dev-key signed version of the firmware with rollback set to one.
	TestImageTypeDevRollbackOne
	// TestImageTypeDevRollbackTwo is a dev-key signed version of the firmware with rollback set to two.
	TestImageTypeDevRollbackTwo
	// TestImageTypeDevRollbackNine is a dev-key signed version of the firmware with rollback set to nine.
	TestImageTypeDevRollbackNine
)

// TestImages maps a given test image type to data describing the image.
type TestImages map[TestImageType]*TestImageData

type fmapSectionValue struct {
	Section *futility.FMapSection
	Bytes   []byte
}

type keyPair struct {
	PublicKeyPath  string
	PrivateKeyPath string
}

type firmwareImageGenerator struct {
	devKeyPair           *keyPair
	origFirmwareFilePath string
	rwVersion            *fmapSectionValue
	roVersion            *fmapSectionValue
}

func signFirmware(ctx context.Context, futilityInstance *futility.Instance, privateKeyFile, firmwareFile string) error {
	opt := futility.NewSignRWSigOptions(firmwareFile).WithPrivateKeyPath(privateKeyFile).WithVersion(1)

	_, err := futilityInstance.SignRWSig(ctx, opt)
	if err != nil {
		return errors.Wrap(err, "failed to run futility sign")
	}
	return nil
}

func createKeyPairFromRSAKey(ctx context.Context, futilityInstance *futility.Instance, pemFilePath, keyDescription string) (*keyPair, error) {
	opt := futility.NewCreateKeyPairOptions(pemFilePath).WithDescription(keyDescription)

	_, err := futilityInstance.CreateKeyPair(ctx, opt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run futility create")
	}

	outputFilePrefix := strings.TrimSuffix(pemFilePath, filepath.Ext(pemFilePath))

	return &keyPair{
		PublicKeyPath:  outputFilePrefix + ".vbpubk2",
		PrivateKeyPath: outputFilePrefix + ".vbprik2",
	}, nil
}

func fmapSectionInfo(ctx context.Context, futilityInstance *futility.Instance, firmwareFilePath string, sections []FMAPSection) ([]futility.FMapSection, error) {
	var sectionsString []string
	for _, section := range sections {
		sectionsString = append(sectionsString, string(section))
	}

	sectionsInfo, _, err := futilityInstance.DumpFmap(ctx, firmwareFilePath, sectionsString)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run futility dump_fmap")
	}

	return sectionsInfo, nil
}

func readFileAtOffset(ctx context.Context, d *rpcdut.RPCDUT, fileName string, data []byte, offset int64) error {
	if _, err := dutfs.NewClient(d.RPC().Conn).ReadFileAtOffset(ctx, fileName, offset, int64(len(data))); err != nil {
		return errors.Wrapf(err, "failed to read from offset: %v", offset)
	}

	return nil
}

func writeFileAtOffset(ctx context.Context, d *rpcdut.RPCDUT, fileName string, data []byte, offset int64) error {
	if err := dutfs.NewClient(d.RPC().Conn).WriteFileAtOffset(ctx, fileName, data, offset); err != nil {
		return errors.Wrapf(err, "failed to write data: %v to offset: %v", data, offset)
	}

	return nil
}

// createVersionStringWithSuffix returns a new copy of version with the last bytes
// replaced by suffix.
func createVersionStringWithSuffix(suffix string, version []byte) ([]byte, error) {
	if len(version) != versionStringLenBytes {
		return nil, errors.Errorf("incorrect version size, actual: %d, expected: %d", len(version), versionStringLenBytes)
	}
	if len(suffix) >= versionStringLenBytes {
		return nil, errors.Errorf("suffix %q is too long for version len: %d", suffix, versionStringLenBytes)
	}
	versionStr := string(version)[:versionStringLenBytes-len(suffix)-1]
	versionStr = strings.TrimRight(versionStr, "\x00")
	newVersion := make([]byte, versionStringLenBytes)
	copy(newVersion[:], versionStr+suffix)
	return newVersion, nil
}

func addSuffixToVersionString(ctx context.Context, d *rpcdut.RPCDUT, filePath, suffix string, version *fmapSectionValue) error {
	newVersion, err := createVersionStringWithSuffix(suffix, version.Bytes)
	if err != nil {
		return errors.Wrap(err, "failed to modify version string")
	}

	if err := writeFileAtOffset(ctx, d, filePath, newVersion, int64(version.Section.Offset)); err != nil {
		return errors.Wrap(err, "failed to update RO version string")
	}

	return nil
}

func createRollbackBytes(newRollbackValue uint32) []byte {
	rollbackBytes := make([]byte, rollbackSizeBytes)
	binary.LittleEndian.PutUint32(rollbackBytes, newRollbackValue)
	return rollbackBytes
}

func modifyFirmwareFileRollbackValue(ctx context.Context, d *rpcdut.RPCDUT, firmwareFilePath string, newRollbackValue uint32, rollback *fmapSectionValue) error {
	if rollback.Section.Size != rollbackSizeBytes {
		return errors.Errorf("incorrect version size, actual: %v, expected: %v", rollback.Section.Size, rollbackSizeBytes)
	}

	rollbackBytes := createRollbackBytes(newRollbackValue)

	if err := writeFileAtOffset(ctx, d, firmwareFilePath, rollbackBytes, int64(rollback.Section.Offset)); err != nil {
		return errors.Wrap(err, "failed to update rollback")
	}

	return nil
}

func newFirmwareImageGenerator(devKeyPair *keyPair, origFirmwareFilePath string, rwVersion, roVersion *fmapSectionValue) *firmwareImageGenerator {
	return &firmwareImageGenerator{
		devKeyPair:           devKeyPair,
		origFirmwareFilePath: origFirmwareFilePath,
		rwVersion:            rwVersion,
		roVersion:            roVersion,
	}
}

func (f *firmwareImageGenerator) DevSignedImage(ctx context.Context, d *rpcdut.RPCDUT, futilityInstance *futility.Instance) (string, error) {
	devFilePath := strings.TrimSuffix(f.origFirmwareFilePath, filepath.Ext(f.origFirmwareFilePath)) + ".dev"

	if err := dutfs.NewClient(d.RPC().Conn).CopyFile(ctx, f.origFirmwareFilePath, devFilePath); err != nil {
		return "", errors.Wrap(err, "failed to copy file")
	}

	if err := addSuffixToVersionString(ctx, d, devFilePath, ".dev", f.roVersion); err != nil {
		return "", errors.Wrap(err, "failed to modify RO version string")
	}

	if err := addSuffixToVersionString(ctx, d, devFilePath, ".dev", f.rwVersion); err != nil {
		return "", errors.Wrap(err, "failed to modify RW version string")
	}

	// The firmware was modified, so we need to re-sign it.
	if err := signFirmware(ctx, futilityInstance, f.devKeyPair.PrivateKeyPath, devFilePath); err != nil {
		return "", errors.Wrap(err, "failed to sign firmware")
	}

	return devFilePath, nil
}

func (f *firmwareImageGenerator) Rollback(ctx context.Context, d *rpcdut.RPCDUT, futilityInstance *futility.Instance, rollback *fmapSectionValue, newRollbackValue uint32) (string, error) {
	versionSuffix := ".rb" + strconv.FormatUint(uint64(newRollbackValue), 10)
	ext := ".dev" + versionSuffix
	rollbackFilePath := strings.TrimSuffix(f.origFirmwareFilePath, filepath.Ext(f.origFirmwareFilePath)) + ext

	if err := dutfs.NewClient(d.RPC().Conn).CopyFile(ctx, f.origFirmwareFilePath, rollbackFilePath); err != nil {
		return "", errors.Wrap(err, "failed to copy file")
	}

	if err := addSuffixToVersionString(ctx, d, rollbackFilePath, ".dev", f.roVersion); err != nil {
		return "", errors.Wrap(err, "failed to modify RO version string")
	}

	if err := addSuffixToVersionString(ctx, d, rollbackFilePath, versionSuffix, f.rwVersion); err != nil {
		return "", errors.Wrap(err, "failed to modify RW version string")
	}

	if err := modifyFirmwareFileRollbackValue(ctx, d, rollbackFilePath, newRollbackValue, rollback); err != nil {
		return "", errors.Wrap(err, "failed to modify rollback value")
	}

	// The firmware was modified, so we need to re-sign it
	if err := signFirmware(ctx, futilityInstance, f.devKeyPair.PrivateKeyPath, rollbackFilePath); err != nil {
		return "", errors.Wrap(err, "failed to sign firmware")
	}

	return rollbackFilePath, nil
}

func (f *firmwareImageGenerator) CorruptFirstByte(ctx context.Context, d *rpcdut.RPCDUT, futilityInstance *futility.Instance) (string, error) {
	corruptFilePath := strings.TrimSuffix(f.origFirmwareFilePath, filepath.Ext(f.origFirmwareFilePath)) + "_corrupt_first_byte.bin"

	if err := dutfs.NewClient(d.RPC().Conn).CopyFile(ctx, f.origFirmwareFilePath, corruptFilePath); err != nil {
		return "", errors.Wrap(err, "failed to copy file")
	}

	sections, err := fmapSectionInfo(ctx, futilityInstance, corruptFilePath, []FMAPSection{RWFirmware})
	if err != nil {
		return "", errors.Wrap(err, "failed to get FMAP info for EC_RW")
	}
	rwSection := sections[0]

	byteToCorrupt := make([]byte, 1)
	if err := readFileAtOffset(ctx, d, corruptFilePath, byteToCorrupt, int64(rwSection.Offset)+100); err != nil {
		return "", errors.Wrap(err, "failed to read byte")
	}

	byteToCorrupt[0]++
	if err := writeFileAtOffset(ctx, d, corruptFilePath, byteToCorrupt, int64(rwSection.Offset)+100); err != nil {
		return "", errors.Wrap(err, "failed to write corrupted byte")
	}

	return corruptFilePath, nil
}

func (f *firmwareImageGenerator) CorruptLastByte(ctx context.Context, d *rpcdut.RPCDUT, futilityInstance *futility.Instance) (string, error) {
	corruptFilePath := strings.TrimSuffix(f.origFirmwareFilePath, filepath.Ext(f.origFirmwareFilePath)) + "_corrupt_last_byte.bin"

	if err := dutfs.NewClient(d.RPC().Conn).CopyFile(ctx, f.origFirmwareFilePath, corruptFilePath); err != nil {
		return "", errors.Wrap(err, "failed to copy file")
	}

	sections, err := fmapSectionInfo(ctx, futilityInstance, corruptFilePath, []FMAPSection{SignatureRW})
	if err != nil {
		return "", errors.Wrap(err, "failed to get FMAP info for SIG_RW")
	}
	rwSection := sections[0]

	byteToCorrupt := make([]byte, 1)
	if err := readFileAtOffset(ctx, d, corruptFilePath, byteToCorrupt, int64(rwSection.Offset)-100); err != nil {
		return "", errors.Wrap(err, "failed to read byte")
	}

	byteToCorrupt[0]++
	if err := writeFileAtOffset(ctx, d, corruptFilePath, byteToCorrupt, int64(rwSection.Offset)-100); err != nil {
		return "", errors.Wrap(err, "failed to write corrupted byte")
	}

	return corruptFilePath, nil
}

func readFMAPSections(ctx context.Context, d *rpcdut.RPCDUT, futilityInstance *futility.Instance, firmwareFilePath string, sections []FMAPSection, sectionValues ...*fmapSectionValue) error {
	sectionInfoList, err := fmapSectionInfo(ctx, futilityInstance, firmwareFilePath, sections)
	if err != nil {
		return errors.Wrapf(err, "failed to get FMAP info for sections: %q", sections)
	}

	for i, sectionInfo := range sectionInfoList {
		sectionData := make([]byte, sectionInfo.Size)
		if err := readFileAtOffset(ctx, d, firmwareFilePath, sectionData, int64(sectionInfo.Offset)); err != nil {
			return errors.Wrapf(err, "unable to read FMAP section: %q", sectionInfo.Name)
		}

		sectionValues[i].Section = &sectionInfoList[i]
		sectionValues[i].Bytes = sectionData
	}

	return nil
}

// generateImages generates various test images from provided file using futility. Please note that the function works on host.
func generateImages(ctx context.Context, d *rpcdut.RPCDUT, keyFilePath, origFWFileCopy string, fpBoard fp.BoardName) (ret TestImages, retErr error) {
	futilityInstance, err := futility.NewLocalBuilder(d.DUT()).Build()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get futility instance")
	}

	devKeyPair, err := createKeyPairFromRSAKey(ctx, futilityInstance, keyFilePath, string(fpBoard)+" dev key")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create key pair")
	}

	var roVersion, rwVersion, rollback fmapSectionValue

	err = readFMAPSections(ctx, d, futilityInstance, origFWFileCopy, []FMAPSection{ROFirmwareID, RWFirmwareID, RWRollbackVersion}, &roVersion, &rwVersion, &rollback)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read firmware info")
	}

	firmwareImageGenerator := newFirmwareImageGenerator(devKeyPair, origFWFileCopy, &roVersion, &rwVersion)

	devFilePath, err := firmwareImageGenerator.DevSignedImage(ctx, d, futilityInstance)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate dev signed image")
	}

	rollbackZeroFilePath, err := firmwareImageGenerator.Rollback(ctx, d, futilityInstance, &rollback, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate image with modified rollback value 0")
	}

	rollbackOneFilePath, err := firmwareImageGenerator.Rollback(ctx, d, futilityInstance, &rollback, 1)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate image with modified rollback value 1")
	}

	rollbackTwoFilePath, err := firmwareImageGenerator.Rollback(ctx, d, futilityInstance, &rollback, 2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate image with modified rollback value 2")
	}

	rollbackNineFilePath, err := firmwareImageGenerator.Rollback(ctx, d, futilityInstance, &rollback, 9)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate image with modified rollback value 9")
	}

	corruptFirstBytePath, err := firmwareImageGenerator.CorruptFirstByte(ctx, d, futilityInstance)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate image with corrupt first byte")
	}

	corruptLastBytePath, err := firmwareImageGenerator.CorruptLastByte(ctx, d, futilityInstance)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate image with corrupt last byte")
	}

	return TestImages{
		TestImageTypeOriginal:         &TestImageData{Path: origFWFileCopy},
		TestImageTypeDev:              &TestImageData{Path: devFilePath},
		TestImageTypeCorruptFirstByte: &TestImageData{Path: corruptFirstBytePath},
		TestImageTypeCorruptLastByte:  &TestImageData{Path: corruptLastBytePath},
		TestImageTypeDevRollbackZero:  &TestImageData{Path: rollbackZeroFilePath},
		TestImageTypeDevRollbackOne:   &TestImageData{Path: rollbackOneFilePath},
		TestImageTypeDevRollbackTwo:   &TestImageData{Path: rollbackTwoFilePath},
		TestImageTypeDevRollbackNine:  &TestImageData{Path: rollbackNineFilePath},
	}, nil
}

// GenerateTestFirmwareImages generates a set of test firmware images from the firmware that is on the DUT.
func GenerateTestFirmwareImages(ctx context.Context, d *rpcdut.RPCDUT, keyFilePath string, fpBoard fp.BoardName, buildFWFile, dutTempDir string) (ret TestImages, retErr error) {
	testing.ContextLog(ctx, "Copying developer key to DUT")
	dutKeyFilePath := filepath.Join(dutTempDir, filepath.Base(keyFilePath))
	if _, err := linuxssh.PutFiles(ctx, d.Conn(), map[string]string{keyFilePath: dutKeyFilePath}, linuxssh.PreserveSymlinks); err != nil {
		return nil, errors.Wrapf(err, "failed to copy file from %q to %q", keyFilePath, dutTempDir)
	}

	origFWFileCopy := filepath.Join(dutTempDir, string(fpBoard)+".bin")
	if err := dutfs.NewClient(d.RPC().Conn).CopyFile(ctx, buildFWFile, origFWFileCopy); err != nil {
		return nil, errors.Wrap(err, "failed to copy original firmware file")
	}

	// Use separate context when generating test images. The context will
	// help to detect issues with slow 'futility'.
	generateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	images, err := generateImages(generateCtx, d, dutKeyFilePath, origFWFileCopy, fpBoard)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate test images on host")
	}

	for _, imageData := range images {
		// Make sure that images were actually copied to DUT.
		exists, err := dutfs.NewClient(d.RPC().Conn).Exists(ctx, imageData.Path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check existence of %q", imageData.Path)
		}
		if !exists {
			return nil, errors.Errorf("expected file to exist, but it does not: %q", imageData.Path)
		}

		// Collect the version strings from each of the generated images.
		version, err := GetBuildROFirmwareVersion(ctx, d, imageData.Path)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get RO version from firmware file: %q", imageData.Path)
		}
		imageData.ROVersion = version

		version, err = GetBuildRWFirmwareVersion(ctx, d, imageData.Path)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get RW version from firmware file: %q", imageData.Path)
		}
		imageData.RWVersion = version
	}

	return images, nil
}
