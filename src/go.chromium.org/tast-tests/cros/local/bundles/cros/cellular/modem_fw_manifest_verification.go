// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/modemfwd"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModemFWManifestVerification,
		Desc:         "Verifies the validity of the firmware manifest",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com", "madhavadas@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_carrier_agnostic", "cellular_cq", "cellular_ota_avl", "cellular_modem_verification", "group:release-health", "release-health_cellular"},
		SoftwareDeps: []string{"modemfwd"},
		Timeout:      5 * time.Minute,
	})
}

// ModemFWManifestVerification Test
func ModemFWManifestVerification(ctx context.Context, s *testing.State) {
	// Stop modemfwd so that the DLCs don't get uninstalled in the middle of the test.
	if err := upstart.StopJob(ctx, modemfwd.JobName); err != nil {
		s.Fatalf("Failed to stop %q: %s", modemfwd.JobName, err)
	}

	if !fileExists(cellular.GetModemFirmwareManifestPath()) {
		s.Fatal("Cannot find ", cellular.GetModemFirmwareManifestPath())
	}

	manifest, err := cellular.ParseModemFirmwareManifest(ctx)
	if err != nil {
		s.Fatal("Failed to parse the firmware manifest: ", err)
	}

	dutVariant, err := cellular.GetDeviceVariant(ctx)
	if err != nil {
		s.Fatalf("Failed to get device variant: %s", err)
	}

	board, err := cellular.GetBoard(ctx)
	if err != nil {
		s.Fatalf("Failed to get board: %s", err)
	}

	// Create a map of all the files in the firmware files. This map will be
	// used to check if any files are unused during the entire script run. The
	// value is set to true when the file is found to be used.
	pathsToFindUnusedFiles := make(map[string]bool)
	setFwPathAsUsed := func(file string) {
		for key := range pathsToFindUnusedFiles {
			// Mark full path matches, or directory names that match.
			if strings.HasPrefix(key, file+"/") || key == file {
				pathsToFindUnusedFiles[key] = true
			}
		}
	}

	fileExistsAndSetFwPathAsUsed := func(file string) bool {
		setFwPathAsUsed(file)
		return fileExists(file)
	}

	missingFiles := make(map[string]bool)
	var mainFirmwares map[string]bool
	dlcCounter := 0
	for _, device := range manifest.Device {
		// rootfs path
		modemFirmwarePaths := []string{cellular.GetModemFirmwarePath()}
		uninstallDlc := false
		if device.GetDlc() != nil && device.GetDlc().GetDlcId() != "" {
			dlcCounter++
			modemType, err := cellular.GetModemTypeFromDeviceID(device.DeviceId)
			if err != nil {
				s.Fatalf("Failed to get modem type: %s", err)
			}
			if err := verifyDlcManifest(ctx, device.GetDlc().GetDlcId(), modemType, device.Variant, board); err != nil {
				s.Fatalf("Invalid DLC manifest : %s", err)
			}
			// Only the variant that matches the device's variant will contain a DLC that is
			// already installed by modemfwd.
			// Manually install the DLCs for other variants. This won't work on non test
			// images since the DLCs are purged.  It works on test images because the modem
			// DLCs have the DLC_PRELOAD flag in their ebuilds.
			if dutVariant != device.Variant {
				uninstallDlc = true
				dlc.Install(ctx, device.Dlc.DlcId, "")
			}
			state, err := dlc.GetDlcState(ctx, device.Dlc.DlcId)
			// Verify that the DLC exists in the dlcservice manifest
			if err != nil {
				s.Fatalf("Failed to get state info for DLC %q: %q", device.Dlc.DlcId, err)
			}
			if state.RootPath == "" {
				s.Fatalf("Failed to get mount path for DLC %q", device.Dlc.DlcId)
			}
			// Append the DLC mount path for verification.
			// Until DLC is fully proven to work 100% for modemfwd, we keep a copy of all
			// modem FWs in the rootfs, so we need to verify that the files are in both locations.
			// When it's time to remove those images from the roofs, only one path will be needed,
			// and the DLC path will override the rootfs path.
			if !device.GetDlc().GetIsDlcEmpty() {
				modemFirmwarePaths = append(modemFirmwarePaths, state.RootPath)
			}
		}

		for _, firmwarePath := range modemFirmwarePaths {
			modemFirmwarePath := firmwarePath
			tmpDecodePath := filepath.Join("/tmp", firmwarePath)
			if fileExistsAndSetFwPathAsUsed(filepath.Join(modemFirmwarePath, "patch_manifest.textproto")) {
				defer os.RemoveAll(tmpDecodePath)
				if !fileExists(tmpDecodePath) {
					os.MkdirAll(tmpDecodePath, 0755)

					// Reconstruct this firmware payload to a temp directory
					if _, err := testexec.CommandContext(ctx, "patchmaker", "--decode", "--src_path="+firmwarePath, "--dest_path="+tmpDecodePath).Output(testexec.DumpLogOnError); err != nil {
						s.Fatalf("Failed to decode patched firmware directory: %s", err)
					}
				}

				// The remainder of this test should use the reconstructed directory
				modemFirmwarePath = tmpDecodePath
			}
			// Add paths to pathsToFindUnusedFiles if they don't already exist.
			for _, path := range getAllFilesInDir(s, modemFirmwarePath) {
				if _, ok := pathsToFindUnusedFiles[path]; !ok {
					pathsToFindUnusedFiles[path] = false // Default is unused
				}
			}
			setFwPathAsUsed(cellular.GetModemFirmwareManifestPath())

			s.Logf("Firmware path location for variant %q: %q", device.Variant, modemFirmwarePath)
			// Verify that we don't have repeated main FWs.
			mainFirmwares = make(map[string]bool)
			for _, mainFirmware := range device.MainFirmware {
				if mainFirmwares[mainFirmware.Version] {
					s.Fatalf("Repeated value for main firmware: %q in variant: %q", mainFirmware.Version, device.Variant)
				}
				mainFirmwares[mainFirmware.Version] = true
			}
			// Verify that all files exist.
			for _, mainFW := range device.MainFirmware {
				fullPath := filepath.Join(modemFirmwarePath, mainFW.Filename)
				if !fileExistsAndSetFwPathAsUsed(fullPath) {
					missingFiles[fullPath] = true
				}
				for _, associatedFW := range mainFW.AssocFirmware {
					fullPath := filepath.Join(modemFirmwarePath, associatedFW.Filename)
					if !fileExistsAndSetFwPathAsUsed(fullPath) {
						missingFiles[fullPath] = true
					}
				}
			}
			for _, oemFW := range device.OemFirmware {
				fullPath := filepath.Join(modemFirmwarePath, oemFW.Filename)
				if !fileExistsAndSetFwPathAsUsed(fullPath) {
					missingFiles[fullPath] = true
				}
				// Verify if main FWs used by OEM FW exist.
				for _, oemVersion := range oemFW.MainFirmwareVersion {
					if !mainFirmwares[oemVersion] {
						s.Fatalf("Main firmware %q referenced by OEM FW %q does not exist", oemVersion, oemFW.Version)
					}
				}
			}
			for _, carrierFW := range device.CarrierFirmware {
				fullPath := filepath.Join(modemFirmwarePath, carrierFW.Filename)
				if !fileExistsAndSetFwPathAsUsed(fullPath) {
					missingFiles[fullPath] = true
				}
				// Verify if main FWs used by carrier FW exist.
				if carrierFW.MainFirmwareVersion != "" && !mainFirmwares[carrierFW.MainFirmwareVersion] {
					s.Fatalf("Main firmware %q referenced by carrier FW %q does not exist", carrierFW.MainFirmwareVersion, carrierFW.Version)
				}
				if len(carrierFW.CarrierId) == 0 {
					s.Fatalf("There is no carrier id defined for carrier FW %q", carrierFW.Version)
				}
			}
			modemType, err := cellular.GetModemTypeFromDeviceID(device.DeviceId)
			if err != nil {
				s.Fatalf("Failed to get modem type: %s", err)
			}
			// Verify recovery on FM101
			if modemType == cellularconst.ModemTypeFM101 {
				recoveryFileList, recoveryDirPath, err := getFM101RecoveryFileList(modemFirmwarePath, device.Variant)
				setFwPathAsUsed(recoveryDirPath)
				if err != nil {
					s.Fatal("Failed to get recovery file list: ", err)
				}
				for _, fullPath := range recoveryFileList {
					if !fileExists(fullPath) {
						missingFiles[fullPath] = true
					}
				}
			}
			// Verify optional recovery_directory entry for this device is valid
			if device.GetRecoveryDirectory() != nil {
				recoveryPath := filepath.Join(modemFirmwarePath, device.GetRecoveryDirectory().GetFilename())
				if !fileExistsAndSetFwPathAsUsed(recoveryPath) {
					missingFiles[recoveryPath] = true
				}
			}

			// Remove any reconstructed DLCs from /tmp. Reconstructed rootfs
			// packages are re-used across devices, so that one is left alone
			// until all checks are complete.
			if firmwarePath != cellular.GetModemFirmwarePath() {
				os.RemoveAll(tmpDecodePath)
			}
		}
		// Remove the DLC after each test, otherwise we might run out of disk
		// space on devices with many DLCs and small hard drives.
		if uninstallDlc {
			dlc.Uninstall(ctx, device.Dlc.DlcId)
		}
	}

	if len(missingFiles) > 0 {
		keys := make([]string, 0, len(missingFiles))
		for k := range missingFiles {
			keys = append(keys, k)
		}
		s.Fatalf("The following firmware files are specified in the manifest, but don't exist: %q", keys)
	}
	var unusedFiles []string
	for k, v := range pathsToFindUnusedFiles {
		if v == false {
			unusedFiles = append(unusedFiles, k)
		}
	}
	if len(unusedFiles) > 0 {
		s.Fatalf("The following firmware files are not used in any devices: %q", unusedFiles)
	}

	if dlcCounter > 0 && dlcCounter != len(manifest.Device) {
		s.Fatal("There is an unequal number of variants and DLCs")
	}
}

