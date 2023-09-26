// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/graphics/expectations"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Idle,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Log into Chrome, request nothing to be on the desktop, wait, verify there are moments where the GPU has nothing to do. If the GPU stays continuously busy system power usage will be unacceptably high",
		// TODO(pwang): Add to CQ once it is green and stable.
		Attr:         []string{"group:graphics", "graphics_nightly", "group:mainline"},
		BugComponent: "b:995569", // ChromeOS > Platform > Graphics > GPU
		Contacts: []string{
			"chromeos-gfx@google.com",
			"pwang@chromium.org",
		},
		Timeout:      chrome.LoginTimeout + 3*time.Minute,
		SoftwareDeps: []string{"chrome", "no_qemu"},
		// This test needs to restart Chrome and log in each time it runs to get a clean instance. It also needs to close open browser windows to hide the Google doodle and other possible active graphic content.
		Params: []testing.Param{{
			Name:              "dvfs",
			Val:               dvfs,
			ExtraHardwareDeps: hwdep.D(hwdep.CPUSocFamily("rockchip", "mediatek")),
			Fixture:           "chromeGraphicsIdle",
		}, {
			Name:              "dvfs_arc",
			Val:               dvfs,
			ExtraHardwareDeps: hwdep.D(hwdep.CPUSocFamily("rockchip", "mediatek")),
			Fixture:           "chromeGraphicsIdleArc",
		}, {
			// TODO(pwang): Not all platform has fbc enabled. Add SoftwareDeps/HardwareDeps once we got some results on stainless.
			Name:              "fbc",
			Val:               fbc,
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.CPUSocFamily("intel")),
			ExtraAttr:         []string{"informational"},
			Fixture:           "chromeGraphicsIdle",
		}, {
			Name:              "fbc_arc",
			Val:               fbc,
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.CPUSocFamily("intel")),
			ExtraAttr:         []string{"informational"},
			Fixture:           "chromeGraphicsIdleArc",
		}, {
			Name:              "psr",
			Val:               psr,
			ExtraHardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
			Fixture:           "chromeGraphicsIdle",
		}, {
			Name:              "psr_arc",
			Val:               psr,
			ExtraHardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
			Fixture:           "chromeGraphicsIdleArc",
		}, {
			Name:              "i915_min_clock",
			Val:               i915MinClock,
			ExtraHardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
			Fixture:           "chromeGraphicsIdle",
		}, {
			Name:              "i915_min_clock_arc",
			Val:               i915MinClock,
			ExtraHardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
			Fixture:           "chromeGraphicsIdleArc",
		}, {
			Name:              "rc6",
			Val:               rc6,
			ExtraHardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
			Fixture:           "chromeGraphicsIdle",
		}, {
			Name:              "rc6_arc",
			Val:               rc6,
			ExtraHardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
			Fixture:           "chromeGraphicsIdleArc",
		}},
	})
}

func Idle(ctx context.Context, s *testing.State) {
	e, err := expectations.GetTestExpectation(ctx, s.TestName())
	if err != nil {
		s.Fatal("Unable to get test expectation: ", err)
	}
	defer func() {
		if err := e.HandleFinalExpectation(); err != nil {
			s.Error("Unmet expectation: ", err)
		}
	}()

	check := s.Param().(func(context.Context) error)
	if err := check(ctx); err != nil {
		if expErr := e.ReportError("Failed with: ", err); expErr != nil {
			s.Error("Failed with: ", expErr)
		}
	}
}

// getValidDir search the list of paths and return the directory which exists.
func getValidDir(paths []string) (string, error) {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			continue
		}
		return path, nil
	}
	return "", errors.Errorf("none of %v exist", paths)
}

// getValidPath search the list of paths and return the path which exists.
func getValidPath(paths []string) (string, error) {
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		return path, nil
	}
	return "", errors.Errorf("none of %v exist", paths)
}

