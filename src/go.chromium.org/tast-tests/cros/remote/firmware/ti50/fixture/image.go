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
	"strings"
	"time"

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

	// FwConfigJSON is the arg name for the json configuration file (for use in case buildurl
	// specifies a single .bin file, rather than a directory).
	FwConfigJSON = "fw_configjson"

	// Chip is an arg name for specifying Ti50 target.
	Chip = "chip"

	// Variant is an arg name for specifying Ti50 target.
	Variant = "variant"

	// Slot can be left empty, or set to either 'A' or 'B'.
	Slot = "slot"

	// Ti50Image fixture downloads the ti50 image bin.
	Ti50Image ImageType = "ti50"

	// SystemTestAutoImage fixture downloads the system_test_auto image bin.
	SystemTestAutoImage ImageType = "system_test_auto"

	// SystemTestAuto2Image fixture downloads the system_test_auto_2 image bin.
	SystemTestAuto2Image ImageType = "system_test_auto_2"

	// imageBin is the name of the image file, it is the same for both images.
	imageBin = "ti50_Unknown_PrePVT_ti50-accessory-nodelocked-ro-premp.bin"

	// branchImageBin is used instead of imageBin on branch builders.
	branchImageBin = "ti50_Unknown_PrePVT_ti50-accessory-mp.bin"

	gsPrefix = "gs://"

	imageDownloadTimeout = 30 * time.Second
	imageDeleteTimeout   = 5 * time.Second
)

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
	if conf, ok := s.Var(FwConfigJSON); ok {
		configPaths = append(configPaths, conf)
	}

	if inputURL == "" {
		testing.ContextLogf(ctx, "-var=%s= not provided, assuming the devboard has a %s image", BuildURL, imageType)
		iv.imagePath = ""
		iv.configPaths = configPaths
		return iv, nil
	}

	if len(inputURL) > len(gsPrefix) && inputURL[:len(gsPrefix)] == gsPrefix {
		fullURL := inputURL
		jsonURL := ""
		// Assume URL is a build folder if it doesn't end in .bin.
		if inputURL[len(inputURL)-4:] != ".bin" {
			tastURL := gsPrefix + filepath.Join(inputURL[len(gsPrefix):], "tast")
			args := []string{"ls", tastURL}
			testing.ContextLogf(ctx, "Looking for tast directory: gsutil %s", strings.Join(args, " "))
			cmd := exec.CommandContext(ctx, "gsutil", args...)
			if err := cmd.Run(); err == nil {
				// Cloud directory (branch or main) has a "tast/" subdirectory,
				// use images from there.

				var prefix string
				switch testbedProperties.TestbedType {
				case "gsc_dt_ab":
					prefix = "andreiboard-"
				case "gsc_ot_fpga_cw310":
					prefix = "opentitan-"
				case "gsc_he":
					prefix = "host_emulation-"
				default:
					return nil, errors.Errorf("unknown testbed type: %q", testbedProperties.TestbedType)
				}
				tastDir := filepath.Join(inputURL[len(gsPrefix):], "tast", prefix+string(imageType))
				fullURL = gsPrefix + filepath.Join(tastDir, "image*.bin")
				jsonURL = gsPrefix + filepath.Join(tastDir, "opentitantool_fw_config.json")
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
		// Given directory is assumed to have ports/ and build/ subdirectories, that is,
		// be ti50/common.
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
