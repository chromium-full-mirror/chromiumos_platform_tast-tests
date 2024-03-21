// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/memory/mempressure"
	"go.chromium.org/tast/core/testing"
)

type memoryPressureParams struct {
	enableARC    bool
	useHugePages bool
	useVulkan    bool
	bt           browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryPressure,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Create memory pressure and collect various measurements from Chrome and from the kernel",
		Contacts:     []string{"chromeos-memory@google.com"},
		BugComponent: "b:167286",
		Attr:         []string{"group:crosbolt"},
		Timeout:      180 * time.Minute,
		Data: []string{
			mempressure.CompressibleData,
			mempressure.WPRArchiveName,
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Val: memoryPressureParams{enableARC: false, useHugePages: false, useVulkan: false, bt: browser.TypeAsh},
			ExtraAttr: []string{"crosbolt_memory_nightly"},
		}, {
			Name:              "vm",
			Val:               memoryPressureParams{enableARC: true, useHugePages: false, useVulkan: false, bt: browser.TypeAsh},
			ExtraAttr:         []string{"crosbolt_arc_perf_qual", "crosbolt_arc_perf_memory_nightly"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name:              "huge_pages_vm",
			Val:               memoryPressureParams{enableARC: true, useHugePages: true, useVulkan: false, bt: browser.TypeAsh},
			ExtraAttr: []string{"crosbolt_memory_nightly"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name:              "container",
			Val:               memoryPressureParams{enableARC: true, useHugePages: false, useVulkan: false, bt: browser.TypeAsh},
			ExtraAttr:         []string{"crosbolt_arc_perf_qual", "crosbolt_arc_perf_memory_nightly"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "lacros",
			Val:               memoryPressureParams{enableARC: false, useHugePages: false, useVulkan: false, bt: browser.TypeLacros},
			ExtraAttr: []string{"crosbolt_memory_nightly"},
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:              "vulkan",
			Val:               memoryPressureParams{enableARC: false, useHugePages: false, useVulkan: true, bt: browser.TypeAsh},
			ExtraAttr: []string{"crosbolt_memory_nightly"},
			ExtraSoftwareDeps: []string{"vulkan_composite"},
		}},
	})
}

// MemoryPressure is the main test function.
func MemoryPressure(ctx context.Context, s *testing.State) {
	enableARC := s.Param().(memoryPressureParams).enableARC
	useHugePages := s.Param().(memoryPressureParams).useHugePages
	useVulkan := s.Param().(memoryPressureParams).useVulkan
	bt := s.Param().(memoryPressureParams).bt

	testEnv, err := mempressure.NewTestEnv(ctx, s.OutDir(), enableARC, useHugePages, useVulkan, bt, s.DataPath(mempressure.WPRArchiveName))
	if err != nil {
		s.Fatal("Failed creating the test environment: ", err)
	}
	defer testEnv.Close(ctx)

	p := &mempressure.RunParameters{
		PageFilePath:             s.DataPath(mempressure.CompressibleData),
		PageFileCompressionRatio: 0.40,
	}

	if err := mempressure.Run(ctx, s.OutDir(), testEnv.Browser(), testEnv.ARC(), p); err != nil {
		s.Fatal("Run failed: ", err)
	}
}
