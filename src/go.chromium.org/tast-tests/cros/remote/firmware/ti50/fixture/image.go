// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	remoteTi50 "go.chromium.org/tast-tests/cros/remote/firmware/ti50"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ImageType represents a kind of ti50 image, either the main production image, or a special test
// image, such as "system_test_auto".
type ImageType string

const (
	// BuildURL is the arg name for the directory of the gs build or full path of the image (local or in gs).
	BuildURL = "buildurl"

	// LatestPrefix allows BuildURL to be specified as latest-<branch> obtain the latest available images.
	LatestPrefix = "latest-"
	// ReleasePrefix allows BuildURL lookup the gs path for the given GSC version string. This uses
	// the same format as the QUAL_VERSION files.
	// ex release-0.24.90/FFFF:0:0x10
	ReleasePrefix = "release-"

	// FwConfigJSON is the arg name for the json configuration file (for use in case buildurl
	// specifies a single .bin file, rather than a directory).
	FwConfigJSON = "fw_configjson"

	// Chip is an arg name for specifying Ti50 target.
	Chip = "chip"

	// Variant is an arg name for specifying Ti50 target.
	Variant = "variant"

	// Slot can be left empty, or set to either 'A' or 'B'.
	Slot = "slot"

	// imageBinGlob matches signed image bin files.
	imageBinGlob = "*.bin"

	// tastConfigGlob matches fw config json files in tast/<testbedtype>-<imagetype>/.
	tastConfigGlob = "opentitantool_fw_config.json"

	gsPrefix = "gs://"

	// ToTBranch is the Tip-of-Tree branch having artifacts at postSubmitArtifactsBuilder.
	ToTBranch                  string = "tot"
	postSubmitArtifactsBuilder string = "chromeos-image-archive/firmware-ti50-postsubmit"

	// Cr50QualBranch is the latest qual candidate for Cr50
	Cr50QualBranch string = "cr50qual"
	// GSCQualBranch is the latest qual candidate for the running GSC testbed type
	GSCQualBranch string = "gscqual"
	// Ti50QualBranch is the latest qual candidate for Ti50
	Ti50QualBranch string = "ti50qual"

	// Remote image paths
	latestQualFile = "chromeos-localmirror-private/distfiles/%s-%s-QUAL_VERSION"
	// DevGSCImageBucket is the bucket where node locked GSC test images are stored.
	DevGSCImageBucket  = "gs://chromeos-localmirror-private/distfiles/chromeos-%s*/"
	debugImageTemplate = "*.dbg%s.0x%s_0x%s.bin.*"
	efiImageTemplate   = "*_Unknown_NodeLocked-%s_*.bin"
	qualPrivateBucket  = "chromeos-localmirror-private/distfiles/"
	qualBucket         = "chromeos-localmirror/distfiles/"
	postsubmitBucket   = "chromeos-image-archive"
	releaseBucket      = "chromeos-releases"

	// GSC filenames
	cr50Release          = "cr50.r0.0.*.w%s.tbz2"
	cr50BIDLockedRelease = "cr50.r0.0.*.w%s_%s_%08x_%08x.tbz2"
	// Ti50 changed formats. This works with old and new versions.
	ti50Release          = "ti50*.r*w*%s.tar.xz"
	ti50BIDLockedRelease = "ti50*.r*w*%s_%s_%08x_%08x.tar.xz"
)

// ImageType declarations, please update AllTi50ImageTypes() after editing.
const (
	// SystemImage fixture downloads the system image bin used by ChromeOS DUTs.
	SystemImage ImageType = "default"

	// SystemTestAutoImage fixture downloads the system_test_auto image bin.
	SystemTestAutoImage ImageType = "system_test_auto"

	// SystemTestAuto2Image fixture downloads the system_test_auto_2 image bin.
	SystemTestAuto2Image ImageType = "system_test_auto_2"

	// SystemTestAutoAImage fixture downloads the system_test_auto_a image bin.
	SystemTestAutoAImage ImageType = "system_test_auto_a"

	// Ti50AImage fixture downloads the ti50a image bin.
	Ti50AImage ImageType = "ti50a"
)

