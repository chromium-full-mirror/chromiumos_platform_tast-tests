// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/memory/mempressure"
	"go.chromium.org/tast/core/testing"
)

type memoryPressureParams struct {
	enableARC    bool
	useHugePages bool
	useVulkan    bool
}

const (
	forceTabVarName = "platform.MemoryPressure.forceTab"
)

var forceTabVar = testing.RegisterVarString(
	forceTabVarName,
	"0",
	"The number of forcibly open tabs (ignoreing discard). 0 means disabling this feature.",
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryPressure,
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
			Val:       memoryPressureParams{enableARC: false, useHugePages: false, useVulkan: false},
			ExtraAttr: []string{"crosbolt_memory_nightly"},
		}, {
			Name:              "vm",
			Val:               memoryPressureParams{enableARC: true, useHugePages: false, useVulkan: false},
			ExtraAttr:         []string{"crosbolt_arc_perf_qual", "crosbolt_arc_perf_memory_nightly"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name:              "huge_pages_vm",
			Val:               memoryPressureParams{enableARC: true, useHugePages: true, useVulkan: false},
			ExtraAttr:         []string{"crosbolt_memory_nightly"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name:              "vulkan",
			Val:               memoryPressureParams{enableARC: false, useHugePages: false, useVulkan: true},
			ExtraAttr:         []string{"crosbolt_memory_nightly"},
			ExtraSoftwareDeps: []string{"vulkan_composite"},
		}},
	})
}

// MemoryPressure is the main test function.
func MemoryPressure(ctx context.Context, s *testing.State) {
	enableARC := s.Param().(memoryPressureParams).enableARC
	useHugePages := s.Param().(memoryPressureParams).useHugePages
	useVulkan := s.Param().(memoryPressureParams).useVulkan
	forceTab, err := strconv.Atoi(forceTabVar.Value())
	if err != nil {
		s.Fatalf("Failed to parse %s: %v", forceTabVarName, err)
	}

	testEnv, err := mempressure.NewTestEnv(ctx, s.OutDir(), enableARC, useHugePages, useVulkan, s.DataPath(mempressure.WPRArchiveName))
	if err != nil {
		s.Fatal("Failed creating the test environment: ", err)
	}
	defer testEnv.Close(ctx)

	p := &mempressure.RunParameters{
		PageFilePath:             s.DataPath(mempressure.CompressibleData),
		PageFileCompressionRatio: 0.40,
		MaxTabCount:              forceTab,
		IgnoreDiscard:            forceTab != 0,
	}

	if err := mempressure.Run(ctx, s.OutDir(), testEnv.Browser(), testEnv.ARC(), p); err != nil {
		s.Fatal("Run failed: ", err)
	}
}
