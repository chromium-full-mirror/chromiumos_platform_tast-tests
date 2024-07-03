// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/testing"
)

type bootTestArgs struct {
	// Run boot this many times
	numTrials int
	// Extra Chrome command line options
	chromeArgs []string
	// Check that the Virtual Machine Memory Management Service is running.
	checkVMMMS bool
	// Value for FieldTrialConfig Chrome parameter
	fieldTrialConfig int
	// Boot and allow Doze Mode to kick in quickly.
	quickDozeMode bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Boot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Android boots",
		Contacts: []string{
			// Please assign test failures to current constable on-call
			"arc-constables@google.com",
			"arc-core@google.com",
		},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
				},
				ExtraAttr:         []string{"group:mainline"},
				ExtraSoftwareDeps: []string{"android_container"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "fieldtrial_testing_config_off",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDisable,
				},
				ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraSoftwareDeps: []string{"android_container"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "fieldtrial_testing_config_on",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigEnable,
				},
				ExtraAttr:         []string{"group:mainline", "informational", "group:chrome_uprev_cbx"},
				ExtraSoftwareDeps: []string{"android_container"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "forever",
				Val: bootTestArgs{
					numTrials:        1000000,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				Timeout:           365 * 24 * time.Hour,
			},
			{
				Name: "stress",
				Val: bootTestArgs{
					numTrials:        10,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
				},
				ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraSoftwareDeps: []string{"android_container"},
				Timeout:           25 * time.Minute,
			},
			{
				Name: "vm",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
				},
				ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "vm_quickdoze",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
					quickDozeMode:    true,
					chromeArgs: []string{
						"--enable-features=ArcIdleManager:ignore_battery_for_test/true/delay_ms/10000",
					},
				},
				ExtraAttr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "fieldtrial_testing_config_off_vm",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDisable,
				},
				ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "fieldtrial_testing_config_on_vm",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigEnable,
				},
				ExtraAttr:         []string{"group:mainline", "informational", "group:chrome_uprev_cbx"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "vm_virtio_blk",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
					chromeArgs: []string{
						"--enable-features=ArcEnableVirtioBlkForData",
					},
				},
				ExtraAttr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "vm_with_per_vcpu_core_scheduling",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
					// Switch from per-VM core scheduling to per-vCPU core scheduling which
					// is more secure but slow.
					chromeArgs: []string{"--disable-features=ArcEnablePerVmCoreScheduling"},
				},
				ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				Name: "vm_forever",
				Val: bootTestArgs{
					numTrials:        1000000,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
				},
				ExtraAttr:         []string{"group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           365 * 24 * time.Hour,
			},
			{
				Name: "vm_stress",
				Val: bootTestArgs{
					numTrials:        10,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
				},
				ExtraAttr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           25 * time.Minute,
			},
			{
				Name: "vm_large_memory",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
					// Boot ARCVM with the largest possible guest memory size.
					chromeArgs: []string{"--enable-features=ArcVmMemorySize:shift_mib/0"},
				},
				ExtraAttr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			},
			{
				// TODO(b:322724008): Remove this param and enable checkVMMMS on all other .vm* tests once we know this is stable.
				Name: "vm_check_vmmms",
				Val: bootTestArgs{
					numTrials:        1,
					fieldTrialConfig: chrome.FieldTrialConfigDefault,
					checkVMMMS:       true,
				},
				ExtraAttr:         []string{"group:mainline", "informational", "group:criticalstaging", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
			}},
	})
}

func Boot(ctx context.Context, s *testing.State) {
	numTrials := s.Param().(bootTestArgs).numTrials
	for i := 0; i < numTrials; i++ {
		if numTrials > 1 {
			s.Logf("Trial %d/%d", i+1, numTrials)
		}
		runBoot(ctx, s)
	}
}

func runBoot(ctx context.Context, s *testing.State) {
	var vmmmsVerify *memory.VmmmsInitVerifier
	args := s.Param().(bootTestArgs)

	if args.checkVMMMS {
		var err error
		vmmmsVerify, err = memory.NewVmmmsInitVerifier(ctx)
		if err != nil {
			s.Fatal("Failed to create VmmmsInitVerifier: ", err)
		}
		defer vmmmsVerify.Close()
	}

	reader, err := syslog.NewReader(ctx)
	if err != nil {
		s.Fatal("Failed to open syslog reader: ", err)
	}
	defer reader.Close()

	opts := []chrome.Option{
		chrome.ARCEnabled(),
		chrome.FieldTrialConfig(args.fieldTrialConfig),
		chrome.ExtraArgs(args.chromeArgs...),
	}
	if !args.quickDozeMode {
		opts = append(opts, chrome.UnRestrictARCCPU())
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		if err := cr.Close(ctx); err != nil {
			s.Fatal("Failed to close Chrome while booting ARC: ", err)
		}
	}()

	a, err := arc.NewWithSyslogReader(ctx, s.OutDir(), reader, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	// Ensures package manager service is running by checking the existence of the "android" package.
	pkgs, err := a.InstalledPackages(ctx)
	if err != nil {
		s.Fatal("Getting installed packages failed: ", err)
	}

	if _, ok := pkgs["android"]; !ok {
		s.Fatal("android package not found: ", pkgs)
	}

	// Ensures that the Virtual Machine Memory Management Service initialized successfully.
	if vmmmsVerify != nil {
		if err := vmmmsVerify.Verify(ctx); err != nil {
			s.Fatal("VMMMS did not initialized properly: ", err)
		}
	}
}