// dvfs checks that we get into the lowest clock frequency.
func dvfs(ctx context.Context) error {
	node, err := getValidDir([]string{
		// Exynos
		"/sys/devices/11800000.mali/",
		// RK3288
		"/sys/devices/ffa30000.gpu/",
		// RK3288_419
		"/sys/devices/platform/ffa30000.gpu/",
		// RK3399
		"/sys/devices/platform/ff9a0000.gpu/",
		// MT8173
		"/sys/devices/soc/13000000.mfgsys-gpu/",
		// MT8173_419
		"/sys/devices/platform/soc/13000000.mfgsys-gpu/",
		// MT8183 (v6.1+ / upstream DT)
		"/sys/devices/platform/soc/13040000.gpu/",
		// MT8183 (legacy downstream DT)
		"/sys/devices/platform/soc/13040000.mali/",
		// MT8192 (v6.1+ / upstream DT)
		"/sys/devices/platform/soc/13000000.gpu/",
		// MT8192 (legacy downstream DT)
		"/sys/devices/platform/soc/13000000.mali/",
	})
	if err != nil {
		return errors.Wrap(err, "unknown soc for dvfs")
	}
	matches, err := filepath.Glob(filepath.Join(node, "devfreq", "*"))
	if err != nil {
		return errors.Wrapf(err, "failed to glob devfreq device under %v", node)
	}
	if len(matches) != 1 {
		return errors.Wrapf(err, "expect 1 devfreq device, got %v", matches)
	}
	devFreqPath := matches[0]
	governorPath := filepath.Join(devFreqPath, "governor")
	clockPath := filepath.Join(devFreqPath, "cur_freq")

	out, err := os.ReadFile(governorPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read dvfs governor path %v", governorPath)
	}
	governors := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(governors) != 1 {
		return errors.Wrapf(err, "more than 1 governor found: %v", governors)
	}
	governor := governors[0]
	testing.ContextLogf(ctx, "DVFS governor = %s", governor)
	if governor != "simple_ondemand" {
		return errors.Errorf("expect simple_ondemand dvfs governor, got %v", governor)
	}

	frequenciesPath := filepath.Join(devFreqPath, "available_frequencies")
	// available_frequencies are always sorted in ascending order.
	// each line may contain one or multiple integers separated by spaces.
	out, err = os.ReadFile(frequenciesPath)
	if err != nil {
		return errors.Wrapf(err, "failed to open %v", frequenciesPath)
	}
	frequencies := strings.Split(strings.TrimSpace(string(out)), " ")
	minFreq, err := strconv.ParseInt(frequencies[0], 0, 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse frequency %v", frequencies[0])
	}
	testing.ContextLog(ctx, "Expecting idle DVFS clock: ", minFreq)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		f, err := os.ReadFile(clockPath)
		if err != nil {
			return errors.Wrapf(err, "failed to open %v", clockPath)
		}
		clockStr := strings.Split(string(f), "\n")[0]
		clock, err := strconv.ParseInt(clockStr, 0, 64)
		if err != nil {
			return errors.Wrapf(err, "failed to parse clock %v", clockStr)
		}
		if clock > minFreq {
			return errors.Errorf("clock frequency %v is higher than idle DVFS clock %v", clock, minFreq)
		}
		testing.ContextLog(ctx, "Found idle DVFS clock: ", clock)
		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute,
	}); err != nil {
		return err
	}
	return nil
}

// fbc checks that we can get into FBC.
func fbc(ctx context.Context) error {
	fbcPath, err := graphics.GetValidKernelDriverDebugFile(ctx, []string{
		"i915_fbc_status",
	})
	if err != nil {
		return errors.Wrap(err, "no FBC_PATHS found")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		f, err := os.ReadFile(fbcPath)
		if err != nil {
			return errors.Wrapf(err, "failed to open %v", fbcPath)
		}
		re := regexp.MustCompile("FBC enabled")
		matches := re.FindStringSubmatch(string(f))
		if matches == nil {
			return errors.Wrapf(err, "FBC enabled not found, last content: %v", string(f))
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute,
	}); err != nil {
		return err
	}
	return nil
}

