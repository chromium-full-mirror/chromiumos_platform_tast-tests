// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for local tests.
package graphics

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/graphics/hardwareprobe"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// The common path prefix for DEQP executables.
	deqpBaseDir = "/usr/local/deqp"

	// Path to the USE flags used by ChromiumCommandBuilder.
	uiUseFlagsPath = "/etc/ui_use_flags.txt"

	// Path to get/set the dirty_writeback_centisecs kernel parameter.
	dirtyWritebackCentisecsPath = "/proc/sys/vm/dirty_writeback_centisecs"
)

var (
	labels     = []string{"objects", "bytes"}
	amdRegex   = []string{"(?P<bytes>\\d*) byte  GTT CPU_ACCESS_REQUIRED CPU_GTT_USWC"}
	intelRegex = []string{
		"(?P<objects>\\d*) shrinkable.*objects, (?P<bytes>\\d*) bytes",
		// This is for kernels upto v4.19
		"(?P<objects>[0-9]+) objects, (?P<bytes>[0-9]+) bytes",
	}
)

// contains checks if an element of type string exists in a slice of strings.
func contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

// parseMemory matches the regex pattern groups with the sysfs file contents
func parseMemory(ctx context.Context, file []byte, memRegexp []string) (map[string]int, error) {
	results := make(map[string]int)
	var matches [][]string
	var memoryRe *regexp.Regexp
	for _, memRegex := range memRegexp {
		memoryRe = regexp.MustCompile(memRegex)
		matches = memoryRe.FindAllStringSubmatch(string(file), -1)
		if len(matches) > 0 {
			break
		}
	}
	if len(matches) == 0 {
		return nil, errors.New("failed to find matches in sysfs memory file")
	}
	groupNames := memoryRe.SubexpNames()
	// the 0th index has the whole string so skip it.
	for index := 1; index < len(matches[0]); index++ {
		value, err := strconv.Atoi(matches[0][index])
		if err != nil {
			return nil, errors.Wrapf(err, " unable to convert value %s to int ", matches[0][index])
		}
		results[groupNames[index]] = value
	}
	return results, nil
}

// GetValidKernelDriverDebugFile search the open nodes in /sys/kernel/debug/dri with a list of files, and returns the first path which exists.
func GetValidKernelDriverDebugFile(ctx context.Context, relPaths []string) (string, error) {
	resultErr := errors.Errorf("failed to find %v", relPaths)
	for _, relPath := range relPaths {
		p, err := getKernelDriverDebugFile(ctx, relPath)
		if err != nil {
			resultErr = errors.Wrap(err, resultErr.Error())
			continue
		}
		return p, nil
	}
	return "", resultErr
}

// getKernelDriverDebugFile search the open nodes in /sys/kernel/debug/dri and returns the a valid path.
func getKernelDriverDebugFile(ctx context.Context, relPath string) (string, error) {
	sysPath := "/sys/kernel/debug/dri/"
	paths, err := filepath.Glob(filepath.Join(sysPath, "*", relPath))
	if err != nil || paths == nil {
		return "", errors.Wrap(err, "failed to glob")
	}
	for _, path := range paths {
		// Skip if minor number is greater than 64.
		var minor uint64
		if _, err := fmt.Sscanf(path, sysPath+"%d", &minor); err != nil {
			testing.ContextLogf(ctx, "Failed to determine the minor number of %v, skipping", path)
			continue
		}
		if minor >= maxDRMDeviceNumber {
			testing.ContextLogf(ctx, "Minor number %d > %d, skipping reading %v in it", minor, maxDRMDeviceNumber, relPath)
			continue
		}

		name, err := os.ReadFile(fmt.Sprintf("%v/%v/name", sysPath, minor))
		if err != nil {
			return "", errors.Wrap(err, "failed to read driver name")
		}
		// Skipping virtual gem object.
		if strings.HasPrefix(string(name), "vgem") {
			continue
		}
		// Try read the content, sometimes the file exist but device is not.
		if _, err := os.ReadFile(path); err != nil {
			return "", errors.Wrap(err, "file exist but not readable")
		}
		testing.ContextLogf(ctx, "File %v under driver (%v) found", relPath, strings.TrimSpace(string(name)))
		return path, nil
	}
	return "", errors.Errorf("can't find any %v in kernel", relPath)
}

