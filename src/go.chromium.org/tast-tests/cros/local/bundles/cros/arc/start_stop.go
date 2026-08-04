// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/startstop"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

// startStopTestArgs represents the arguments passed to each parameterized test.
type startStopTestArgs struct {
	subtests         []startstop.Subtest // subtests represents the subtests to run.
	fieldTrialConfig int                 // Value for FieldTrialConfig Chrome parameter
}

func init() {
	testing.AddTest(&testing.Test{
		Func: StartStop,
		Desc: "Verifies clean start and stop of CrOS Chrome and Android container",
		Contacts: []string{
			"arcvm-software@google.com", // Owner team.
			"raging@google.com",         // Owner for VM tests.

			// Contacts for TestMidis.
			"pmalani@chromium.org", // original author
			"chromeos-audio@google.com",

			"hidehiko@chromium.org", // Tast port author.
		},
		BugComponent: "b:883059",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: arc.ArcBootHwDeps,
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				Val: startStopTestArgs{
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
					subtests: []startstop.Subtest{
						&startstop.TestMidis{},
						&startstop.TestPID{},
					},
				},
			},
			{
				Name:              "fieldtrial_testing_config_off_vm",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Val: startStopTestArgs{
					fieldTrialConfig: chrome.FieldTrialConfigDisable,
					subtests: []startstop.Subtest{
						&startstop.TestMidis{},
						&startstop.TestPID{},
					},
				},
			},
			{
				Name:              "fieldtrial_testing_config_on_vm",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Val: startStopTestArgs{
					fieldTrialConfig: chrome.FieldTrialConfigEnable,
					subtests: []startstop.Subtest{
						&startstop.TestMidis{},
						&startstop.TestPID{},
					},
				},
			}},
	})
}

func StartStop(ctx context.Context, s *testing.State) {
	args := s.Param().(startStopTestArgs)

	s.Log("Restarting Chrome")

	// Restart ui job to ensure starting from logout state.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to log out: ", err)
	}
	for _, t := range args.subtests {
		s.Run(ctx, t.Name()+".PreStart", t.PreStart)
	}

	s.Log("Starting ARC")

	// Launch Chrome with enabling ARC.
	func() {
		cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.FieldTrialConfig(args.fieldTrialConfig))
		if err != nil {
			s.Fatal("Failed to connect to Chrome: ", err)
		}
		defer cr.Close(ctx)

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer a.Close(ctx)

		for _, t := range args.subtests {
			s.Run(ctx, t.Name()+".PostStart", t.PostStart)
		}
	}()

	s.Log("Stopping ARC and Chrome")

	// Log out from Chrome, which shuts down ARC.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to log out: ", err)
	}
	for _, t := range args.subtests {
		s.Run(ctx, t.Name()+".PostStop", t.PostStop)
	}

	s.Log("Restoring Chrome")

	if err := upstart.StartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to restart: ", err)
	}
}
