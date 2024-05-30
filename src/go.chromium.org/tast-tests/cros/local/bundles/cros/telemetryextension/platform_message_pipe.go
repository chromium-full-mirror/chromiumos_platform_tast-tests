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
		Func:         PlatformMessagePipe,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests message pipe functionality between PWA and Chrome extension",
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

// PlatformMessagePipe tests that PWA and Chrome extension have a capability to communicate with each other.
func PlatformMessagePipe(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	type telemetryRequest struct {
		InfoType string `json:"infoType"`
	}

	type request struct {
		Type      string           `json:"type"`
		Telemetry telemetryRequest `json:"telemetry"`
	}

	type response struct {
		Success   bool        `json:"success"`
		Telemetry interface{} `json:"telemetry"`
	}

	// chrome.runtime might not be available until a companion Chrome Extension is
	// completely installed, so wait until chrome.runtime.sendMessage is not null.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		available := false
		if err := v.PwaConn.Eval(ctx, "chrome.runtime !== null && chrome.runtime.sendMessage !== null", &available); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to verify whether chrome.runtime.sendMessage is null"))
		}
		if !available {
			return errors.New("chrome.runtime.sendMessage is null")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for response from Telemetry extension service worker: ", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var resp response
	if err := v.PwaConn.Call(ctx, &resp,
		"tast.promisify(chrome.runtime.sendMessage)",
		v.ExtID,
		request{Type: "telemetry", Telemetry: telemetryRequest{InfoType: "vpd"}},
	); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	if want := true; resp.Success != want {
		s.Errorf("Unexpected response success: got %t; want %t. Response telemetry: %v", resp.Success, want, resp.Telemetry)
	}
}
