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
	"time"

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

	// FwConfigJSON is the arg name for the json configuration file (for use in case buildurl
	// specifies a single .bin file, rather than a directory).
	FwConfigJSON = "fw_configjson"

	// Chip is an arg name for specifying Ti50 target.
	Chip = "chip"

	// Variant is an arg name for specifying Ti50 target.
	Variant = "variant"

	// Slot can be left empty, or set to either 'A' or 'B'.
	Slot = "slot"

	// imageBin is the name of the image file, it is the same for both images.
	// It may be signed with the PrePVT or Nightly target
	//     - ti50_Unknown_PrePVT_ti50-accessory-nodelocked-ro-premp.bin
	//     - ti50_Unknown_Nightly_ti50-accessory-nodelocked-ro-premp.bin
	imageBin = "ti50_Unknown_*_ti50-accessory-nodelocked-ro-premp.bin"

	// branchImageBin is used instead of imageBin on branch builders.
	branchImageBin = "ti50_Unknown_PrePVT_ti50-accessory-mp.bin"

	// tastImageGlob matches images in tast/<testbedtype>-<imagetype>/.
	tastImageGlob = "image*.bin"

	// tastConfigGlob matches fw config json files in tast/<testbedtype>-<imagetype>/.
	tastConfigGlob = "opentitantool_fw_config.json"

	gsPrefix = "gs://"

	// ToTBranch is the Tip-of-Tree branch having artifacts at postSubmitArtifactsBuilder.
	ToTBranch                  string = "tot"
	postSubmitArtifactsBuilder        = "chromeos-image-archive/firmware-ti50-postsubmit"

	// Cr50QualBranch is the latest qual candidate for Cr50
	Cr50QualBranch string = "cr50qual"
	// Ti50QualBranch is the latest qual candidate for Ti50
	Ti50QualBranch string = "ti50qual"

	// Remote image paths
	latestQualFile     = "chromeos-localmirror-private/distfiles/chromeos-%s-QUAL_VERSION"
	debugImageTemplate = "gs://chromeos-localmirror-private/distfiles/chromeos-%s*/*_shield/*.dbg.0x%s_0x%s.bin.*"
	efiImageTemplate   = "gs://chromeos-localmirror-private/distfiles/chromeos-%s*/*_shield/*_Unknown_NodeLocked-%s_*-accessory-mp.bin"
	qualPrivateBucket  = "chromeos-localmirror-private/distfiles/"
	qualBucket         = "chromeos-localmirror/distfiles/"

	// GSC filenames
	cr50Release          = "cr50.r0.0.*.w%s.tbz2"
	cr50BIDLockedRelease = "cr50.r0.0.*.w%s_%s_%08x_%08x.tbz2"
	// Ti50 changed formats. This works with old and new versions.
	ti50Release          = "ti50.r*w*%s.tar.xz"
	ti50BIDLockedRelease = "ti50.r*w*%s_%s_%08x_%08x.tar.xz"

	imageDownloadTimeout = 30 * time.Second
	imageDeleteTimeout   = 5 * time.Second
)

// ImageType declarations, please update AllTi50ImageTypes() after editing.
const (
	// SystemImage fixture downloads the system image bin used by ChromeOS DUTs.
	SystemImage ImageType = "default"

	// SystemTestAutoImage fixture downloads the system_test_auto image bin.
	SystemTestAutoImage ImageType = "system_test_auto"

	// SystemTestAuto2Image fixture downloads the system_test_auto_2 image bin.
	SystemTestAuto2Image ImageType = "system_test_auto_2"
)

var (
	// defaultFwConfigs are fw config files packaged in the data directory.
	defaultFwConfigs = []string{"cr50_h1.json", "ti50_dt.json", "ti50_he.json", "ti50_ot.json"}

	// reQualVersion extracts relevant contents of qual files.
	reQualVersion = regexp.MustCompile(`(.*)`)
	// reBIDLockedQualVersion extracts relevant contents of qual files.
	reBIDLockedQualVersion = regexp.MustCompile(`(.*)/(.*):(.*):(.*)`)
	// reGSCReleaseTarball extracts the firmware name from the tarball filename.
	reGSCReleaseTarball = regexp.MustCompile(`(cr50|ti50).*(tar.xz|tbz2)`)

	// reTestbedTypeParts extracts relevant parts of the testbed type string.
	reTestbedTypeParts = regexp.MustCompile(`gsc_([[:alnum:]]*)`)
)