func fileExists(file string) bool {
	_, err := os.Stat(file)
	return !os.IsNotExist(err)
}

func getAllFilesInDir(s *testing.State, dirPath string) []string {
	var files []string
	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		s.Fatal("Failed to walk the dir: ", err)
	}
	return files
}

func getFM101RecoveryFileList(firmwarePath, variant string) ([]string, string, error) {
	type recoveryData struct {
		XMLName xml.Name `xml:"data"`
		Text    string   `xml:"chardata"`
		Erase   []struct {
			Text                    string `xml:"chardata"`
			PAGESPERBLOCK           string `xml:"PAGES_PER_BLOCK,attr"`
			SECTORSIZEINBYTES       string `xml:"SECTOR_SIZE_IN_BYTES,attr"`
			NumPartitionSectors     string `xml:"num_partition_sectors,attr"`
			PhysicalPartitionNumber string `xml:"physical_partition_number,attr"`
			StartSector             string `xml:"start_sector,attr"`
			Tag                     string `xml:"tag,attr"`
		} `xml:"erase"`
		Program []struct {
			Text                    string `xml:"chardata"`
			PAGESPERBLOCK           string `xml:"PAGES_PER_BLOCK,attr"`
			SECTORSIZEINBYTES       string `xml:"SECTOR_SIZE_IN_BYTES,attr"`
			Filename                string `xml:"filename,attr"`
			Label                   string `xml:"label,attr"`
			NumPartitionSectors     string `xml:"num_partition_sectors,attr"`
			PhysicalPartitionNumber string `xml:"physical_partition_number,attr"`
			StartSector             string `xml:"start_sector,attr"`
			Tag                     string `xml:"tag,attr"`
		} `xml:"program"`
	}

	recoveryDirPath := filepath.Join(firmwarePath, "fm101", "download_agent"+"_"+variant)
	if !fileExists(recoveryDirPath) {
		recoveryDirPath = filepath.Join(firmwarePath, "fm101", "download_agent")
		if !fileExists(recoveryDirPath) {
			return nil, "", errors.New("missing download_agent")
		}
	}

	recoveryPath := filepath.Join(recoveryDirPath, "rawprogram_nand_p2K_b128K.xml")
	b, err := os.ReadFile(recoveryPath)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to read recovery xml")
	}
	var data recoveryData
	if err := xml.Unmarshal(b, &data); err != nil {
		return nil, "", errors.Wrap(err, "failed to unmarshal recovery xml")
	}
	var ret []string
	for i := 0; i < len(data.Program); i++ {
		if data.Program[i].Filename != "" {
			fullPath := filepath.Join(firmwarePath, "fm101", data.Program[i].Filename)
			ret = append(ret, fullPath)
		}
	}
	fileList := []string{"prog_nand_firehose_9x55.mbn", "patch_p2K_b128K.xml"}
	for _, file := range fileList {
		fullPath := filepath.Join(recoveryDirPath, file)
		ret = append(ret, fullPath)
	}
	return ret, recoveryDirPath, nil
}

