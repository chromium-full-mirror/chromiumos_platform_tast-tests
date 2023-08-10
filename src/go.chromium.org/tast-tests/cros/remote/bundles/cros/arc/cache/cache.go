// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cache provides set of util functions used to work with ARC caches.
package cache

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/arc/dututils"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Base path for Chrome build Android artifacts.
	buildsRoot = "gs://chromeos-arc-images/builds"

	// Name of jar cache library file.
	cacheBuilderJarName = "org.chromium.arc.cachebuilder.jar"
)

// regExpEndsWithBuildID is the regexp to find the build ID from the path entry where build ID
// is the laset segment in path.
var regExpEndsWithBuildID = regexp.MustCompile(`^.+/(\d+)/$`)

// findRecentCacheBuilderJar scans the list of available entries with ARC cache builders jar files
// and returns one which has the highest build ID that indicates the most recent entry.
func findRecentCacheBuilderJar(ctx context.Context, versionRelease int) (string, error) {
	testing.ContextLogf(ctx, "Build is not official, finding the latest %q", cacheBuilderJarName)

	branch := ""
	switch versionRelease {
	case 9:
		branch = "pi-arc"
	case 11:
		branch = "rvc-arc"
	case 13:
		branch = "tm-arc"
	default:
		return "", errors.Errorf("unrecognized ARC release %d", versionRelease)
	}

	root := fmt.Sprintf("%s/git_%s-linux-apps/", buildsRoot, branch)
	out, err := testexec.CommandContext(ctx, "gsutil", "ls", root).Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to list apps")
	}

	result := ""
	resultBuildID := 0

	for _, candidate := range strings.Split(string(out), "\n") {
		m := regExpEndsWithBuildID.FindStringSubmatch(candidate)
		// Not finding match is normal once this is external folder and may contain non-matching entries
		if m == nil {
			continue
		}
		candidateBuildID, err := strconv.Atoi(m[1])
		if err != nil {
			return "", errors.Wrapf(err, "failed to parse buildID from %s", candidate)
		}
		if candidateBuildID > resultBuildID {
			result = candidate
			resultBuildID = candidateBuildID
		}
	}

	if result == "" {
		return "", errors.Errorf("failed to find %q at %q", cacheBuilderJarName, root)
	}

	result = result + cacheBuilderJarName
	testing.ContextLogf(ctx, "Resolved recent cache builder jar as %q", result)
	return result, nil
}

// getCacheBuilderJar gets ARC build properties from the device, parses for build ID, and
// generates gs URL for org.chromium.ard.cachebuilder.jar
func getCacheBuilderJar(ctx context.Context, dut *dut.DUT, vmEnabled bool) (string, error) {
	desc, err := dututils.GetBuildDescriptorRemotely(ctx, dut, vmEnabled)
	if err != nil {
		return "", errors.Wrap(err, "failed to get ARC build desc")
	}

	if desc.Official {
		return fmt.Sprintf("%s/%s/%s/%s", buildsRoot, "git_*-linux-apps", desc.BuildID, cacheBuilderJarName), nil
	}

	return findRecentCacheBuilderJar(ctx, desc.VersionRelease)
}

// InstallCacheBuilderJar downloads corresponding version of cache builder library jar file from
// the cloud and install it into provided directory.
func InstallCacheBuilderJar(ctx context.Context, dut *dut.DUT, vmEnabled bool, dir string) (string, error) {
	url, err := getCacheBuilderJar(ctx, dut, vmEnabled)
	if err != nil {
		return "", err
	}

	jarPath := filepath.Join(dir, filepath.Base(url))
	testing.ContextLogf(ctx, "Installing cache builder jar from %q to %q", url, jarPath)

	if err := testexec.CommandContext(ctx, "gsutil", "copy", url, jarPath).Run(testexec.DumpLogOnError); err != nil {
		return "", errors.Wrapf(err, "failed to download from %s", url)
	}

	return jarPath, nil
}