// AllTi50TestbedTypes returns all the testbed types that use ti50 images.
func AllTi50TestbedTypes() []ti50.TestbedType {
	return []ti50.TestbedType{ti50.GscDTAndreiboard, ti50.GscDTShield, ti50.GscOpentitanCw310Fpga, ti50.GscHostEmulation, ti50.GscOTShield}
}

// AllTi50ImageTypes returns all the ti50 image types.
func AllTi50ImageTypes() []ImageType {
	return []ImageType{SystemImage, SystemTestAutoImage, SystemTestAuto2Image}
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
	inputURL, _ := s.Var(BuildURL)
	iv := &ImageValue{}

	// For inputURL that is in the form of latest-*, convert it to the corresponding GS path.
	if strings.HasPrefix(inputURL, LatestPrefix) {
		var latestURL string
		var err error
		branch := inputURL[len(LatestPrefix):]
		switch branch {
		case ToTBranch:
			// Special value "latests-tot" finds the most recent complete set of artifacts,
			// and then goes into the case below.
			latestURL, err = findLatestCompletedTi50PostsubmitBuildURL(ctx)
			if err != nil {
				return nil, err
			}
			testing.ContextLogf(ctx, "Found %s for %s", latestURL, inputURL)
		case Cr50QualBranch:
			latestURL, err = lookupLatestGSCQualTarball(ctx, "cr50")
			if err != nil {
				return nil, err
			}
		case Ti50QualBranch:
			latestURL, err = lookupLatestGSCQualTarball(ctx, "ti50")
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("unrecognized branch " + branch)
		}
		inputURL = latestURL
	}

	// For inputURL that is in the form of gs://*.tbz2, convert it to a local file by downloading.
	if strings.HasPrefix(inputURL, gsPrefix) && reGSCReleaseTarball.FindStringSubmatch(inputURL) != nil {
		downloadedFile, err := downloadToTempFile(ctx, "tbz2", inputURL)
		if err != nil {
			return nil, err
		}
		inputURL = downloadedFile
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
			tastURL := gsPrefix + filepath.Join(inputURL[len(gsPrefix):], "tast")

			testing.ContextLogf(ctx, "Looking for tast directory %s", tastURL)
			if gsURLExists(ctx, tastURL) {
				imageDir, err := ti50ImageDirectory(testbedProperties.TestbedType, imageType)
				if err != nil {
					return nil, err
				}
				// Cloud directory (branch or main) has a "tast/" subdirectory,
				// use images from there.
				tastDir := filepath.Join(inputURL[len(gsPrefix):], "tast", imageDir)
				fullURL = gsPrefix + filepath.Join(tastDir, tastImageGlob)
				jsonURL = gsPrefix + filepath.Join(tastDir, tastConfigGlob)
			} else {
				// Legacy artifact directory structure.
				// Assume branch builds have a -channel in the URL.
				var subDir string
				bin := branchImageBin
				if !strings.Contains(inputURL, "-channel/") {
					p := ti50ImageTypeToProject(imageType)
					// Postsubmit builder images are 1 subdir deeper.
					subDir = p + ".tar.bz2"
					bin = imageBin
				}
				fullURL = gsPrefix + filepath.Join(inputURL[len(gsPrefix):], subDir, bin)
			}
			testing.ContextLogf(ctx, "Found tast directory %s", fullURL)
		}

		downloadedBin, err := downloadToTempFile(ctx, "image bin", fullURL)
		if err != nil {
			return nil, err
		}
		iv.imagePath = downloadedBin

		if jsonURL != "" {
			downloadedJSON, err := downloadToTempFile(ctx, "fw conf json", jsonURL)
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
func findLatestCompletedTi50PostsubmitBuildURL(ctx context.Context) (string, error) {
	builds, err := gsLs(ctx, "builds for tot", gsPrefix+postSubmitArtifactsBuilder)
	if err != nil {
		return "", err
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
		scannedDirs := make(map[string]bool)
		for _, boardType := range AllTi50TestbedTypes() {
			for _, imageType := range AllTi50ImageTypes() {
				dir, err := ti50ImageDirectory(boardType, imageType)
				if err != nil {
					return "", err
				}
				if _, ok := scannedDirs[dir]; ok {
					continue
				}
				scannedDirs[dir] = true
				artifactsDir := build + filepath.Join("tast", dir) + "/"

				for _, g := range []string{tastImageGlob, tastConfigGlob} {
					if !gsURLExists(ctx, artifactsDir+g) {
						testing.ContextLogf(ctx, "Rejecting %s: %s missing", artifactsDir, g)
						continue Loop
					}
				}
			}
		}
		return build, nil
	}
	return "", errors.New("found no completed builds for tot")
}

// downloadToTempFile downloads url (gs) to a temp file.
func downloadToTempFile(ctx context.Context, desc, url string) (string, error) {
	ext := filepath.Ext(url)
	f, err := os.CreateTemp("", "*"+ext)
	if err != nil {
		return "", errors.Wrap(err, "create temp file for "+desc)
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

// findGSCImage finds the image with the given gsTemplate.
func findGSCImage(ctx context.Context, gsTemplate string) (string, error) {
	gsURL, err := gsLs(ctx, "list gsc images", gsTemplate)
	if err != nil || len(gsURL) != 1 {
		return "", errors.Errorf("failed to find GSC image: %s", gsTemplate)
	}

	return gsURL[0], nil
}

// findGSCDebugImage finds the debug image for cr50 board.
func findGSCDebugImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties, fwName string) (string, error) {
	devIds := strings.Split(testbedProperties.UsbSerial, "-")
	if len(devIds) != 2 {
		return "", errors.New("usb_serial parse error " + testbedProperties.UsbSerial)
	}

	debugImageGlob := fmt.Sprintf(debugImageTemplate, fwName, strings.ToLower(devIds[0]), strings.ToLower(devIds[1]))
	return findGSCImage(ctx, debugImageGlob)
}

// findGSCEFIImage finds the eraseflashinfo image for cr50 board.
func findGSCEFIImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties, fwName string) (string, error) {
	efiDevidStr := strings.ToLower(testbedProperties.UsbSerial)
	efiImageGlob := fmt.Sprintf(efiImageTemplate, fwName, efiDevidStr)
	return findGSCImage(ctx, efiImageGlob)
}

// DownloadGSCTestImages finds the debug and eraseflashinfo images for gsc board.
func DownloadGSCTestImages(ctx context.Context, testbedProperties remoteTi50.TestbedProperties, fwName string) (string, string, error) {
	// Download the debug image.
	debugImageURL, err := findGSCDebugImage(ctx, testbedProperties, fwName)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to find gsc debug image")
	}
	debugImage, err := downloadToTempFile(ctx, "debug image", debugImageURL)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to download the debug image")
	}

	// Download the eraseflashinfo image.
	efiImageURL, err := findGSCEFIImage(ctx, testbedProperties, fwName)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to find gsc efi image")
	}
	efiImage, err := downloadToTempFile(ctx, "efi image", efiImageURL)
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
func lookupLatestGSCQualTarball(ctx context.Context, fwName string) (string, error) {
	gsURL := gsPrefix + fmt.Sprintf(latestQualFile, fwName)
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
	output, err := c.Output()
	if err != nil {
		err = errors.Wrap(err, desc)
	}
	return string(output), err
}

// gsURLExists retruns whether a gs URL is valid.
func gsURLExists(ctx context.Context, url string) bool {
	_, err := cmd(ctx, url+" exists?", "gsutil", "ls", url)
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
		return "andreiboard-" + n, nil
	case ti50.GscOTShield:
		fallthrough
	case ti50.GscOpentitanCw310Fpga:
		return "opentitan-" + n, nil
	case ti50.GscHostEmulation:
		return "host_emulation-" + n, nil
	default:
		return "", errors.New("unknown ti50 testbed type: " + string(t))
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

	switch imageType {
	case SystemImage, SystemTestAutoImage, SystemTestAuto2Image:
		if c == "h1" {
			fw = "cr50"
		} else {
			fw = "ti50"
		}
	default:
		s.Fatal("Unknown image type: ", string(imageType))
	}

	dataFile := fw + "_" + c + ".json"

	return s.DataPath(dataFile)
}