var (
	// defaultFwConfigs are fw config files packaged in the data directory.
	defaultFwConfigs = []string{"cr50_h1.json", "ti50_dt.json", "ti50_he.json", "ti50_ot.json"}

	// reQualVersion extracts relevant contents of qual files.
	reQualVersion = regexp.MustCompile(`(.*)`)
	// reBIDLockedQualVersion extracts relevant contents of qual files.
	reBIDLockedQualVersion = regexp.MustCompile(`(.*)/(.*):(.*):(.*)`)
	// reGSCReleaseTarball extracts the firmware name from the tarball filename.
	reGSCReleaseTarball = regexp.MustCompile(`(cr50|ti50-nt|ti50).*(tar.xz|tbz2)`)

	// reTestbedTypeParts extracts relevant parts of the testbed type string.
	reTestbedTypeParts = regexp.MustCompile(`gsc_([[:alnum:]]*)`)
)

// AllTi50TestbedTypes returns all the testbed types that use ti50 images.
func AllTi50TestbedTypes() []ti50.TestbedType {
	return []ti50.TestbedType{ti50.GscDTAndreiboard, ti50.GscDTShield, ti50.GscOpentitanCw310Fpga, ti50.GscHostEmulation, ti50.GscOTShield, ti50.GscDTShieldV2, ti50.GscNTShieldV2}
}

// AllTi50ImageTypes returns all the ti50 image types.
func AllTi50ImageTypes() []ImageType {
	return []ImageType{SystemImage, SystemTestAutoImage, SystemTestAuto2Image, SystemTestAutoAImage}
}

// ImageValue provides access to a image binary along with json configuration files.
type ImageValue struct {
	imagePath   string
	configPaths []string
}

// ImagePath returns the path to the image binary.
func (v *ImageValue) ImagePath() string {
	return v.imagePath
}

// FwConfigPaths returns the list of json FW configuration files to use with the image.
func (v *ImageValue) FwConfigPaths() []string {
	return v.configPaths
}

