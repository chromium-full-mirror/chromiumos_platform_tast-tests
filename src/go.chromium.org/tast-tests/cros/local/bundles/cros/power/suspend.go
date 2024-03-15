// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"bufio"
	"context"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"

	"golang.org/x/sys/unix"
)

var (
	// These models are affected by an issue where reads done via
	// /dev/drm_dp_aux1 cause fwupd to not be freezeable when the kernel is
	// suspending the device. b/319036849
	nofwupdFilteredModels = []string{"tentacool", "tentacruel", "elm", "hana", "homestar", "quackingstick", "wormdingler", "kingoftown", "lazor", "limozeen", "pazquel", "pompom"}

	// These nami models are filtered out because Nami/sona seems to be unstable
	// when suspending. b/324533891
	namiFilteredModels = []string{"sona"}

	// These octopus models are filtered out because they have a touchpad issue
	// on kernel-upstream (b/329161200)
	octopusFilteredModels = []string{"foob", "foob360"}

	allFilteredModels = append(append(nofwupdFilteredModels, namiFilteredModels...), octopusFilteredModels...)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Suspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Simple, single-cycle Suspend and Resume",
		Contacts: []string{
			"chromeos-platform-power@google.com",
		},
		BugComponent: "b:1361410",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Timeout:      4 * time.Minute,
		// TODO(b/319036849): when the issues with these devices are resolved, remove parameterised
		// versions of this test and also remove the hwdeps - this should be run on all devices
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(allFilteredModels...)),
				Val:               "fwupd_nochange",
			}, {
				Name:              "unstable",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(allFilteredModels...)),
				Val:               "fwupd_nochange",
			}, {
				Name:              "nofwupd",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(nofwupdFilteredModels...)),
				Val:               "fwupd_off",
			},
		},
	})
}

func startEvtestLog(ctx context.Context, path string) (func() (string, error), error) {
	cmd := testexec.CommandContext(ctx, "evtest", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create evtest stdout pipe for "+path)
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start evtest "+path)
	}
	testing.ContextLogf(ctx, "started evtest %s", path)

	resultChannel := make(chan string)
	go func() {
		scanner := bufio.NewScanner(stdout)
		var lines []string
		testing.ContextLogf(ctx, "started soaking output of evtest %s", path)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		testing.ContextLogf(ctx, "end of output for evtest %s", path)
		resultChannel <- strings.Join(lines, "\n")
	}()

	stopCallback := func() (string, error) {
		if err := cmd.Kill(); err != nil {
			testing.ContextLogf(ctx, "Error killing evtest %s: %v", path, err)
		}
		testing.ContextLogf(ctx, "killed evtest %s", path)
		if err := cmd.Wait(); err != nil {
			status := cmd.ProcessState.Sys().(syscall.WaitStatus)
			signaled := status.Signaled()
			signal := status.Signal()
			// Expect it to be killed.
			if !signaled || signal != unix.SIGKILL {
				return "", errors.Wrap(err, "evtest "+path+" not finished by expected SIGKILL")
			}
		}

		select {
		case output := <-resultChannel:
			testing.ContextLogf(ctx, "received output for evtest %s", path)
			return output, nil
		case <-ctx.Done():
			return "", errors.Wrap(ctx.Err(), "failed waiting for evtest "+path)
		}
	}

	return stopCallback, nil
}

func startEvTestLogging(ctx context.Context, s *testing.State) (func(), error) {
	infos, err := input.ReadInputDevices("")
	if err != nil {
		return nil, errors.Wrap(err, "couldn't ReadInputDevices")
	}

	stopCallbacks := make(map[string]func() (string, error))

	for _, info := range infos {
		cb, err := startEvtestLog(ctx, info.Path)
		if err != nil {
			testing.ContextLog(ctx, "Couldn't startEvTestLog: ", err)
			continue
		}
		stopCallbacks[info.Name+" @ "+info.Path] = cb
	}

	stopCallback := func() {
		var i int = 0
		for name, stopCb := range stopCallbacks {
			output, err := stopCb()
			if err != nil {
				testing.ContextLogf(ctx, "Error with %s stop callback: %v", name, err)
				continue
			}
			filename := "evtest" + strconv.Itoa(i) + ".txt"
			i = i + 1
			testing.ContextLogf(ctx, "Writing evtest output for %s to %s", name, filename)
			if err := ioutil.WriteFile(filepath.Join(s.OutDir(), filename), []byte(output), 0644); err != nil {
				testing.ContextLogf(ctx, "Failed to write %s: %v", filename, err)
			}
		}
	}

	return stopCallback, nil
}

// Suspend suspends the DUT and wakes again. If the suspend fails, an
// error is returned. If the resume fails, the DUT may stay suspended
// indefinitely, causing the test infrastucture to mark the test as failed.
// TODO: add different kinds of suspend test, including stress test
func Suspend(ctx context.Context, s *testing.State) {
	// TODO(b/324513129): remove this once we have a better solution in place.
	// **DO NOT COPY-PASTE THIS CODE IF YOU ARE NOT AFFECTED BY b/324513129**
	// If you are affected by b/324513129, please add a comment on that bug
	// and retain this comment block in the new location.
	_, err := setup.EnableService(ctx, "powerd")
	if err != nil {
		s.Fatal("Did not start powerd: ", err)
	}
	testing.ContextLog(ctx, "waiting for powerd to become ready")
	if _, err := power.NewPowerManager(ctx); err != nil {
		s.Fatal("Failed to connect to PowerManager DBus interface after restarting powerd: ", err)
	}

	if s.Param().(string) == "fwupd_off" {
		// Sometimes fwupd may end up being stuck in a lengthy transfer from a device making it non-
		// suspendable. Stop fwupd before trying the suspend so this doesn't happen.
		startFwupdFn, err := setup.DisableServiceIfExists(ctx, "fwupd")
		if err != nil {
			s.Fatal("Failed to stop fwupd: ", err)
		}
		if startFwupdFn != nil {
			defer startFwupdFn(ctx)
		}
	}

	stopEvtest, err := startEvTestLogging(ctx, s)
	if err != nil {
		s.Fatal("Couldn't startEvTestLogging")
	}
	defer stopEvtest()

	_, err = suspend.ForDurationWithKernelFreezeTimeout(ctx, 10*time.Second, 8*time.Second)
	if err != nil {
		s.Fatal("Failed to suspend: ", err)
	}
}
