// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dututils provides set of util functions used to work with ARC version properties.
package dututils

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

// BuildDescriptor contains essential parameters of ARC Android image taken from test device.
type BuildDescriptor struct {
	// true in case built by ab/
	Official bool
	// ab/buildID
	BuildID string
	// build version in case build is official e.g 9138603
	BuildVersion int
	// build type e.g. user, userdebug
	BuildType string
	// model type e.g. eve
	ModelType string
	// binary translation type e.g. houdini, ndk, native
	BinaryTranslationType string
	// Host ureadahead abi e.g. x86_64, arm, arm64
	HostUreadaheadAbi string
	// Guest cpu abi e.g. x86_64, x86, arm, arm64
	CPUAbi string
	// version release e.g. 9, 11
	VersionRelease int
	// ChromeOS milestone e.g 108
	Milestone int
}

func getBinaryTranslationType(ctx context.Context, dut *dut.DUT) (string, error) {
	b, err := LsCPURemote(ctx, dut)
	if err != nil {
		return "", errors.Wrap(err, "failed to check lscpu remotely")
	}
	lscpuResult := string(b)

	vendorID := regexp.MustCompile(`(\n|^)Vendor ID:(.+)(\n|$)`).FindStringSubmatch(lscpuResult)
	if vendorID == nil {
		return "", errors.Errorf("Vendor ID field is not found in %q", lscpuResult)
	}

	switch strings.TrimSpace(vendorID[2]) {
	case "GenuineIntel":
		return "houdini", nil
	case "AuthenticAMD":
		return "ndk", nil
	default:
		return "native", nil
	}
}

// GetBuildDescriptorRemotely gets ARC build properties from the device, parses for build ID, ABI,
// and returns these fields as a combined string. It also return whether this is official build.
func GetBuildDescriptorRemotely(ctx context.Context, dut *dut.DUT, vmEnabled bool) (*BuildDescriptor, error) {
	var propertyFile string
	if vmEnabled {
		propertyFile = "/usr/share/arcvm/properties/build.prop"
	} else {
		propertyFile = "/usr/share/arc/properties/build.prop"
	}

	buildProp, err := CatRemote(ctx, dut, propertyFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read ARC build property file remotely")
	}
	buildPropStr := string(buildProp)

	lsbRelease, err := CatRemote(ctx, dut, "/etc/lsb-release")
	if err != nil {
		return nil, errors.Wrap(err, "failed to lsb-release remotely")
	}
	lsbReleaseStr := string(lsbRelease)

	mCPUAbi := regexp.MustCompile(`(\n|^)ro.product.cpu.abi=(.+)(\n|$)`).FindStringSubmatch(buildPropStr)
	if mCPUAbi == nil {
		return nil, errors.Errorf("ro.product.cpu.abi is not found in %q", buildPropStr)
	}

	// ro.product.cpu.abilist32 has higher priority however some boards may not have it set.
	mCPUAbi32 := regexp.MustCompile(`(\n|^)ro.product.cpu.abilist32=(.*)(\n|$)`).FindStringSubmatch(buildPropStr)
	if mCPUAbi32 == nil {
		mCPUAbi32 = regexp.MustCompile(`(\n|^)ro.system.product.cpu.abilist32=(.*)(\n|$)`).FindStringSubmatch(buildPropStr)
		if mCPUAbi32 == nil {
			return nil, errors.Errorf("ro[.system].product.cpu.abilist32 is not found in %q", buildPropStr)
		}
	}

	mModelType := regexp.MustCompile(`(\n|^)ro.product(\.[a-z]+)?.model=(.+)(\n|$)`).FindStringSubmatch(buildPropStr)
	if mModelType == nil {
		return nil, errors.Errorf("ro.product*.model is not found in %q", buildPropStr)
	}

	mBuildType := regexp.MustCompile(`(\n|^)ro.build.type=(.+)(\n|$)`).FindStringSubmatch(buildPropStr)
	if mBuildType == nil {
		return nil, errors.Errorf("ro.product.cpu.abi is not found in %q", buildPropStr)
	}

	// Note, this should work on official builds only. Custom built Android image contains the
	// version in different format.
	mBuildID := regexp.MustCompile(`(\n|^)ro.build.version.incremental=(.+)(\n|$)`).FindStringSubmatch(buildPropStr)
	if mBuildID == nil {
		return nil, errors.Errorf("ro.build.version.incremental is not found in %q", buildPropStr)
	}

	mVersionRelease := regexp.MustCompile(`(\n|^)ro.build.version.release=(\d+)(\n|$)`).FindStringSubmatch(buildPropStr)
	if mVersionRelease == nil {
		return nil, errors.Errorf("ro.build.version.release is not found in %q", buildPropStr)
	}

	versionRelease, err := strconv.Atoi(mVersionRelease[2])
	if err != nil {
		return nil, errors.Errorf("could not parse ro.build.version.release=%s: %q", mVersionRelease[2], err)
	}

	mMilestone := regexp.MustCompile(`(\n|^)CHROMEOS_RELEASE_CHROME_MILESTONE=(\d+)(\n|$)`).FindStringSubmatch(lsbReleaseStr)
	if mMilestone == nil {
		return nil, errors.Errorf("CHROMEOS_RELEASE_CHROME_MILESTONE is not found in %q", lsbReleaseStr)
	}

	milestone, err := strconv.Atoi(mMilestone[2])
	if err != nil {
		return nil, errors.Errorf("could not parse CHROMEOS_RELEASE_CHROME_MILESTONE=%s: %q", mMilestone[2], err)
	}

	buildVersion := 0
	official := regexp.MustCompile(`^\d+$`).MatchString(mBuildID[2])
	if official {
		buildVersion, err = strconv.Atoi(mBuildID[2])
		if err != nil {
			return nil, errors.Errorf("could not parse ro.build.version.incremental=%s: %q", mBuildID[2], err)
		}
	}

	abiMap := map[string]string{
		"arm64-v8a": "arm64",
		"x86_64":    "x86_64",
	}

	abi, ok := abiMap[mCPUAbi[2]]
	if !ok {
		return nil, errors.Errorf("failed to map ABI %q", mCPUAbi[2])
	}

	if mCPUAbi32[2] == "" {
		if abi == "arm64" {
			abi = "arm64only"
		} else if abi == "x86_64" {
			abi = "x64only"
		} else {
			return nil, errors.Errorf("arc-64bit-only is not supported for %q", mCPUAbi[2])
		}
	}

	binaryTranslationType, err := getBinaryTranslationType(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get binary translation type")
	}

	desc := BuildDescriptor{
		Official:              official,
		BuildID:               mBuildID[2],
		BuildVersion:          buildVersion,
		BuildType:             mBuildType[2],
		ModelType:             mModelType[3],
		BinaryTranslationType: binaryTranslationType,
		CPUAbi:                abi,
		VersionRelease:        versionRelease,
		Milestone:             milestone,
	}

	return &desc, nil
}
