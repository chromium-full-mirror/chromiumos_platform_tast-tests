// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/testing"
)

type bootConfig struct {
	// Run boot this many times
	numTrials int
	// Use O_DIRECT in read-only system/vendor disk access for ARCVM
	rootfsODirect bool
	// Use O_DIRECT in /data disk access for ARCVM
	dataDiskODirect bool
	// Extra Chrome command line options
	chromeArgs []string
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
		Params: []testing.Param{{
			Val: bootConfig{
				numTrials: 1,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"android_container"},
			Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		}, {
			Name: "forever",
			Val: bootConfig{
				numTrials: 1000000,
			},
			ExtraSoftwareDeps: []string{"android_container"},
			Timeout:           365 * 24 * time.Hour,
		}, {
			Name: "stress",
			Val: bootConfig{
				numTrials: 10,
			},
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraSoftwareDeps: []string{"android_container"},
			Timeout:           25 * time.Minute,
		}, {
			Name: "vm",
			Val: bootConfig{
				numTrials: 1,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		}, {
			Name: "vm_virtio_blk",
			Val: bootConfig{
				numTrials: 1,
				chromeArgs: []string{
					"--enable-features=ArcEnableVirtioBlkForData",
				},
			},
			ExtraAttr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		}, {
			Name: "vm_virtio_blk_data_o_direct",
			Val: bootConfig{
				numTrials: 1,
				chromeArgs: []string{
					"--enable-features=ArcEnableVirtioBlkForData",
				},
				dataDiskODirect: true,
			},
			ExtraAttr:         []string{"group:mainline", "informational", "group:criticalstaging", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		}, {
			Name: "vm_with_per_vcpu_core_scheduling",
			Val: bootConfig{
				numTrials: 1,
				// Switch from per-VM core scheduling to per-vCPU core scheduling which
				// is more secure but slow.
				chromeArgs: []string{"--disable-features=ArcEnablePerVmCoreScheduling"},
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		}, {
			Name: "vm_forever",
			Val: bootConfig{
				numTrials: 1000000,
			},
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Timeout:           365 * 24 * time.Hour,
		}, {
			Name: "vm_o_direct",
			Val: bootConfig{
				numTrials:     1,
				rootfsODirect: true,
			},
			ExtraAttr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		}, {
			Name: "vm_stress",
			Val: bootConfig{
				numTrials: 10,
			},
			ExtraAttr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Timeout:           25 * time.Minute,
		}, {
			Name: "vm_large_memory",
			Val: bootConfig{
				numTrials: 1,
				// Boot ARCVM with the largest possible guest memory size.
				chromeArgs: []string{"--enable-features=ArcVmMemorySize:shift_mib/0"},
			},
			ExtraAttr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Timeout:           chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		}},
	})
}

func Boot(ctx context.Context, s *testing.State) {
	numTrials := s.Param().(bootConfig).numTrials
	for i := 0; i < numTrials; i++ {
		if numTrials > 1 {
			s.Logf("Trial %d/%d", i+1, numTrials)
		}
		runBoot(ctx, s)
	}
}

func runBoot(ctx context.Context, s *testing.State) {
	arcvmConf := ""
	if s.Param().(bootConfig).rootfsODirect {
		// Set up O_DIRECT for /dev/vda (system.img) and /dev/vdb (vendor.img).
		arcvmConf += "O_DIRECT_N=0\nO_DIRECT_N=1\n"
	}
	if s.Param().(bootConfig).dataDiskODirect {
		arcvmConf += "O_DIRECT_N=4\n"
	}
	if arcvmConf != "" {
		if err := arc.WriteArcvmDevConf(ctx, arcvmConf); err != nil {
			s.Fatal("Failed to set arcvm_dev.conf: ", err)
		}
		defer arc.RestoreArcvmDevConf(ctx)
	}

	reader, err := syslog.NewReader(ctx)
	if err != nil {
		s.Fatal("Failed to open syslog reader: ", err)
	}
	defer reader.Close()

	args := s.Param().(bootConfig).chromeArgs
	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.ExtraArgs(args...))
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
}