// downloadImage downloads the image from google storage if necessary.
// inputURL can be a local file, a gs file, or a gs build folder.
func downloadImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties, imageType ImageType, s *testing.FixtState) (*ImageValue, error) {
	var err error
	var fullGlob, jsonGlob string
	inputURL, _ := s.Var(BuildURL)
	iv := &ImageValue{}

	// For inputURL that is in the form of release-*, extract the version and lookup the corresponding GS path.
	if strings.HasPrefix(inputURL, ReleasePrefix) {
		v := inputURL[len(ReleasePrefix):]
		inputURL, err = LookupGSCReleaseTarball(ctx, v, FindFwName(testbedProperties.TestbedType))
		if err != nil {
			return nil, err
		}
	} else if strings.HasPrefix(inputURL, LatestPrefix) {
		// For inputURL that is in the form of latest-*, convert it to the corresponding GS path.
		branch := inputURL[len(LatestPrefix):]
		if branch == ToTBranch {
			fullGlob, jsonGlob, err = findLatestCompletedTi50PostsubmitBuildURL(ctx, testbedProperties.TestbedType, imageType)
			if err != nil {
				return nil, err
			}
			testing.ContextLogf(ctx, "Found image: %s, config: %s for %s", fullGlob, jsonGlob, inputURL)
			inputURL = jsonGlob
		} else if strings.HasSuffix(branch, "qual") {
			inputURL, err = lookupLatestGSCQualTarball(ctx, testbedProperties.TestbedType, strings.Contains(branch, "ti50a"))
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errors.New("unrecognized branch " + branch)
		}
	}

	// For inputURL that is in the form of gs://*.tbz2, convert it to a local file by downloading.
	if strings.HasPrefix(inputURL, gsPrefix) && reGSCReleaseTarball.FindStringSubmatch(inputURL) != nil {
		downloadedFile, err := DownloadToTempFile(ctx, "tbz2", inputURL)
		if err != nil {
			return nil, err
		}
		inputURL = downloadedFile
		testing.ContextLogf(ctx, "Downloaded: %s", inputURL)
	}

	// For inputURL that is still in gs://, it should now be either a build folder or .bin file
	if strings.HasPrefix(inputURL, gsPrefix) {
		// BuildURL pointing to artifacts via a Google Storage URL.  The logic below
		// selects an image and corresponding json file depending on the type needed by
		// the test case.
		fullURL := inputURL
		jsonURL := ""
		// Assume URL is a build folder if it doesn't end in .bin.
		if !strings.HasSuffix(inputURL, ".bin") {
			if fullGlob == "" {
				tastURL := gsPrefix + filepath.Join(strings.TrimPrefix(inputURL, gsPrefix), "tast")
				testing.ContextLogf(ctx, "Looking for tast directory %s", tastURL)
				if !gsURLExists(ctx, tastURL) {
					return nil, errors.New("Missing tast directory")
				}
				// Cloud directory has a "tast/" subdirectory, use images from there.
				imageDir, err := ti50ImageDirectory(testbedProperties.TestbedType, imageType)
				if err != nil {
					return nil, err
				}
				imageGlob, jsonGlob, exists := tastImageAndJSONExists(ctx, inputURL, imageDir)
				if !exists {
					return nil, errors.New("Image or json files missing")
				}
				fullURL = imageGlob
				jsonURL = jsonGlob
				testing.ContextLogf(ctx, "Found tast directory %s", fullURL)
			} else {
				fullURL = fullGlob
				jsonURL = jsonGlob
			}
		}

		downloadedBin, err := DownloadToTempFile(ctx, "image bin", fullURL)
		if err != nil {
			return nil, err
		}
		iv.imagePath = downloadedBin

		if jsonURL != "" {
			downloadedJSON, err := DownloadToTempFile(ctx, "fw conf json", jsonURL)
			if err != nil {
				return nil, err
			}
			iv.configPaths = []string{downloadedJSON}
		}
		// For inputURL that is empty, use existing image
	} else if inputURL == "" {
		// Absence of BuildURL argument.  This instructs tast to not flash any image to
		// the devboard, but run the test against the code already running.  This works
		// only if the image on the board is of the same type (system_test_auto / ti50) as
		// the test case expects.  A json configuration file will be selected from
		// ti50/common based on the information from devboardservice and the type of image
		// declared on the test case.
		testing.ContextLogf(ctx, "-var=%s= not provided, assuming the devboard has a %s image", BuildURL, imageType)
		// For inputURL that is a local path, may need to extract compressed archive
	} else {
		img, err := os.Stat(inputURL)
		if err != nil {
			return nil, err
		}
		// Disallow directories
		if img.IsDir() {
			return nil, errors.New("-var=" + BuildURL + " must be a file: " + inputURL)
			// Extract image from tarball
		} else if reGSCReleaseTarball.FindStringSubmatch(inputURL) != nil {
			extractedFile, err := ExtractGSCQualImageFromTarball(ctx, inputURL)
			if err != nil {
				return nil, err
			}
			inputURL = extractedFile
		}
		iv.imagePath = inputURL
	}

	conf, hasManualConf := s.Var(FwConfigJSON)
	if hasManualConf {
		// Manual Configuration overrides everything.
		_, err := os.Stat(conf)
		if err != nil {
			return nil, err
		}
		iv.configPaths = []string{conf}
	} else if len(iv.configPaths) == 0 {
		// No configPaths found yet, use default configs.
		conf = defaultConfigPath(s, testbedProperties.TestbedType, imageType)
		testing.ContextLogf(ctx, "-var=%s= not provided, and non found in tast artifacts, using default at %s", FwConfigJSON, conf)
		iv.configPaths = []string{conf}
	}

	return iv, nil
}

