// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"strconv"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/crosconfig"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrosConfig,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check and verify camera configuration",
		Contacts:     []string{"chromeos-camera-eng@google.com", "rabbim@chromium.org"},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{caps.BuiltinCamera},
		BugComponent: "b:167281",
	})
}

func hasCameraConfig(ctx context.Context) error {
	_, err := crosconfig.Get(ctx, "/camera", "count")
	if crosconfig.IsNotFound(err) {
		return errors.Wrap(err, "cros_config camera is not available")
	}
	if err != nil {
		return errors.Wrap(err, "failed to execute cros_config")
	}
	return nil
}

func verifyContent(ctx context.Context) error {
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
			return errors.Wrap(err, "failed to execute cros_config for camera devices")
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
		return errors.Wrap(err, "failed to execute cros_config for camera count")
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
	if err := hasCameraConfig(ctx); err != nil {
		s.Fatal("Failed to get camera config: ", err)
	}
	if err := verifyContent(ctx); err != nil {
		s.Fatalf("Content of the config file could not be verified :%v", err)
	}
}