// processLabels find the label of interest in the sysfs file
func processLabels(ctx context.Context, file []byte) (map[string]int, error) {
	results := make(map[string]int)
	// When a label has been found, the previous word should be the value. e.g. "3200 bytes"
	var prevWord string
	for _, line := range strings.Split(string(file), "\n") {
		lineWords := strings.Split(strings.Replace(line, ",", "", -1), " ")
		for _, word := range lineWords {
			if _, exists := results[word]; exists {
				testing.ContextLogf(ctx, "%v is already recorded while parsing sysfs memory", word)
				continue
			}
			if contains(labels, word) && len(prevWord) > 0 {
				value, err := strconv.Atoi(prevWord)
				if err != nil {
					return nil, errors.Wrapf(err, " unable to convert value %s to int ", prevWord)
				}
				results[word] = value
			}
			prevWord = word
			if len(results) == len(labels) {
				return results, nil
			}
		}
	}
	return results, nil
}

// parseSysfsMemory parses output of graphics memory sysfs to determine the number of buffer objects and bytes.
func parseSysfsMemory(ctx context.Context, file string) (map[string]int, error) {
	output, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, " error encountered while reading file %s ", file)
	}
	soc, err := hardwareprobe.CPUFamily(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Machine CPU family is not supported")
	}
	if soc == "intel" {
		return parseMemory(ctx, output, intelRegex)
	} else if soc == "amd" {
		return parseMemory(ctx, output, amdRegex)
	} else {
		return processLabels(ctx, output)
	}
}

// GetSysfsMemory returns gpu memory usage
func GetSysfsMemory(ctx context.Context) (int, error) {
	var errMsg string
	file, err := GetValidKernelDriverDebugFile(ctx, []string{
		"i915_gem_objects",
	})
	if err != nil {
		return 0, err
	}
	parsedResults, err := parseSysfsMemory(ctx, file)
	if err != nil {
		return 0, err
	}
	bytes, exists := parsedResults["bytes"]
	if exists && bytes == 0 {
		errMsg = strings.Join([]string{errMsg, string(file), "reported 0 bytes"}, " ")
		return 0, errors.New(errMsg)
	}
	return bytes, nil

}

// APIType identifies a graphics API that can be tested by DEQP.
type APIType int

const (
	// UnknownAPI represents an unknown API.
	UnknownAPI APIType = iota
	// EGL represents Khronos EGL.
	EGL
	// GLES2 represents OpenGL ES 2.0.
	GLES2
	// GLES3 represents OpenGL ES 3.0.
	GLES3
	// GLES31 represents OpenGL ES 3.1. Note that DEQP also includes OpenGL ES
	// 3.2 in this category to reduce testing time.
	GLES31
	// VK represents Vulkan.
	VK
)

// Provided for getting readable API names.
func (a APIType) String() string {
	switch a {
	case EGL:
		return "EGL"
	case GLES2:
		return "GLES2"
	case GLES3:
		return "GLES3"
	case GLES31:
		return "GLES31"
	case VK:
		return "VK"
	}
	return fmt.Sprintf("UNKNOWN (%d)", a)
}

// parseUIUseFlags parses the configuration file located at path to get the UI
// USE flags: empty lines and lines starting with # are ignored. No end-of-line
// comments should be used. An empty non-nil map is returned if no flags are
// parsed. This is roughly a port of get_ui_use_flags() defined in
// autotest/files/client/bin/utils.py.
func parseUIUseFlags(path string) (map[string]struct{}, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	flags := make(map[string]struct{})
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if len(l) > 0 && l[0] != '#' {
			flags[l] = struct{}{}
		}
	}
	return flags, nil
}

// api returns a string identifying the graphics api, e.g. gl or gles2. This is
// roughly a port of graphics_api() defined in
// autotest/files/client/bin/utils.py.
func api(uiUseFlags map[string]struct{}) string {
	if _, ok := uiUseFlags["opengles"]; ok {
		return "gles2"
	}
	return "gl"
}