// findLatestCompletedTi50PostsubmitBuildURL finds the most recent build with the full set of image artifacts.
// Returns the image and fw config json globs for the specified image type.
func findLatestCompletedTi50PostsubmitBuildURL(ctx context.Context, t ti50.TestbedType, iT ImageType) (string, string, error) {
	builds, err := gsLs(ctx, "builds for tot", gsPrefix+postSubmitArtifactsBuilder)

	if err != nil {
		return "", "", err
	}

	sort.Slice(builds, func(i, j int) bool {
		// buildRe extracts the build number, YYYYY, in .*/Rxxx-xxxxx.x.x-YYYYY-xxxx...
		var buildRe = regexp.MustCompile(`.*/R\d+-[0-9.]*-(\d+)-\d*/?`)
		am := buildRe.FindStringSubmatch(builds[i])
		bm := buildRe.FindStringSubmatch(builds[j])

		// Consider non-matching builds older than matching builds
		if am == nil {
			return true
		}
		if bm == nil {
			return false
		}

		ai, _ := strconv.Atoi(am[1])
		bi, _ := strconv.Atoi(bm[1])
		return ai < bi
	})

Loop:
	for i := len(builds) - 1; i >= 0; i-- {
		build := builds[i]
		var theImageGlob, theJSONGlob string
		scannedDirs := make(map[string]bool)
		for _, boardType := range AllTi50TestbedTypes() {
			if len(t) > 0 && boardType != t {
				continue
			}
			for _, imageType := range AllTi50ImageTypes() {
				dir, err := ti50ImageDirectory(boardType, imageType)
				if err != nil {
					return "", "", err
				}
				if _, ok := scannedDirs[dir]; ok {
					continue
				}
				scannedDirs[dir] = true
				imageGlob, jsonGlob, exists := tastImageAndJSONExists(ctx, build, dir)
				if !exists {
					continue Loop
				}
				if imageType == iT {
					theImageGlob = imageGlob
					theJSONGlob = jsonGlob
				}
			}
		}
		return theImageGlob, theJSONGlob, nil
	}
	return "", "", errors.New("found no completed builds for tot")
}

func tastImageAndJSONExists(ctx context.Context, tastParentURL, imageDir string) (imageGlob, jsonGlob string, exists bool) {
	tastParentURL = strings.TrimPrefix(tastParentURL, gsPrefix)
	artifactsDir := filepath.Join(tastParentURL, "tast", imageDir)

	skipArtifactsDir := false
	if strings.HasPrefix(imageDir, "nt-") && strings.Contains(artifactsDir, postSubmitArtifactsBuilder) {
		// Avoid invalid rw.bin for NT.
		// gs://chromeos-image-archive/firmware-ti50-postsubmit/R137-16246.0.0-109457-8718210281088122625/tast/nt-ti50/rw.bin
		skipArtifactsDir = true
	}

	imageGlob = gsPrefix + filepath.Join(artifactsDir, imageBinGlob)
	if skipArtifactsDir || !gsURLExists(ctx, imageGlob) {
		imageGlob2 := ""
		if strings.HasPrefix(imageDir, "dt-") {
			// Check alternate location for DT signed images.
			// gs://chromeos-image-archive/firmware-ti50-postsubmit/R137-16239.0.0-109298-8718832882770194753/tast/dt-ti50/opentitantool_fw_config.json
			// gs://chromeos-releases/firmware-ti50-postsubmit/R137-16239.0.0-109298-8718832882770194753/dt-ti50.tar.bz2/*.bin
			imageGlob2 = gsPrefix + filepath.Join(strings.Replace(tastParentURL, postsubmitBucket, releaseBucket, 1), imageDir+".tar.bz2", imageBinGlob)
		} else if strings.HasPrefix(imageDir, "nt-") {
			// Check alternate location for NT signed images.
			// gs://chromeos-image-archive/firmware-ti50-postsubmit/R137-16246.0.0-109457-8718210281088122625/tast/nt-ti50/opentitantool_fw_config.json
			// gs://chromeos-releases/ti50/nt-signed/R137-16246.0.0-109457/ti50_Ti50NuvoTitanSemiProdKeys_nt-ti50.bin
			psRe := regexp.MustCompile(`.*/firmware-ti50-postsubmit/([^/]+)-[0-9]+/`)
			psm := psRe.FindStringSubmatch(artifactsDir)
			if psm != nil {
				imageGlob2 = gsPrefix + filepath.Join(releaseBucket, "ti50/nt-signed", psm[1], "ti50_Ti50NuvoTitanSemiProdKeys_"+imageDir+".bin")
			}
		}
		if imageGlob2 == "" {
			testing.ContextLogf(ctx, "Rejecting %s: %s missing", artifactsDir, imageGlob)
			return imageGlob, jsonGlob, false
		}
		imageGlob = imageGlob2
		if !gsURLExists(ctx, imageGlob) {
			testing.ContextLogf(ctx, "Rejecting %s: %s missing", artifactsDir, imageGlob)
			return imageGlob, jsonGlob, false
		}
	}

	jsonGlob = gsPrefix + filepath.Join(artifactsDir, tastConfigGlob)
	if !gsURLExists(ctx, jsonGlob) {
		testing.ContextLogf(ctx, "Rejecting %s: %s missing", artifactsDir, jsonGlob)
		return imageGlob, jsonGlob, false
	}

	return imageGlob, jsonGlob, true
}

