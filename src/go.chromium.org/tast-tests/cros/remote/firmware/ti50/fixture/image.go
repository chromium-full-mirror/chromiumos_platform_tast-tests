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

	imageDownloadTimeout = 30 * time.Second
	imageDeleteTimeout   = 5 * time.Second
)

// ImageType declarations, please update AllImageTypes() after editing.
const (
	// Ti50Image fixture downloads the ti50 image bin.
	Ti50Image ImageType = "ti50"

	// SystemTestAutoImage fixture downloads the system_test_auto image bin.
	SystemTestAutoImage ImageType = "system_test_auto"

	// SystemTestAuto2Image fixture downloads the system_test_auto_2 image bin.
	SystemTestAuto2Image ImageType = "system_test_auto_2"
)

// AllImageTypes returns all the possible image types.
func AllImageTypes() []ImageType {
	return []ImageType{Ti50Image, SystemTestAutoImage, SystemTestAuto2Image}
}

// ImageValue provides access to a image binary along with json configuration files.
type ImageValue struct {
	imagePath   string
	configPaths []string

	// If downloaded is true, it means that the imagePath and each of the configPaths are
	// temporary files, which should be eventually deleted by the caller.
	downloaded bool
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
	iv := &ImageValue{downloaded: false}

	var configPaths []string
	conf, hasManualConf := s.Var(FwConfigJSON)
	if hasManualConf {
		configPaths = append(configPaths, conf)
	}

	if inputURL == "" {
		// Absence of BuildURL argument.  This instructs tast to not flash any image to
		// the devboard, but run the test against the code already running.  This works
		// only if the image on the board is of the same type (system_test_auto / ti50) as
		// the test case expects.  A json configuration file will be selected from
		// ti50/common based on the information from devboardservice and the type of image
		// declared on the test case.
		testing.ContextLogf(ctx, "-var=%s= not provided, assuming the devboard has a %s image", BuildURL, imageType)
		iv.imagePath = ""

		if !hasManualConf {
			testing.ContextLogf(ctx, "-var=%s= not provided, using default from ti50/common", FwConfigJSON)
			config, err := defaultConfigPath(testbedProperties.TestbedType, imageType)
			if err != nil {
				return nil, err
			}
			configPaths = append(configPaths, config)
		}
		iv.configPaths = configPaths

		return iv, nil
	}

	if strings.HasPrefix(inputURL, LatestPrefix) {
		// Special value "latests-tot" finds the most recent complete set of artifacts,
		// and then goes into the case below.
		latestURL, err := findLatestCompletedBuildURL(ctx, inputURL[len(LatestPrefix):])
		if err != nil {
			return nil, err
		}
		testing.ContextLogf(ctx, "Found %s for %s", latestURL, inputURL)
		inputURL = latestURL
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

				imageDir, err := imageDirectory(testbedProperties.TestbedType, imageType)
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
					// Postsubmit builder images are 1 subdir deeper.
					subDir = string(imageType) + ".tar.bz2"
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

		if jsonURL != "" {
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
		iv.downloaded = true
		return iv, nil
	}

	img, err := os.Stat(inputURL)
	if err != nil {
		return nil, err
	}
	if img.IsDir() {
		// BuildURL is a local directory, which must be a ti50/common checkout.  Code
		// below will select an image file within the build/ directory and json file
		// within the oports/ directory, based on the type of image required by the test
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
		iv.imagePath = filepath.Join(inputURL, "build", string(imageType), chip, variant, name)
		iv.configPaths = []string{filepath.Join(inputURL, "ports", chip, "software", "tools", string(imageType)+"_"+chip+".json")}
		return iv, nil
	}

	iv.imagePath = inputURL
	iv.configPaths = configPaths
	return iv, nil
}

// findLatestCompletedBuildURL finds the most recent build with the full set of image artifacts.
func findLatestCompletedBuildURL(ctx context.Context, branch string) (string, error) {
	var branchGsPrefix string

	switch branch {
	case ToTBranch:
		branchGsPrefix = gsPrefix + postSubmitArtifactsBuilder
	default:
		return "", errors.New("unrecognied branch " + branch)
	}

	args := []string{"ls", branchGsPrefix}
	testing.ContextLogf(ctx, "Listing builds for %s: gsutil %s", branch, strings.Join(args, " "))
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
			for _, imageType := range AllImageTypes() {
				dir, err := imageDirectory(boardType, imageType)
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
	return "", errors.New("found no completed builds for " + branch)
}

// gsURLExists retruns whether a gs URL is valid.
func gsURLExists(ctx context.Context, url string) bool {
	args := []string{"ls", url}
	cmd := exec.CommandContext(ctx, "gsutil", args...)
	return cmd.Run() == nil
}

// imageDirectory returns the image directory under the tast folder.
func imageDirectory(t ti50.TestbedType, i ImageType) (string, error) {
	switch t {
	case "gsc_dt_ab":
		fallthrough
	case "gsc_dt_shield":
		return "andreiboard-" + string(i), nil
	case "gsc_ot_fpga_cw310":
		return "opentitan-" + string(i), nil
	case "gsc_he":
		return "host_emulation-" + string(i), nil
	default:
		return "", errors.New("unknown testbed type: " + string(t))
	}
}

func defaultConfigPath(testbedType ti50.TestbedType, imageType ImageType) (string, error) {
	switch testbedType {
	case "gsc_dt_ab":
		fallthrough
	case "gsc_dt_shield":
		return "/mnt/host/source/src/platform/ti50/common/ports/dauntless/software/tools/" + string(imageType) + "_dauntless.json", nil
	case "gsc_ot_fpga_cw310":
		return "/mnt/host/source/src/platform/ti50/common/ports/opentitan/software/tools/" + string(imageType) + "_opentitan.json", nil
	case "gsc_he":
		return "/mnt/host/source/src/platform/ti50/common/ports/host_emulation/software/tools/" + string(imageType) + "_host_emulation.json", nil
	default:
		return "", errors.New("unknown testbed type: " + string(testbedType))
	}
}
