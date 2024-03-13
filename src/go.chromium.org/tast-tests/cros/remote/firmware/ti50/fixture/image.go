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
	imageBin = "ti50_Unknown_PrePVT_ti50-accessory-nodelocked-ro-premp.bin"

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
	Cr50QualBranch         string = "cr50qual"
	cr50LatestQualFile            = "chromeos-localmirror-private/distfiles/chromeos-cr50-QUAL_VERSION"
	cr50QualFolder                = "chromeos-localmirror-private/distfiles/cr50"
	cr50DebugImageTemplate        = "gs://chromeos-localmirror-private/distfiles/chromeos-cr50-debug-0.0.11/h1_shield/cr50.dbg.0x%s_0x%s.bin.*"

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
	reQualVersion = regexp.MustCompile(`(.*)/(.*):(.*):0x(.*)`)

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
			latestURL, err = lookupLatestCr50QualTbz2(ctx)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("unrecognized branch " + branch)
		}
		inputURL = latestURL
	}

	// For inputURL that is in the form of gs://*.tbz2, convert it to a local file by downloading.
	if strings.HasPrefix(inputURL, gsPrefix) && strings.HasSuffix(inputURL, ".tbz2") {
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
			// Extract tbz2 archive
		} else if strings.HasSuffix(inputURL, ".tbz2") {
			chip := testbedTypeToChip(testbedProperties.TestbedType)
			if chip != "h1" {
				return nil, errors.New("testbedType must be h1 for .tbz2 url: " + chip)
			}
			extractedFile, err := extractCr50QualImageFromTbz2(ctx, inputURL)
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
		return "", err
	}
	return f.Name(), nil
}

// qualVersionToGsGlob converts contents of qual file to a glob expression of its .tbz2 file.
func qualVersionToGsGlob(qualVersion, prefix string) (string, error) {
	m := reQualVersion.FindStringSubmatch(qualVersion)
	if m == nil {
		return "", errors.New("qual version not recognized: " + qualVersion)
	}
	return prefix + ".*.w" + m[1] + "_" + m[2] + "_" + m[3] + "_" + m[4] + ".tbz2", nil
}

// findGSCImage finds the image with the given gsTemplate.
func findGSCImage(ctx context.Context, gsTemplate string) (string, error) {
	gsURL, err := gsLs(ctx, "list gsc images", gsTemplate)
	if err != nil || len(gsURL) != 1 {
		return "", errors.New("find gsc image")
	}

	return gsURL[0], nil
}

// findCr50DebugImage finds the debug image for cr50 board.
func findCr50DebugImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties) (string, error) {
	devIds := strings.Split(testbedProperties.UsbSerial, "-")
	if len(devIds) != 2 {
		return "", errors.New("usb_serial parse error " + testbedProperties.UsbSerial)
	}

	debugImageGlob := fmt.Sprintf(cr50DebugImageTemplate, strings.ToLower(devIds[0]), strings.ToLower(devIds[1]))
	return findGSCImage(ctx, debugImageGlob)
}

// lookupLatestCr50QualTbz2 downloads the image binary indicated in the qual file.
func lookupLatestCr50QualTbz2(ctx context.Context) (string, error) {
	v, err := cmd(ctx, "read qual file", "gsutil", "cat", gsPrefix+cr50LatestQualFile)
	if err != nil {
		return "", err
	}

	pat, err := qualVersionToGsGlob(v, "cr50")
	if err != nil {
		return "", err
	}
	return findGSCImage(ctx, gsPrefix+cr50QualFolder+"/"+pat)
}

// extractCr50QualImageFromTbz2 extracts the image binary from bz2 archive.
func extractCr50QualImageFromTbz2(ctx context.Context, tbz2 string) (string, error) {
	// extract qual image tbz2
	d, err := os.MkdirTemp("", "cr50qual")
	if err != nil {
		return "", errors.Wrap(err, "create temp file for cr50 qual image")
	}
	//tar -jxvf cr50.r0.0.12.w0.6.210_FFFF_00000000_00000010.tbz2 -C ./asdf
	if _, err := cmd(ctx, "extract qual image", "tar", "-jxvf", tbz2, "-C", d); err != nil {
		return "", err
	}

	// check bin was extracted
	bin, err := cmd(ctx, "find extracted cr50.bin.prod", "find", d, "-name", "cr50.bin.prod")
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
