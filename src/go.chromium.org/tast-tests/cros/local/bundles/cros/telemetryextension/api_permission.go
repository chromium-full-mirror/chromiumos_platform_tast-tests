// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           APIPermission,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Tests extension access permission. Test one API is sufficient because API guard is a single entry point for telemetry and diagnostics APIs. It verifies whether APIs can be called within caller context",
		Contacts:       []string{"cros-tdm-tpe-eng@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr:         []string{"group:telemetry_extension_hw"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.TelemetryExtension,
		Params: []testing.Param{
			{
				Name:              "supported",
				Val:               true,
				ExtraHardwareDeps: hwdep.D(hwdep.OEM("ASUS", "HP")),
			},
			{
				Name:              "not_supported",
				Val:               false,
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnOEM("ASUS", "HP")),
			},
		},
	})
}

func APIPermission(ctx context.Context, s *testing.State) {
	supported, ok := s.Param().(bool)
	if !ok {
		s.Fatal("Failed to convert params value into bool: ", s.Param())
	}

	type response struct {
		Routines []string `json:"routines"`
	}
	var resp response
	v := s.FixtValue().(*fixture.Value)
	if err := v.ExtConn.Call(ctx, &resp,
		"tast.promisify(chrome.os.diagnostics.getAvailableRoutines)",
	); err != nil {
		if !supported {
			if strings.Contains(err.Error(), "Unauthorized access to chrome.os.diagnostics.getAvailableRoutines") {
				return
			}
			s.Fatal("Unexpected error message accessing Telemetry extension service for unsupported models: ", err)
		}

		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	} else {
		if !supported {
			s.Fatal("Not supported models should raise unauthorized error")
		}
	}
}
