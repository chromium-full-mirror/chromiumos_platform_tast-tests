// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package virtualmultidisplay

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func ensureVirtioGpuDummyModuleLoaded() error {
	return ensureModuleLoadedOrNot("virtio_gpu_dummy", true)
}

func ensureVkmsDriverLoaded() error {
	return ensureModuleLoadedOrNot("vkms", true)
}

func ensureModuleLoadedOrNot(moduleName string, loaded bool) error {
	f, err := os.Open("/proc/modules")
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)

	sc.Split(bufio.ScanLines)

	for sc.Scan() {
		modName := strings.Split(sc.Text(), " ")[0]
		if modName == moduleName {
			if !loaded {
				return errors.Errorf("expected module %s to not be loaded but it was", moduleName)
			}

			return nil
		}
	}

	if loaded {
		return errors.New("virtio_gpu_dummy is not loaded, cannot use VirtioGpuDummyMultiDisplayController")
	}

	return nil
}

func killDrmProcessesAndServices(ctx context.Context) error {
	for _, drmService := range drmServices {
		testing.ContextLogf(ctx, "stopping %s", drmService)
		if err := upstart.StopJob(ctx, drmService); err != nil {
			return errors.Wrapf(err, "could not stop service %q", drmService)
		}
	}

	testing.ContextLog(ctx, "Killing all DRM processes so that the video drivers can be loaded/unloaded")
	// DRM clients can be easily checked by lsof /dev/dri/card0 and lsof /dev/dri/renderD128.
	// Error ignored because lsof will return an error 1 when nothing is found.
	psToKill, _ := testexec.CommandContext(ctx, "lsof", "-t", "/dev/dri/card0", "/dev/dri/renderD128").Output(testexec.DumpLogOnError)
	pToKillSpaced := strings.Trim(strings.ReplaceAll(string(psToKill), "\n", " "), " ")
	// Logs all processes that are DRM clients that will be killed for the driver to be reloaded.
	testing.ContextLogf(ctx, "Proceses that will be killed: %s", pToKillSpaced)

	for _, pid := range strings.Split(pToKillSpaced, " ") {
		pidi, err := strconv.Atoi(pid)
		if err != nil {
			// This shouldn't happen, but if it does no need to end the test as long as we can still reload the driver.
			testing.ContextLogf(ctx, "Invalid pid %s: %s", pid, err)
			continue
		}

		// Some processes are transient, so check if they're still alive.
		p, err := os.FindProcess(pidi)
		if err != nil {
			testing.ContextLogf(ctx, "No such process %d", pidi)
			continue
		}

		if err := p.Kill(); err != nil {
			testing.ContextLogf(ctx, "Warning: Could not kill process %q, it may have been transient: %s", p, err)
		}
	}

	return nil
}

func resumeDrmServices(ctx context.Context) error {
	for _, drmService := range drmServices {
		testing.ContextLogf(ctx, "resuming %s", drmService)
		if err := upstart.EnsureJobRunning(ctx, drmService); err != nil {
			return errors.Wrapf(err, "could not start service %q", drmService)
		}
	}

	return nil
}
