// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlatformAPICPUPrimeSearchRoutine,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests chrome.os.diagnostics.runCpuPrimeSearchRoutine Chrome Extension API function exposed to Telemetry Extension",
		Contacts:     []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:mainline",
			"informational",
			"group:hw_agnostic",
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

func PlatformAPICPUPrimeSearchRoutine(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	type response struct {
		Status string `json:"status"`
		ID     int64  `json:"id"`
	}

	var resp response
	startReq := struct {
		LengthSeconds int64 `json:"length_seconds"`
	}{
		1,
	}

	// get result from the diagnostics API.
	if err := v.ExtConn.Call(ctx, &resp,
		"tast.promisify(chrome.os.diagnostics.runCpuPrimeSearchRoutine)", &startReq); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	// make sure the routine was started.
	if resp.Status != "running" {
		s.Errorf(`Unexpected routine status: got %q, want "running"`, resp.Status)
	}

	updateReq := struct {
		ID      int64  `json:"id"`
		Command string `json:"command"`
	}{
		resp.ID, "status",
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// request a routine update.
		if err := v.ExtConn.Call(ctx, &resp,
			"tast.promisify(chrome.os.diagnostics.getRoutineUpdate)", updateReq); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get response from Telemetry extension service worker"))
		}

		// assert the status is "passed" or "failed". We allow a failing routine (note: no
		// "error", "cancelled", etc.) because we can't make assumptions about the DUT's
		// state. "passed", and "failed" signal that the request successfully reached cros_healthd.
		if resp.Status != "passed" && resp.Status != "failed" {
			return errors.Errorf(`unexpected routine status: got %q; want "passed" or "failed"`, resp.Status)
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait routine to finish: ", err)
	}
}