// DownloadToTempFile downloads url (gs) to a temp file.
func DownloadToTempFile(ctx context.Context, desc, url string) (string, error) {
	baseName := filepath.Base(url)
	baseName = strings.ReplaceAll(baseName, "*", "STAR")
	f, err := os.CreateTemp("", "*."+baseName)
	if err != nil {
		return "", errors.Wrapf(err, "Unable to download %s to: %s ", desc, baseName)
	}
	f.Close()

	if _, err := cmd(ctx, "download "+desc, "gsutil", "cp", url, f.Name()); err != nil {
		os.Remove(f.Name())
		return "", errors.Wrapf(err, "failed to download %s %s", desc, url)
	}
	return f.Name(), nil
}

// getBIDInt converts a board id mask or flag string to an int.
func getBIDInt(b string) (int64, error) {
	b = strings.TrimPrefix(b, "0x")
	if len(b) > 8 {
		return 0, errors.Errorf("%s is too long: BID fields are 32 bits", b)
	}
	return strconv.ParseInt(b, 16, 64)
}

// qualVersionToGsGlob converts contents of qual file to a glob expression of its .tbz2 file.
func qualVersionToGsGlob(qualVersion, fwName string) (string, error) {
	var releaseFormat, releaseBIDLockedFormat string

	switch fwName {
	case "cr50":
		releaseFormat = cr50Release
		releaseBIDLockedFormat = cr50BIDLockedRelease
	case "ti50":
		releaseFormat = ti50Release
		releaseBIDLockedFormat = ti50BIDLockedRelease
	default:
		return "", errors.Errorf("unknown fwName: %s", fwName)
	}

	m := reBIDLockedQualVersion.FindStringSubmatch(qualVersion)
	if m == nil {
		m = reQualVersion.FindStringSubmatch(qualVersion)
		if m == nil {
			return "", errors.New("qual version not recognized: " + qualVersion)
		}
		return fmt.Sprintf(releaseFormat, strings.TrimSuffix(m[1], "/")), nil
	}
	bIDMask, err := getBIDInt(m[3])
	if err != nil {
		return "", errors.Wrapf(err, "invalid bid mask: %s", m[3])
	}
	bIDFlags, err := getBIDInt(m[4])
	if err != nil {
		return "", errors.Wrapf(err, "invalid bid flags: %s", m[4])
	}
	return fmt.Sprintf(releaseBIDLockedFormat, m[1], m[2], bIDMask, bIDFlags), nil
}

// imageDir returns a cloud directory storing EFI and debug images signed for particular testbeds.
func imageDir(t ti50.TestbedType) string {
	if t == ti50.GscH1Shield {
		return fmt.Sprintf(DevGSCImageBucket, "cr50") + "h1_shield"
	} else if t == ti50.GscDTShield {
		return fmt.Sprintf(DevGSCImageBucket, "ti50") + "dt_shield"
	} else if t == ti50.GscDTShieldV2 {
		return fmt.Sprintf(DevGSCImageBucket, "ti50") + "dt_shield"
	} else if t == ti50.GscOTShield {
		return fmt.Sprintf(DevGSCImageBucket, "ti50") + "ot_shield"
	} else if t == ti50.GscNTShieldV2 {
		return fmt.Sprintf(DevGSCImageBucket, "ti50") + "ot_shield"
	} else if t == ti50.GscOpentitanCw310Fpga {
		return fmt.Sprintf(DevGSCImageBucket, "ti50") + "ot_fpga_cw310"
	} else {
		panic("Directory with EFI/Debug images not declared for testbed type: " + t)
	}
}

// findGSCImage finds the image with the given gsTemplate.
func findGSCImage(ctx context.Context, gsTemplate string) (string, error) {
	gsURL, err := gsLs(ctx, "list gsc images", gsTemplate)
	if err != nil || len(gsURL) != 1 {
		return "", errors.Errorf("failed to find GSC image: %s", gsTemplate)
	}

	return gsURL[0], nil
}