// extractOpenGLVersion takes the output of the wflinfo command and attempts to
// extract the OpenGL version. An example of the OpenGL version string expected
// in the wflinfo output is:
// OpenGL version string: OpenGL ES 3.2 Mesa 18.1.0-devel (git-131e871385)
func extractOpenGLVersion(ctx context.Context, wflout string) (major,
	minor int, err error) {
	re := regexp.MustCompile(
		`OpenGL version string: OpenGL ES ([0-9]+).([0-9]+)`)
	matches := re.FindAllStringSubmatch(wflout, -1)
	if len(matches) != 1 {
		if dir, ok := testing.ContextOutDir(ctx); ok {
			if err := ioutil.WriteFile(filepath.Join(dir, "wflinfo.txt"),
				[]byte(wflout), 0644); err != nil {
				testing.ContextLog(ctx, "Failed to write wflinfo output: ", err)
			}
		}
		return 0, 0, errors.Errorf("%d OpenGL version strings found in wflinfo output", len(matches))
	}
	testing.ContextLogf(ctx, "Got %q", matches[0][0])
	if major, err = strconv.Atoi(matches[0][1]); err != nil {
		return 0, 0, errors.Wrap(err, "could not parse major version")
	}
	if minor, err = strconv.Atoi(matches[0][2]); err != nil {
		return 0, 0, errors.Wrap(err, "could not parse minor version")
	}
	return major, minor, nil
}

// GLESVersion returns the OpenGL major and minor versions extracted from the
// output of the wflinfo command. This is roughly a port of get_gles_version()
// defined in autotest/files/client/cros/graphics/graphics_utils.py.
func GLESVersion(ctx context.Context) (major, minor int, err error) {
	f, err := parseUIUseFlags(uiUseFlagsPath)
	if err != nil {
		return 0, 0, errors.Wrap(err, "could not get UI USE flags")
	}
	cmd := testexec.CommandContext(ctx, "wflinfo", "-p", "null", "-a", api(f))
	out, err := cmd.Output()
	if err != nil {
		cmd.DumpLog(ctx)
		return 0, 0, errors.Wrap(err, "running the wflinfo command failed")
	}
	return extractOpenGLVersion(ctx, string(out))
}

// SupportsVulkanForDEQP decides whether the board supports Vulkan for DEQP
// testing. An error is returned if something unexpected happens while deciding.
// This is a port of part of the functionality of GraphicsApiHelper defined in
// autotest/files/client/cros/graphics/graphics_utils.py.
func SupportsVulkanForDEQP(ctx context.Context) (bool, error) {
	// First, search for libvulkan.so.
	hasVulkan := false
	for _, dir := range []string{"/usr/lib", "/usr/lib64", "/usr/local/lib", "/usr/local/lib64"} {
		if _, err := os.Stat(filepath.Join(dir, "libvulkan.so")); err == nil {
			hasVulkan = true
			break
		} else if !os.IsNotExist(err) {
			return false, errors.Wrap(err, "libvulkan.so search error")
		}
	}
	if !hasVulkan {
		testing.ContextLog(ctx, "Could not find libvulkan.so")
		return false, nil
	}

	// Then, search for the DEQP Vulkan testing binary.
	p, err := DEQPExecutable(VK)
	if err != nil {
		return false, errors.Wrapf(err, "could not get the executable for %v", VK)
	}
	if _, err := os.Stat(p); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, errors.Wrapf(err, "%v search error", p)
	}

	testing.ContextLogf(ctx, "Found libvulkan.so but not the %v binary", p)
	return false, nil
}

// SupportedAPIs returns an array of supported APIs given the OpenGL version and
// whether Vulkan is supported. If no APIs are supported, nil is returned. This
// is a port of part of the functionality of GraphicsApiHelper defined in
// autotest/files/client/cros/graphics/graphics_utils.py.
func SupportedAPIs(glMajor, glMinor int, vulkan bool) []APIType {
	var apis []APIType
	if glMajor >= 2 {
		apis = append(apis, GLES2)
	}
	if glMajor >= 3 {
		apis = append(apis, GLES3)
		if glMajor > 3 || glMinor >= 1 {
			apis = append(apis, GLES31)
		}
	}
	if vulkan {
		apis = append(apis, VK)
	}
	return apis
}

