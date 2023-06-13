// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/autocaps"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const crosConfigCmdFail = "failed to execute cros_config"

// These are just a few examples, the actual list is too long.
// TODO(b/260046862): enable on all devices.
var crosConfigReadyModels = []string{
	"atlas",
	"nautilus",
	"primus",
	"jinlon",
	"vell",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrosConfig,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check and verify camera configuration",
		Contacts:     []string{"chromeos-camera-eng@google.com", "yerlandinata@chromium.org"},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{caps.BuiltinCamera},
		HardwareDeps: hwdep.D(hwdep.Model(crosConfigReadyModels...)),
		BugComponent: "b:167281",
	})
}

func hasCameraConfig(ctx context.Context) (bool, error) {
	countStr, err := crosconfig.Get(ctx, "/camera", "count")
	if crosconfig.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, crosConfigCmdFail)
	}

	count, err := strconv.Atoi(countStr)
	if err != nil {
		return false, err
	}

	if count < 1 {
		return false, nil
	}

	_, err = crosconfig.Get(ctx, "/camera/devices/0", "interface")
	if crosconfig.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, crosConfigCmdFail)
	}

	return true, nil
}

func verifyConfigConsistency(ctx context.Context) error {
	cameraCount := 0
	foundUsb := false
	foundMipi := false

	for i := 0; ; i++ {
		devicePath := fmt.Sprintf("/camera/devices/%v", i)
		cameraType, err := crosconfig.Get(ctx, devicePath, "interface")
		if crosconfig.IsNotFound(err) {
			break
		}
		if err != nil {
			return errors.Wrapf(err, "%s for camera devices", crosConfigCmdFail)
		}
		if cameraType == "usb" {
			foundUsb = true
			cameraCount++
		} else if cameraType == "mipi" {
			foundMipi = true
			cameraCount++
		} else {
			return errors.Errorf("invalid camera type found : %s", cameraType)
		}
	}
	// verify /camera/count equals to the number of /camera/devices/*
	configCount, err := crosconfig.Get(ctx, "/camera/", "count")
	if err != nil {
		return errors.Wrapf(err, "%s for camera count", crosConfigCmdFail)
	}

	if configCount != strconv.Itoa(cameraCount) {
		errors.Errorf(" camera count mismatch, expectedCount : %s, found: %d", configCount, cameraCount)
	}

	// Get capabilities defined in autocaps package
	staticCaps, err := autocaps.Read(autocaps.DefaultCapabilityDir, nil)
	if err != nil {
		return errors.Wrap(err, "failed to read statically-set capabilities")
	}

	// verify caps.Builtin{USB,MIPI}Camera matches /camera/*/interface
	capsToVerify := map[string]bool{
		"builtin_usb_camera":  foundUsb,
		"builtin_mipi_camera": foundMipi,
		"builtin_camera":      foundUsb || foundMipi,
	}

	for c, found := range capsToVerify {
		if staticCaps[c] == autocaps.Yes && !found {
			return errors.Errorf("%s statically set but not found in cros_config", c)
		}
		if staticCaps[c] != autocaps.Yes && found {
			return errors.Errorf("%s found in cros_config but not statically set", c)
		}
	}
	return nil
}

// CrosConfig checks if camera config available and verify it's content
func CrosConfig(ctx context.Context, s *testing.State) {
	// TODO(b/260046862): enable on all devices.
	hasConfig, err := hasCameraConfig(ctx)
	if err != nil {
		s.Fatal("Failed to get camera config: ", err.Error())
	}
	if !hasConfig {
		s.Fatal("Device doesn't have camera config in cros_config")
	}
	if err := verifyConfigConsistency(ctx); err != nil {
		s.Fatalf("Content of the config file could not be verified :%v", err)
	}
}
