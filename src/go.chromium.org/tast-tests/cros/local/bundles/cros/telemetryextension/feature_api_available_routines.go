// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           FeatureAPIAvailableRoutines,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Tests chrome.os.diagnostics.getAvailableRoutines Chrome Extension API function exposed to Telemetry Extension and check availability of all routines",
		Contacts:       []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr:         []string{"group:telemetry_extension_hw"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.TelemetryExtension,
		Params: []testing.Param{
			// OEMs need to fill in configs to enable the fingerprint routine.
			// See https://chromium.googlesource.com/chromium/src/+/HEAD/docs/telemetry_extension/fingerprint_diag.md
			{
				Name: "fingerprint_alive",
				Val:  "fingerprint_alive",
			},
		},
	})
}

func FeatureAPIAvailableRoutines(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	routineName, ok := s.Param().(string)
	if !ok {
		s.Fatal("Failed to convert params value into string: ", s.Param())
	}

	type response struct {
		Routines []string `json:"routines"`
	}

	var resp response
	if err := v.ExtConn.Call(ctx, &resp,
		"tast.promisify(chrome.os.diagnostics.getAvailableRoutines)",
	); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	contains := func(list []string, want string) bool {
		for _, elem := range list {
			if elem == want {
				return true
			}
		}
		return false
	}

	if !contains(resp.Routines, routineName) {
		s.Errorf(`Unexpected result from "getAvailableRoutines": %q is not present in %v as expected`, routineName, resp.Routines)
	}
}