type dlcSpecs struct {
	minPreallocSize int64 // Size in MiB
	// max values should always be defined a little bigger than expected, because dlcservice makes
	// the number bigger depending on the drive partition configuration.
	maxPreallocSize int64 // Size in MiB
	factoryInstall  bool
}

func newDlcSpec(minPreallocSize, maxPreallocSize int64) *dlcSpecs {
	return &dlcSpecs{
		minPreallocSize: minPreallocSize,
		maxPreallocSize: maxPreallocSize,
		factoryInstall:  true,
	}
}

func (f *dlcSpecs) setFactoryInstall(value bool) *dlcSpecs {
	f.factoryInstall = value
	return f
}

var (
	// This map should be used to override dlcSpecsPerModem on variants that have special requirements
	dlcSpecsPerDlcID = map[string]dlcSpecs{
		"modem-fw-dlc-nipperkin":        *newDlcSpec(29, 30),
		"modem-fw-dlc-guybrush360-l850": *newDlcSpec(30, 30),
		"modem-fw-dlc-guybrush-fm350":   *newDlcSpec(50, 50).setFactoryInstall(false),
	}

	dlcSpecsPerModem = map[cellularconst.ModemType]dlcSpecs{
		cellularconst.ModemTypeL850:   *newDlcSpec(39, 39),
		cellularconst.ModemTypeFM350:  *newDlcSpec(156, 156),
		cellularconst.ModemTypeFM101:  *newDlcSpec(200, 200),
		cellularconst.ModemTypeEM060:  *newDlcSpec(100, 300),
		cellularconst.ModemTypeLCUK54: *newDlcSpec(200, 200),
		cellularconst.ModemTypeRW101:  *newDlcSpec(200, 200),
		cellularconst.ModemTypeRW135:  *newDlcSpec(200, 310),
	}
)

