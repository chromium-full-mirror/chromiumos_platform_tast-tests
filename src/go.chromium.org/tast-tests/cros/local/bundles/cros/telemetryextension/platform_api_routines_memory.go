// Copyright 2023 The ChromiumOS Authors
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
		Func:         PlatformAPIRoutinesMemory,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests flow of the diagnostics Chrome Extension API exposed to Telemetry Extension",
		Contacts:     []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:mainline", "informational",
			"group:hw_agnostic",
			"group:criticalstaging",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: fixture.TelemetryExtensionSkipOEMNameCheck,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.TelemetryExtensionSkipOEMNameCheckLacros,
			},
		},
	})
}

// PlatformAPIRoutinesMemory tests the new chrome.os.diagnostics Chrome
// Extension API functionality. This test specifically tests communication between
// cros_healthd and the Telemetry Extension by invoking the memory routine with a
// really small limit and checking whether it traverses all states correctly.
func PlatformAPIRoutinesMemory(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	type response struct {
		Err       string `json:"err"`
		AllCalled bool   `json:"allCalled"`
	}
	var resp response
	if err := v.ExtConn.Eval(ctx,
		`(async () => {
		return await new Promise(async (resolve, reject) => {
			// Cancel after 10s.
			setTimeout(() => {
				reject(new Error("Promise timed out"));
			}, 10 * 1000);

			let error = "";
			let initCalled = false;
			let runningCalled = false;
			
			// First add an event listener for when a routine is initialized.
			chrome.os.diagnostics.onRoutineInitialized.addListener((status) => {
				initCalled = true;
				error = status !== undefined ? "" : "InitState has invalid status, ";
			});

			// Then add an event listener for when a routine is running.
			chrome.os.diagnostics.onRoutineRunning.addListener((status) => {
				runningCalled = true;
				error += status !== undefined ? "" : "RunningState has invalid status, ";
			});

			// Finally add a listener for when the routine is finished, resolving
			// the promise with all information from previous callbacks.
			chrome.os.diagnostics.onMemoryRoutineFinished.addListener((status) => {
				error += status !== undefined ? "" : "FinishedState has invalid status";

				resolve({
					err: error,
					allCalled: initCalled && runningCalled,
				});
			});

			// Then create the routine.
			const resp = await chrome.os.diagnostics.createMemoryRoutine({
				maxTestingMemKib: 10,
			});

			// And run it.
			await chrome.os.diagnostics.startRoutine({uuid: resp.uuid});
		})
	})()`, &resp); err != nil {
		s.Fatal("Error running the test: ", err)
	}

	if !resp.AllCalled {
		s.Error("Unexpected Error: routine did not traverse all states")
	}

	if len(resp.Err) != 0 {
		s.Errorf("Unexpected Error: routine did got unexpected data in the following states: %s", resp.Err)
	}
}
