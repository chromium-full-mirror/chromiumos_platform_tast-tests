// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type inputInfo struct {
	TouchpadLibraryName string              `json:"touchpad_library_name"`
	TouchscreenDevices  []touchscreenDevice `json:"touchscreen_devices"`
	TouchpadDevices     *[]touchpadDevice   `json:"touchpad_devices"`
}

type touchpadDevice struct {
	DriverName  string      `json:"driver_name"`
	InputDevice inputDevice `json:"input_device"`
}

type touchscreenDevice struct {
	InputDevice           inputDevice `json:"input_device"`
	TouchPoints           int         `json:"touch_points"`
	HasStylus             bool        `json:"has_stylus"`
	HasStylusGarageSwitch bool        `json:"has_stylus_garage_switch"`
}

type inputDevice struct {
	Name             string `json:"name"`
	ConnectionType   string `json:"connection_type"`
	PhysicalLocation string `json:"physical_location"`
	IsEnabled        bool   `json:"is_enabled"`
}

type touchpadInfoTestParams struct {
	TouchpadValidation bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeInputInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for input info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"chungsheng@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Params: []testing.Param{{
			Name: "with_touchpad",
			Val: touchpadInfoTestParams{
				TouchpadValidation: true,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Touchpad()),
			ExtraAttr:         []string{"informational"},
		}, {
			Name: "",
			Val: touchpadInfoTestParams{
				TouchpadValidation: false,
			},
		}},
		Fixture: "crosHealthdRunning",
		Timeout: 1 * time.Minute,
	})
}

func validateTouchpads(ctx context.Context, info *inputInfo) error {
	if info.TouchpadDevices == nil || len(*info.TouchpadDevices) == 0 {
		return errors.New("failed to get touchpads")
	}
	return nil
}

func ProbeInputInfo(ctx context.Context, s *testing.State) {
	touchpadValidation := s.Param().(touchpadInfoTestParams).TouchpadValidation

	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryInput}
	var input inputInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &input); err != nil {
		s.Fatal("Failed to get input telemetry info: ", err)
	}

	if touchpadValidation {
		if err := validateTouchpads(ctx, &input); err != nil {
			s.Fatal("Failed to validate touchpad data: ", err)
		}
	}

	// In later CL, we'll get input device node information from Chrome.
	// With that, we may be able to do some verification of some fields.
	// In this CL, we only make sure that there is no crash when we fetch
	// input data.
}