// ImageLoaderManifest holds the fields related to a imageloader manifest.
type imageLoaderManifest struct {
	Attributes          map[string]interface{} `json:"attributes"`
	CriticalUpdate      bool                   `json:"critical-update"`
	FactoryInstall      bool                   `json:"factory-install"`
	ID                  string                 `json:"id"`
	LoadPinVerityDigest bool                   `json:"loadpin-verity-digest"`
	PowerwashSafe       bool                   `json:"powerwash-safe"`
	PreAllocatedSize    int64                  `json:"pre-allocated-size,string"`
	PreloadAllowed      bool                   `json:"preload-allowed"`
	UseLogicalVolume    bool                   `json:"use-logical-volume"`
}

// Metadata holds the fields related to the DLC metadata.
type dlcMetadata struct {
	Manifest imageLoaderManifest `json:"manifest"`
	Table    string              `json:"table"`
}

func getDlcMetadata(ctx context.Context, id string) (*dlcMetadata, error) {
	buf, err := testexec.CommandContext(ctx, "dlc_metadata_util", "--get", "--id="+id).Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the DLC metadata")
	}

	var metadata dlcMetadata
	if err := json.Unmarshal(buf, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func verifyDlcManifest(ctx context.Context, dlcID string, modemType cellularconst.ModemType, variantInManifest, board string) error {
	metadata, err := getDlcMetadata(ctx, dlcID)
	if err != nil {
		return errors.Wrapf(err, "failed to get manifest for DLC: %s", dlcID)
	}
	var specs dlcSpecs
	origin := ""
	if val, ok := dlcSpecsPerDlcID[dlcID]; ok {
		specs = val
		origin = "DlcId " + dlcID
	} else if val, ok := dlcSpecsPerModem[modemType]; ok {
		specs = val
		origin = "modem " + modemType.String()
	} else {
		return errors.Errorf("cannot find DLC specifications for DLC: %s", dlcID)
	}
	preallocSizeInMib := metadata.Manifest.PreAllocatedSize / 1024.0 / 1024
	if preallocSizeInMib < specs.minPreallocSize {
		return errors.Errorf("pre-allocated size less than expected for %s. DLC: %s. Got %d, expected >= %d", origin, dlcID, preallocSizeInMib, specs.minPreallocSize)
	}
	if preallocSizeInMib > specs.maxPreallocSize {
		return errors.Errorf("pre-allocated size greater than expected for %s. DLC: %s. Got %d, expected <= %d", origin, dlcID, preallocSizeInMib, specs.maxPreallocSize)
	}

	if metadata.Manifest.FactoryInstall != specs.factoryInstall {
		return errors.Errorf("DLC_FACTORY_INSTALL was not configured correctly in DLC %s. Got %t, expected %t", dlcID, metadata.Manifest.FactoryInstall, specs.factoryInstall)
	}
	if !metadata.Manifest.PreloadAllowed {
		return errors.Errorf("DLC_PRELOAD was not set in DLC %s", dlcID)
	}
	if !metadata.Manifest.CriticalUpdate {
		return errors.Errorf("DLC_CRITICAL_UPDATE was not set in DLC %s", dlcID)
	}
	if !metadata.Manifest.PowerwashSafe {
		return errors.Errorf("DLC_POWERWASH_SAFE was not set in DLC %s", dlcID)
	}
	if !metadata.Manifest.UseLogicalVolume {
		return errors.Errorf("DLC_USE_LOGICAL_VOLUME was not set in DLC %s", dlcID)
	}
	if (modemType != cellularconst.ModemTypeL850) && !metadata.Manifest.LoadPinVerityDigest {
		return errors.Errorf("DLC_LOADPIN_VERITY_DIGEST was not set in DLC %s", dlcID)
	}
	if _, ok := metadata.Manifest.Attributes["modem"]; !ok {
		return errors.Errorf("attributes missing the `modem` attribute. DLC: %s. Attributes: %q", dlcID, metadata.Manifest.Attributes)
	}
	if _, ok := metadata.Manifest.Attributes[variantInManifest]; !ok {
		return errors.Errorf("attributes missing variant. Variant: '%q' DLC: %q Attributes: '%q'", variantInManifest, dlcID, metadata.Manifest.Attributes)
	}
	return nil
}
