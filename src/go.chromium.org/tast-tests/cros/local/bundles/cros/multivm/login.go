// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	arcpackage "go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/memory/metrics"
	"go.chromium.org/tast-tests/cros/local/multivm"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Login,
		Desc: "Tests Chrome Login with different VMs running",
		Contacts: []string{
			"arcvm-memory@google.com",
			"cwd@google.com",
		},
		// ChromeOS > Platform > baseOS > Virtualization > ARC++ & ARCVM
		BugComponent: "b:882467",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Timeout:      10 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "novm",
			Pre:  multivm.NoVMStarted(),
		}, {
			Name:              "arc_crostini",
			Pre:               multivm.ArcCrostiniStarted(),
			ExtraData:         []string{crostini.GetContainerMetadataArtifact("bookworm", false), crostini.GetContainerRootfsArtifact("bookworm", false)},
			ExtraHardwareDeps: crostini.CrostiniStable,
			ExtraSoftwareDeps: []string{"vm_host", "android_vm"},
		}, {
			Name:              "arc_vm",
			Pre:               multivm.ArcStartedNoSync(),
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name:              "crostini",
			Pre:               multivm.CrostiniStarted(),
			ExtraData:         []string{crostini.GetContainerMetadataArtifact("bookworm", false), crostini.GetContainerRootfsArtifact("bookworm", false)},
			ExtraHardwareDeps: crostini.CrostiniStable,
			ExtraSoftwareDeps: []string{"vm_host"},
		}},
	})
}

const (
	postLoginCoolDownDuration     = 10 * time.Second
	postDropCacheCoolDownDuration = 5 * time.Second
	quietDuration                 = 55 * time.Second
)

func Login(ctx context.Context, s *testing.State) {
	pre := s.PreValue().(*multivm.PreData)

	if err := pre.Chrome.Responded(ctx); err != nil {
		s.Fatal("Chrome did not respond: ", err)
	}
	arc := multivm.ARCFromPre(pre)
	basemem, err := metrics.NewBaseMemoryStats(ctx, arc)
	if err != nil {
		s.Fatal("Failed to retrieve base memory stats: ", err)
	}

	if arc != nil {
		// Ensures package manager service is running by checking the existence of the "android" package.
		pkgs, err := arc.InstalledPackages(ctx)
		if err != nil {
			s.Fatal("Getting installed packages failed: ", err)
		}

		if _, ok := pkgs["android"]; !ok {
			s.Fatal("Android package not found: ", pkgs)
		}
	}

	crostiniVM := multivm.CrostiniFromPre(pre)
	if crostiniVM != nil {
		if err := crostini.BasicCommandWorks(ctx, crostiniVM); err != nil {
			s.Fatal("Crostini basic commands don't work: ", err)
		}
	}

	p := perf.NewValues()
	if err := metrics.LogMemoryStats(ctx, basemem, arc, p, s.OutDir(), "_login"); err != nil {
		s.Error("Failed to collect memory metrics: ", err)
	}

	s.Log("No activity for a short while to cool down")
	// GoBigSleepLint: Cool down a little post login, before starting memory measurements.
	if err := testing.Sleep(ctx, postLoginCoolDownDuration); err != nil {
		s.Error("Failed to sleep: ", err)
	}

	arcvmEnabled := false
	if arc != nil {
		arcvmEnabled, err = arcpackage.VMEnabled()
		if err != nil {
			s.Fatal("Unable to query arc's VM status: ", err)
		}
	}

	if arcvmEnabled {
		if err := disk.DropCaches(ctx); err != nil {
			s.Fatal("Failed to drop host caches: ", err)
		}

		s.Log("Measuring system memory and pressure post dropcaches")
		if err := basemem.Reset(); err != nil {
			s.Error("Unable to reset metrics: ", err)
		}
		// GoBigSleepLint: Take some time so that the drop-cache operation doesn't affect memory pressure.
		if err := testing.Sleep(ctx, postDropCacheCoolDownDuration); err != nil {
			s.Error("Failed to sleep: ", err)
		}
		s.Log("Will now collect post-dropcache perf values")
		if err := metrics.LogMemoryStats(ctx, basemem, arc, p, s.OutDir(), "_cachedropped"); err != nil {
			s.Error("Failed to collect memory metrics: ", err)
		}
	} else {
		s.Log("No ARCVM, skip waiting for dropCaches")
	}

	s.Log("Measuring system memory and pressure in idle state")
	if err := basemem.Reset(); err != nil {
		s.Error("Unable to reset metrics: ", err)
	}
	// GoBigSleepLint: Let the system quiesce for a while and measure its memory consumption.
	if err := testing.Sleep(ctx, quietDuration); err != nil {
		s.Error("Failed to sleep: ", err)
	}
	s.Log("Will now collect idle perf values")
	if err := metrics.LogMemoryStats(ctx, basemem, arc, p, s.OutDir(), "_quiesce"); err != nil {
		s.Error("Failed to collect memory metrics: ", err)
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf.Values: ", err)
	}
}