// psr checks that we can get into PSR if device support it.
func psr(ctx context.Context) error {
	psrPath, err := graphics.GetValidKernelDriverDebugFile(ctx, []string{
		"i915_edp_psr_status",
	})
	if err != nil {
		// PSR is display specific feature, simply let it pass.
		testing.ContextLog(ctx, "Failed to find valid psr path: ", err)
		return nil
	}
	kernelVersion, _, err := sysutil.KernelVersionAndArch()
	if err != nil {
		return errors.Wrap(err, "failed to get kernel version")
	}
	testing.ContextLogf(ctx, "Kernel version: %s", kernelVersion)

	// checks if PSR is enabled on the device.
	f, err := os.ReadFile(psrPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %v", psrPath)
	}
	re := regexp.MustCompile("Enabled: yes")
	matches := re.FindStringSubmatch(string(f))
	if matches == nil {
		testing.ContextLog(ctx, "PSR not enabled")
		return nil
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		f, err := os.ReadFile(psrPath)
		if err != nil {
			return errors.Wrapf(err, "failed to open %v", psrPath)
		}
		var re *regexp.Regexp
		if kernelVersion.IsOrLater(4, 4) {
			re = regexp.MustCompile("PSR status: .* \\[SRDENT")
		} else if kernelVersion.Is(3, 18) {
			re = regexp.MustCompile("Performance_Counter: 0")
		}
		if re.FindStringSubmatch(string(f)) == nil {
			return errors.Errorf("didn't not see psr activity in %s", psrPath)
		}
		testing.ContextLogf(ctx, "Found active with kernel: %s", kernelVersion)
		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute,
	}); err != nil {
		return err
	}
	return nil
}

// i915MinClock checks that we get into the lowest clock frequency.
func i915MinClock(ctx context.Context) error {
	clockPath, err := graphics.GetValidKernelDriverDebugFile(ctx, []string{
		"i915_frequency_info",
		// TODO(marcheu): remove if this is not available/used anymore.
		"i915_cur_delayinfo",
	})
	if err != nil {
		return errors.Wrap(err, "failed to get clock path")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		f, err := os.ReadFile(clockPath)
		if err != nil {
			return errors.Wrapf(err, "failed to open %v", clockPath)
		}

		content := string(f)

		// This file has a different format depending on the kernel/board, so we loop them.
		// Also, it would be tedious to add the minimum clock for each board, so instead
		// we use 650MHz which is the max of the minimum clocks.
		for _, re := range []*regexp.Regexp{
			regexp.MustCompile(`CAGF: (.*)MHz`),
			regexp.MustCompile(`current GPU freq: (.*) MHz`),
			regexp.MustCompile(`Actual freq: (\d*) MHz`), // slpc freq dumps
		} {
			matches := re.FindStringSubmatch(content)
			if matches == nil {
				continue
			}
			hz, err := strconv.ParseInt(matches[1], 0, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse %s to int", matches[1])
			}
			// Print the line for debugging.
			testing.ContextLog(ctx, "Successfully parsed: ", matches[0])
			if hz <= 650 {
				return nil
			}
		}
		return errors.New("did not see the min i915 clock")
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute,
	}); err != nil {
		return err
	}
	return nil
}

// rc6 checks that we are able to get into rc6.
func rc6(ctx context.Context) error {
	rc6Path, err := graphics.GetValidKernelDriverDebugFile(ctx, []string{
		"i915_drpc_info",
		"gt*/drpc",
	})
	if err != nil {
		return errors.Wrap(err, "failed to get valid rc6 path")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		f, err := os.ReadFile(rc6Path)
		if err != nil {
			return errors.Wrapf(err, "failed to open %v", rc6Path)
		}
		re := regexp.MustCompile("Current RC state: (.*)")
		matches := re.FindStringSubmatch(string(f))
		if matches != nil && matches[1] == "RC6" {
			return nil
		}
		return errors.New("did not see the GPU in RC6")
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute,
	}); err != nil {
		return err
	}
	return nil
}