// DEQPExecutable maps an API identifier to the path of the appropriate DEQP
// executable (or an empty string if the API identifier is not valid). This is a
// port of part of the functionality of GraphicsApiHelper defined in
// autotest/files/client/cros/graphics/graphics_utils.py.
func DEQPExecutable(api APIType) (string, error) {
	switch api {
	case EGL:
		return "", errors.New("cannot run DEQP/EGL on ChromeOS")
	case GLES2:
		return filepath.Join(deqpBaseDir, "modules/gles2/deqp-gles2"), nil
	case GLES3:
		return filepath.Join(deqpBaseDir, "modules/gles3/deqp-gles3"), nil
	case GLES31:
		return filepath.Join(deqpBaseDir, "modules/gles31/deqp-gles31"), nil
	case VK:
		return filepath.Join(deqpBaseDir, "external/vulkancts/modules/vulkan/deqp-vk"), nil
	}
	return "", errors.Errorf("unknown graphics API: %s", api)
}

// DEQPEnvironment returns a list of environment variables of the form
// "key=value" that are appropriate for running DEQP binaries. To build it, the
// function starts from the given environment and modifies the LD_LIBRARY_PATH
// to insert /usr/local/lib:/usr/local/lib64 in the front, even if those two
// folders are already in the value. This is a port of part of the functionality
// of the initialization defined in
// autotest/files/client/site_tests/graphics_dEQP/graphics_dEQP.py.
func DEQPEnvironment(env []string) []string {
	// Start from a copy of the passed environment.
	nenv := make([]string, len(env))
	copy(nenv, env)

	// Search for the LD_LIBRARY_PATH variable in the environment.
	oldld := ""
	ldi := -1
	for i, s := range nenv {
		// Each s is of the form key=value.
		kv := strings.Split(s, "=")
		if kv[0] == "LD_LIBRARY_PATH" {
			ldi = i
			oldld = kv[1]
			break
		}
	}

	const paths = "/usr/local/lib:/usr/local/lib64"
	if ldi != -1 {
		// Found the LD_LIBRARY_PATH variable in the environment.
		if len(oldld) > 0 {
			nenv[ldi] = fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", paths, oldld)
		} else {
			nenv[ldi] = "LD_LIBRARY_PATH=" + paths
		}
	} else {
		// Did not find the LD_LIBRARY_PATH variable in the environment.
		nenv = append(nenv, "LD_LIBRARY_PATH="+paths)
	}

	return nenv
}

// SetDirtyWritebackDuration flushes pending data to disk and sets the
// dirty_writeback_centisecs kernel parameter to a specified time. If the time
// is negative, it only flushes pending data without changing the kernel
// parameter. This function should be used before starting graphics tests to
// shorten the time between flushes so that logs retain as much information as
// possible in case the system hangs/reboots. Note that the specified time is
// rounded down to the nearest centisecond. This is a port of the
// set_dirty_writeback_centisecs() function in
// autotest/files/client/bin/utils.py.
func SetDirtyWritebackDuration(ctx context.Context, d time.Duration) error {
	// Performing a full sync makes it less likely that there are pending writes
	// that might defer logging from being written immediately later.
	cmd := testexec.CommandContext(ctx, "sync")
	err := cmd.Run()
	if err != nil {
		cmd.DumpLog(ctx)
		return errors.Wrap(err, "sync failed")
	}
	if d >= 0 {
		centisecs := d / (time.Second / 100)
		if err = ioutil.WriteFile(dirtyWritebackCentisecsPath, []byte(fmt.Sprintf("%d", centisecs)), 0600); err != nil {
			return err
		}

		// Read back the kernel parameter because it is possible that the
		// written value was silently discarded (e.g. if the value was too
		// large).
		actual, err := GetDirtyWritebackDuration()
		if err != nil {
			return errors.Wrapf(err, "could not read %v", filepath.Base(dirtyWritebackCentisecsPath))
		}
		expected := centisecs * (time.Second / 100)
		if actual != expected {
			return errors.Errorf("%v contains %d after writing %d", filepath.Base(dirtyWritebackCentisecsPath), actual, expected)
		}
	}
	return nil
}

