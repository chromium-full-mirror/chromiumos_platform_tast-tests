// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/fixture"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlatformAPIEventsAudioJack,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests audio jack part of the chrome.os.events Chrome Extension API exposed to Telemetry Extension",
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

// PlatformAPIEventsAudioJack tests chrome.os.events Chrome Extension API functionality related to audio jack.
func PlatformAPIEventsAudioJack(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	// Fail fast in case the API is not defined.
	if err := v.ExtConn.Call(ctx, nil,
		"tast.promisify(chrome.os.events.stopCapturingEvents)", "audio_jack"); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	errorCh := make(chan error)
	defer func() {
		if err := <-errorCh; err != nil {
			s.Fatal("Failed to plug or unplug audio jack: ", err)
		}
	}()

	goroutineCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start a goroutine that plugs in the headphone every 200 milliseconds.
	go func(ctx context.Context) {
		var firstError error
		defer func() {
			errorCh <- firstError
		}()

		aw, err := input.AudioJack(ctx)
		if err != nil {
			firstError = err
			return
		}
		defer func(ctx context.Context) {
			if err := aw.Close(ctx); err != nil {
				if firstError != nil {
					testing.ContextLog(ctx, "Failed to close audio jack: ", err)
				} else {
					firstError = errors.Wrap(err, "failed to close audio jack")
				}
			}
		}(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
				// Trigger the audio jack event by plugging and unplugging.
				if err := aw.PlugOutHeadphone(); err != nil {
					firstError = errors.Wrap(err, "failed to plug out headphone")
					return
				}
				if err := aw.PlugInHeadphone(); err != nil {
					firstError = errors.Wrap(err, "failed to plug in headphone")
					return
				}
				if err := aw.PlugOutMicrophone(); err != nil {
					firstError = errors.Wrap(err, "failed to plug out microphone")
					return
				}
				if err := aw.PlugInMicrophone(); err != nil {
					firstError = errors.Wrap(err, "failed to plug in microphone")
					return
				}
			}
		}
	}(goroutineCtx)

	var resp string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Wait for an event or time out after 2s.
		err := v.ExtConn.Eval(ctx,
			`(async () => {
			return await new Promise(async (resolve, reject) => {
				// Cancel after 2s.
				setTimeout(() => {
					reject(new Error("Promise timed out"));
				}, 2 * 1000);

				// First add an event listener that stops listening on events
				// and resolves the promise.
				chrome.os.events.onAudioJackEvent.addListener((event) => {
					resolve(event.event);
				});

				// Then start capturing events.
				await chrome.os.events.startCapturingEvents("audio_jack");
			})
		})()`, &resp)

		// Stop observing the audio jack device.
		if jsErr := v.ExtConn.Call(ctx, nil,
			"tast.promisify(chrome.os.events.stopCapturingEvents)", "audio_jack"); jsErr != nil {
			if err != nil {
				testing.ContextLog(ctx, "Failed listen to events: ", err)
			}

			return testing.PollBreak(errors.Wrap(jsErr, "unable to call start and stop APIs, aborting test"))
		}

		if err != nil {
			return errors.Wrap(err, "failed to wait for audio jack event")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	if resp != "connected" && resp != "disconnected" {
		s.Errorf(`Unexpected event = got %q; want "connect" or "disconnect"`, resp)
	}
}
