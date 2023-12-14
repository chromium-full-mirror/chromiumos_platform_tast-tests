// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"io/ioutil"
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
	Cr50QualBranch     string = "cr50qual"
	cr50LatestQualFile        = "chromeos-localmirror-private/distfiles/chromeos-cr50-QUAL_VERSION"
	cr50QualFolder            = "chromeos-localmirror-private/distfiles/cr50"

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
	iv.imagePath = inputURL

	var hasManualConf bool
	if conf, ok := s.Var(FwConfigJSON); ok {
		iv.configPaths = []string{conf}
		hasManualConf = true
	}

	if inputURL == "" {
		// Absence of BuildURL argument.  This instructs tast to not flash any image to
		// the devboard, but run the test against the code already running.  This works
		// only if the image on the board is of the same type (system_test_auto / ti50) as
		// the test case expects.  A json configuration file will be selected from
		// ti50/common based on the information from devboardservice and the type of image
		// declared on the test case.
		testing.ContextLogf(ctx, "-var=%s= not provided, assuming the devboard has a %s image", BuildURL, imageType)

		if !hasManualConf {
			config := defaultConfigPath(s, testbedProperties.TestbedType, imageType)
			testing.ContextLogf(ctx, "-var=%s= not provided, using default at %s", FwConfigJSON, config)
			iv.configPaths = []string{config}
		}

		return iv, nil
	}

	if strings.HasPrefix(inputURL, LatestPrefix) {
		branch := inputURL[len(LatestPrefix):]
		switch branch {
		case ToTBranch:
			// Special value "latests-tot" finds the most recent complete set of artifacts,
			// and then goes into the case below.
			latestURL, err := findLatestCompletedTi50PostsubmitBuildURL(ctx)
			if err != nil {
				return nil, err
			}
			testing.ContextLogf(ctx, "Found %s for %s", latestURL, inputURL)
			iv.imagePath = latestURL
		case Cr50QualBranch:
			latestURL, err := downloadLatestCr50QualImage(ctx)
			if err != nil {
				return nil, err
			}
			iv.imagePath = latestURL

			if !hasManualConf {
				config := defaultConfigPath(s, testbedProperties.TestbedType, imageType)
				testing.ContextLogf(ctx, "-var=%s= not provided, using default at %s", FwConfigJSON, config)
				iv.configPaths = []string{config}
			}
		default:
			return nil, errors.New("unrecognized branch " + branch)
		}

		return iv, nil
	}

	if strings.HasPrefix(inputURL, gsPrefix) {
		// BuildURL pointing to artifacts via a Google Storage URL.  The logic below
		// selects an image and corresponding json file depending on the type needed by
		// the test case.
		fullURL := inputURL
		jsonURL := ""
		// Assume URL is a build folder if it doesn't end in .bin.
		if !strings.HasSuffix(inputURL, ".bin") {
			tastURL := gsPrefix + filepath.Join(inputURL[len(gsPrefix):], "tast")
			args := []string{"ls", tastURL}
			testing.ContextLogf(ctx, "Looking for tast directory: gsutil %s", strings.Join(args, " "))
			cmd := exec.CommandContext(ctx, "gsutil", args...)
			if err := cmd.Run(); err == nil {

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
		f, err := ioutil.TempFile("", "*.bin")
		if err != nil {
			return nil, errors.Wrap(err, "create temp image file")
		}
		f.Close()

		{
			args := []string{"cp", fullURL, f.Name()}
			testing.ContextLogf(ctx, "Download image: gsutil %s", strings.Join(args, " "))
			cmd := exec.CommandContext(ctx, "gsutil", args...)
			if err := cmd.Run(); err != nil {
				return nil, errors.Wrapf(err, "download %q", fullURL)
			}
			iv.imagePath = f.Name()
		}

		if !hasManualConf && jsonURL != "" {
			jsonf, err := ioutil.TempFile("", "*.json")
			if err != nil {
				return nil, errors.Wrap(err, "create temp json file")
			}
			jsonf.Close()
			iv.configPaths = []string{jsonf.Name()}

			args := []string{"cp", jsonURL, jsonf.Name()}
			testing.ContextLogf(ctx, "Download conf: gsutil %s", strings.Join(args, " "))
			cmd := exec.CommandContext(ctx, "gsutil", args...)
			if err := cmd.Run(); err != nil {
				return nil, errors.Wrapf(err, "download %q", jsonURL)
			}
		}
		return iv, nil
	}

	img, err := os.Stat(inputURL)
	if err != nil {
		return nil, err
	}
	if img.IsDir() {
		// BuildURL is a local directory, which must be a ti50/common checkout.  Code
		// below will select an image file within the build/ directory and json file
		// within the ports/ directory, based on the type of image required by the test
		// case and chip/variant.
		var name string
		slot, _ := s.Var(Slot)
		chip, _ := s.Var(Chip)
		variant, _ := s.Var(Variant)
		if slot != "" {
			name = "full_image." + slot + ".signed.bin"
		} else {
			name = "full_image.signed.bin"
		}
		if testbedProperties.TestbedType == "gsc_he" {
			chip = "host_emulation"
			variant = "host_emulation"
			name = "image.A.bin"
		}

		p := ti50ImageTypeToProject(imageType)

		iv.imagePath = filepath.Join(inputURL, "build", p, chip, variant, name)
		if !hasManualConf {
			testing.ContextLogf(ctx, "-var=%s= not provided, using directory default", FwConfigJSON)
			iv.configPaths = []string{filepath.Join(inputURL, "ports", chip, "software", "tools", p+"_"+chip+".json")}
		}
		return iv, nil
	}

	return iv, nil
}

// findLatestCompletedTi50PostsubmitBuildURL finds the most recent build with the full set of image artifacts.
func findLatestCompletedTi50PostsubmitBuildURL(ctx context.Context) (string, error) {
	branchGsPrefix := gsPrefix + postSubmitArtifactsBuilder

	args := []string{"ls", branchGsPrefix}
	testing.ContextLogf(ctx, "Listing builds for tot: gsutil %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "gsutil", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	builds := strings.Split(string(output), "\n")
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
		for _, boardType := range ti50.AllTestbedTypes() {
			for _, imageType := range AllTi50ImageTypes() {
				dir, err := ti50ImageDirectory(boardType, imageType)
				if err != nil {
					return "", err
				}
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

// downloadLatestCr50QualImage downloads the image binary indicated in the qual file.
func downloadLatestCr50QualImage(ctx context.Context) (string, error) {
	v, err := cmd(ctx, "read qual file", "gsutil", "cat", gsPrefix+cr50LatestQualFile)
	if err != nil {
		return "", err
	}

	pat, err := qualVersionToGsGlob(v, "cr50")
	if err != nil {
		return "", err
	}

	// find unique qual image tbz2
	urls, err := gsLs(ctx, "cr50 quals", gsPrefix+cr50QualFolder+"/"+pat)
	if err != nil {
		return "", err
	}
	if len(urls) != 1 {
		return "", errors.Errorf("non-unique qual image: %v", urls)
	}

	// download qual image tbz2
	tbz2, err := downloadToTempFile(ctx, "cr50 qual tbz2", urls[0])
	if err != nil {
		return "", err
	}
	defer os.Remove(tbz2)

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
	args := []string{"ls", url}
	cmd := exec.CommandContext(ctx, "gsutil", args...)
	return cmd.Run() == nil
}

// gsLs finds urls matching expr and returns them as a list
func gsLs(ctx context.Context, desc, expr string) ([]string, error) {
	args := []string{"ls", expr}
	testing.ContextLogf(ctx, "Listing %s: gsutil %s", desc, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "gsutil", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "listing "+desc)
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n"), nil
}

// ti50ImageDirectory returns the image directory under the tast folder.
func ti50ImageDirectory(t ti50.TestbedType, i ImageType) (string, error) {
	n := ti50ImageTypeToProject(i)

	switch t {
	case "gsc_dt_ab":
		fallthrough
	case "gsc_dt_shield":
		return "andreiboard-" + n, nil
	case "gsc_ot_fpga_cw310":
		return "opentitan-" + n, nil
	case "gsc_he":
		return "host_emulation-" + n, nil
	default:
		return "", errors.New("unknown testbed type: " + string(t))
	}
}

// ti50ImageTypeToProject returns the project.
func ti50ImageTypeToProject(i ImageType) string {
	if i == SystemImage {
		return "ti50"
	}
	return string(i)
}

// defaultConfigPath determines the chroot path of fw config json files base on testbed and image types.
func defaultConfigPath(s *testing.FixtState, testbedType ti50.TestbedType, imageType ImageType) string {
	var fw, c string

	m := reTestbedTypeParts.FindStringSubmatch(string(testbedType))
	if m == nil {
		s.Fatal("Unable to determine chip from testbedType: ", testbedType)
	}
	c = m[1]

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