// GetDirtyWritebackDuration reads the dirty_writeback_centisecs kernel
// parameter and returns it as a time.Duration (in nanoseconds). Note that it is
// possible for the returned value to be negative. This is a port of the
// get_dirty_writeback_centisecs() function in
// autotest/files/client/bin/utils.py.
func GetDirtyWritebackDuration() (time.Duration, error) {
	b, err := ioutil.ReadFile(dirtyWritebackCentisecsPath)
	if err != nil {
		return -1, err
	}
	s := strings.TrimSpace(string(b))
	if len(s) == 0 {
		return -1, errors.Errorf("%v is empty", filepath.Base(dirtyWritebackCentisecsPath))
	}
	centisecs, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return -1, errors.Wrapf(err, "could not parse %v", filepath.Base(dirtyWritebackCentisecsPath))
	}
	return time.Duration(centisecs) * (time.Second / 100), nil
}

// DumpGraphicsDebugFiles saves graphics related debug files to outDir.
func DumpGraphicsDebugFiles(ctx context.Context, outDir string) error {
	// Save files under /sys/kernel/debug/dri
	for _, opt := range []struct {
		name     string
		optional bool
	}{{
		name:     "i915_error_state",
		optional: true,
	}} {
		file, err := GetValidKernelDriverDebugFile(ctx, []string{opt.name})
		if err != nil {
			testing.ContextLogf(ctx, "Failed to find %v: %v", opt.name, err)
			if !opt.optional {
				return errors.Wrapf(err, "failed to find %v", opt.name)
			}
			continue
		}
		destName := filepath.Join(outDir, opt.name)
		testing.ContextLogf(ctx, "from %v to %v", file, destName)
		if err = fsutil.CopyFile(file, destName); err != nil {
			return err
		}
	}
	return nil
}

const (
	waitTime      = 5 * time.Second
	vt2Path       = "/run/frecon/vt1"
	escapeCodeVT1 = "\\033]switchvt:0\\a"
	escapeCodeVT2 = "\\033]switchvt:1\\a"
	// FreconCurrentPath is the symlink path to the currently active terminal
	FreconCurrentPath = "/run/frecon/current"
)

// OpenVT1 switches to the VT1 terminal
func OpenVT1(ctx context.Context) error {
	// If link does not exist we do not need to switch
	if _, err := os.Stat(FreconCurrentPath); err != nil {
		return nil
	}
	/* Frecon needs quotes around the command see the following:
	 * https://source.corp.google.com/chromeos_public/src/platform/frecon/README.md
	 */
	testing.ContextLog(ctx, "Switching to VT1")
	cmd := "printf \"" + escapeCodeVT1 + "\" > \"" + FreconCurrentPath + "\""
	err := testexec.CommandContext(ctx, "bash", "-c", cmd).Run(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to switch to VT1 through frecon escape code")
	}
	// Delete link
	os.Remove(FreconCurrentPath)
	// GoBigSleepLint: Allowing some wait time for switching to happen.
	// TODO(b:198837833): Replace with testing.Poll to query the current vts node.
	if err = testing.Sleep(ctx, waitTime); err != nil {
		return errors.Wrap(err, "error while waiting for switching to VT1")
	}
	return nil
}

// OpenVT2 switches to the VT2 terminal
func OpenVT2(ctx context.Context) error {
	/* Frecon needs quotes around the command see the following:
	 * https://source.corp.google.com/chromeos_public/src/platform/frecon/README.md
	 */
	cmd := "printf \"" + escapeCodeVT2 + "\" > \"" + FreconCurrentPath + "\""
	err := testexec.CommandContext(ctx, "ln", "-s", vt2Path, FreconCurrentPath).Run(testexec.DumpLogOnError)
	testing.ContextLog(ctx, "Switching to VT2")
	if err != nil {
		return errors.Wrap(err, "failed to link VT2 through frecon")
	}
	err = testexec.CommandContext(ctx, "bash", "-c", cmd).Run(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to switch to VT2 through frecon escape code")
	}
	// GoBigSleepLint: Allowing some wait time for switching to happen.
	// TODO(b:198837833): Replace with testing.Poll to query the current vts node.
	if err = testing.Sleep(ctx, waitTime); err != nil {
		return errors.Wrap(err, "error while waiting for switching to VT2")
	}
	return nil
}