// findGSCDebugImage finds the debug image for gsc board.
func findGSCDebugImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties, ext string) (string, error) {
	devIds := strings.Split(testbedProperties.UsbSerial, "-")
	if len(devIds) != 2 {
		return "", errors.New("usb_serial parse error " + testbedProperties.UsbSerial)
	}

	imageDir := imageDir(testbedProperties.TestbedType)
	debugImageGlob := fmt.Sprintf(debugImageTemplate, ext, strings.ToLower(devIds[0]), strings.ToLower(devIds[1]))
	return findGSCImage(ctx, imageDir+"/"+debugImageGlob)
}

// findGSCEFIImage finds the eraseflashinfo image for gsc board.
func findGSCEFIImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties) (string, error) {
	imageDir := imageDir(testbedProperties.TestbedType)
	efiDevidStr := strings.ToLower(testbedProperties.UsbSerial)
	efiImageGlob := fmt.Sprintf(efiImageTemplate, efiDevidStr)
	return findGSCImage(ctx, imageDir+"/"+efiImageGlob)
}

// DownloadEfiImage finds the eraseflashinfo image for gsc board.
func DownloadEfiImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties) (string, error) {
	efiImageURL, err := findGSCEFIImage(ctx, testbedProperties)
	if err != nil {
		return "", errors.Wrap(err, "failed to find gsc efi image")
	}
	return DownloadToTempFile(ctx, "efi image", efiImageURL)
}

// DownloadDebugImage finds the debug image for gsc board.
func DownloadDebugImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties, ext string) (string, error) {
	debugImageURL, err := findGSCDebugImage(ctx, testbedProperties, ext)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find gsc debug %s image", ext)
	}
	return DownloadToTempFile(ctx, ext+"debug image", debugImageURL)
}

// DownloadGSCTestImages finds the debug and eraseflashinfo images for gsc board.
func DownloadGSCTestImages(ctx context.Context, testbedProperties remoteTi50.TestbedProperties) (string, string, error) {

	debugImage, err := DownloadDebugImage(ctx, testbedProperties, "")
	if err != nil {
		return "", "", errors.Wrap(err, "failed to download the debug image")
	}

	efiImage, err := DownloadEfiImage(ctx, testbedProperties)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to download the efi image")
	}
	return debugImage, efiImage, nil
}

// LookupGSCReleaseTarball downloads the image binary indicated by the version.
func LookupGSCReleaseTarball(ctx context.Context, version, fwName string) (string, error) {
	pat, err := qualVersionToGsGlob(version, fwName)
	if err != nil {
		return "", err
	}
	// Check for the image in the public bucket first.
	gsURL, err := findGSCImage(ctx, gsPrefix+qualBucket+pat)
	if err == nil {
		return gsURL, err
	}
	// Check for the image in the private bucket, if it's not found in the public one.
	return findGSCImage(ctx, gsPrefix+qualPrivateBucket+fwName+"/"+pat)
}

// lookupLatestGSCQualTarball downloads the image binary indicated in the qual file.
func lookupLatestGSCQualTarball(ctx context.Context, t ti50.TestbedType, isTi50a bool) (string, error) {
	fwName := FindFwName(t)
	qualFilePart := fwName
	if GetFwAndDevicePrefix(t) == "ti50-nt" {
		qualFilePart = "ti50-nt"
	}
	qualTarget := "chromeos"
	if isTi50a {
		qualTarget = "ti50a"
	}
	gsURL := gsPrefix + fmt.Sprintf(latestQualFile, qualTarget, qualFilePart)
	v, err := cmd(ctx, "read qual file", "gsutil", "cat", gsURL)
	if err != nil {
		return "", err
	}
	return LookupGSCReleaseTarball(ctx, v, fwName)
}