// ValidatePackagesCache validates sourcePath packages xml against the referencePath packages
// xml where dynamicValidate will determine whether to verify all permissions elements and
// fingerprints are also required (e.g. certificates, tag metadata, non user ID attributes,
// signatures, signing keyset, etc.)
func ValidatePackagesCache(ctx context.Context, jarPath, sourcePath, referencePath string, validateAll bool) error {
	const javaClass = "org.chromium.arc.cachebuilder.Validator"
	if jarPath == "" || sourcePath == "" || referencePath == "" {
		return errors.New("failed to run cache validator with invalid empty path(s)")
	}
	var dynamicValidateStr string
	if validateAll {
		dynamicValidateStr = "yes"
	} else {
		dynamicValidateStr = "no"
	}

	if err := testexec.CommandContext(
		ctx, "java", "-cp", jarPath, javaClass,
		"--source", sourcePath, "--reference", referencePath,
		"--dynamic-validate", dynamicValidateStr).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}

// InstallGmsCoreCaches installs GMS Core caches based on files from manifestPath and tarPath where
// enforceMatchingTimestamp will determine whether to require manifest entry timestamp and timestamp
// of actual system image apk files to match from the system-root rootDir path.
func InstallGmsCoreCaches(ctx context.Context, jarPath, rootDir, tarPath, manifestPath, outDir string, enforceMatchingTimestamp, preservePermissionsOwnership bool) error {
	const (
		javaClass  = "org.chromium.arc.cachebuilder.GmsCoreCacheInstaller"
		appChimera = "app_chimera"
	)

	if tarPath == manifestPath {
		return errors.New("failed to run cache installer due to invalid identical tar and manifest paths")
	}
	if jarPath == "" || tarPath == "" || manifestPath == "" {
		return errors.New("failed to run cache installer with invalid empty path(s)")
	}
	if !strings.Contains(rootDir, "/") || !strings.Contains(outDir, "/") {
		return errors.New("failed to run cache installer, please specify valid directories")
	}
	var enforceMatchingTimestampStr string
	if enforceMatchingTimestamp {
		enforceMatchingTimestampStr = "yes"
	} else {
		enforceMatchingTimestampStr = "no"
	}
	var preservePermissionsOwnershipStr string
	if preservePermissionsOwnership {
		preservePermissionsOwnershipStr = "yes"
	} else {
		preservePermissionsOwnershipStr = "no"
	}

	if err := testexec.CommandContext(
		ctx, "sudo", "java", "-cp", jarPath, javaClass,
		"--system-root", rootDir, "--gms-caches", tarPath,
		"--enforce-matching-timestamp", enforceMatchingTimestampStr,
		"--preserve-permissions-ownership", preservePermissionsOwnershipStr,
		"--manifest", manifestPath, "--output-dir", outDir).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	appChimeraPath := filepath.Join(outDir, appChimera)
	testing.ContextLogf(ctx, "Updating permissions for %q", appChimeraPath)
	if err := testexec.CommandContext(ctx, "sudo", "chmod", "-R", "0755", appChimeraPath).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}

// GeneratePackagesCache generates packages cache based on files in the vendor image where useReference means
// packages reference will be used for dynamic generation.
func GeneratePackagesCache(ctx context.Context, jarPath, rootDir, androidPath, vendorTmpDir, packagesCache, fileHashCache, packagesReference string, useReference bool) error {
	const javaClass = "org.chromium.arc.cachebuilder.CacheGenerator"
	if androidPath == "" || vendorTmpDir == "" || packagesCache == "" || fileHashCache == "" {
		return errors.New("failed to run cache generator with invalid empty path(s)")
	}
	args := []string{
		"java", "-cp", jarPath, javaClass,
		"--system-root", rootDir,
		"--vendor-root", androidPath,
		"--vendor-root", vendorTmpDir,
		"--output", packagesCache, "--output-apk-hashes", fileHashCache,
	}
	if useReference {
		if packagesReference == "" {
			return errors.New("failed to run cache generator with invalid empty reference path")
		}
		args = append(args, "--reference", packagesReference, "--generate", "yes")
	}
	if err := testexec.CommandContext(ctx, "sudo", args...).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}