// ExtractGSCQualImageFromTarball extracts the image binary from the release archive.
func ExtractGSCQualImageFromTarball(ctx context.Context, tarball string) (string, error) {
	m := reGSCReleaseTarball.FindStringSubmatch(tarball)
	if m == nil {
		return "", errors.Errorf("Unable to find fw name in %s", tarball)
	}
	fwName := m[1]

	// extract image from tarball
	d, err := os.MkdirTemp("", fwName+"qual")
	if err != nil {
		return "", errors.Wrap(err, "create temp file for gsc qual image")
	}
	//tar -xf cr50.r0.0.12.w0.6.210_FFFF_00000000_00000010.tbz2 -C ./asdf
	if _, err := cmd(ctx, "extract qual image", "tar", "-xf", tarball, "-C", d); err != nil {
		return "", err
	}

	// check bin was extracted
	name := fwName + ".bin.prod"
	bin, err := cmd(ctx, "find extracted "+name, "find", d, "-name", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(bin), nil
}

// cmd runs cmd and returns the output.  Desc is used for logging and error description.
func cmd(ctx context.Context, desc, cmd string, args ...string) (string, error) {
	testing.ContextLogf(ctx, "%s: %s %s", desc, cmd, strings.Join(args, " "))
	c := exec.CommandContext(ctx, cmd, args...)
	output, err := c.CombinedOutput()
	if err != nil {
		err = errors.Wrap(err, desc)
	}
	return string(output), err
}

// gsURLExists retruns whether a gs URL is valid.
func gsURLExists(ctx context.Context, url string) bool {
	output, err := cmd(ctx, url+" exists?", "gsutil", "ls", url)
	if err != nil {
		testing.ContextLogf(ctx, "gsURLExists err (%v): %v", err, output)
	}
	return err == nil
}

// gsLs finds urls matching expr and returns them as a list
func gsLs(ctx context.Context, desc, expr string) ([]string, error) {
	output, err := cmd(ctx, "Listing "+desc, "gsutil", "ls", expr)
	if err != nil {
		return nil, errors.Wrap(err, "listing "+desc)
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n"), nil
}

// ti50ImageDirectory returns the image directory under the tast folder.
func ti50ImageDirectory(t ti50.TestbedType, i ImageType) (string, error) {
	n := ti50ImageTypeToProject(i)

	switch t {
	case ti50.GscDTAndreiboard:
		fallthrough
	case ti50.GscDTShield:
		fallthrough
	case ti50.GscDTShieldV2:
		return "dt-" + n, nil
	case ti50.GscOTShield:
		fallthrough
	case ti50.GscNTShieldV2:
		return "nt-" + n, nil
	case ti50.GscOpentitanCw310Fpga:
		return "nuvotitan_cw310_a1-" + n, nil
	case ti50.GscHostEmulation:
		return "he-" + n, nil
	default:
		return "", errors.New("unknown ti50 testbed type: " + string(t))
	}
}

// FindFwName returns the firmware name for the board.
func FindFwName(t ti50.TestbedType) string {
	if t == ti50.GscH1Shield {
		return "cr50"
	}
	return "ti50"
}

// GetFwAndDevicePrefix returns ti50-dt, ti50-nt, or cr50
func GetFwAndDevicePrefix(t ti50.TestbedType) string {
	switch t {
	case ti50.GscDTAndreiboard:
		fallthrough
	case ti50.GscDTShield:
		fallthrough
	case ti50.GscDTShieldV2:
		return "ti50-dt"
	case ti50.GscOTShield:
		fallthrough
	case ti50.GscOpentitanCw310Fpga:
		fallthrough
	case ti50.GscNTShieldV2:
		return "ti50-nt"
	case ti50.GscHostEmulation:
		return "ti50-he"
	default:
		return FindFwName(t)
	}
}

// ti50ImageTypeToProject returns the project.
func ti50ImageTypeToProject(i ImageType) string {
	if i == SystemImage {
		return "ti50"
	}
	return string(i)
}

func testbedTypeToChip(testbedType ti50.TestbedType) string {
	m := reTestbedTypeParts.FindStringSubmatch(string(testbedType))
	if m == nil {
		return ""
	}
	return m[1]
}

// defaultConfigPath determines the chroot path of fw config json files base on testbed and image types.
func defaultConfigPath(s *testing.FixtState, testbedType ti50.TestbedType, imageType ImageType) string {
	var fw, c string

	c = testbedTypeToChip(testbedType)
	if c == "" {
		s.Fatal("Unable to determine chip from testbedType: ", testbedType)
	}
	if c == "nt" {
		// nt shares fw config with ot.
		c = "ot"
	}

	switch imageType {
	case SystemImage, SystemTestAutoImage, SystemTestAuto2Image, SystemTestAutoAImage, Ti50AImage:
		fw = FindFwName(testbedType)
	default:
		s.Fatal("Unknown image type: ", string(imageType))
	}

	dataFile := fw + "_" + c + ".json"

	return s.DataPath(dataFile)
}
