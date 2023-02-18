// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"

	rppb "chromiumos/system_api/runtime_probe_proto"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/platform/runtimeprobe"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CrosRuntimeProbeMemory,
		Desc: "Checks that memory probe results are expected",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"clarkchung@google.com",
		},
		BugComponent: "b:606088",
		Attr:         []string{"group:runtime_probe"},
		SoftwareDeps: []string{"racc"},
		HardwareDeps: hwdep.D(hwdep.RuntimeProbeConfig()),
		Vars:         []string{"autotest_host_info_labels"},
	})
}

// CrosRuntimeProbeMemory checks if the memory names in cros-label are
// consistent with probed names from runtime_probe.
func CrosRuntimeProbeMemory(ctx context.Context, s *testing.State) {
	categories := []string{"dram"}
	getComponents := func(result *rppb.ProbeResult, category string) ([]runtimeprobe.Component, error) {
		var comps []runtimeprobe.Component
		var rppbComps []*rppb.Memory
		switch category {
		case "dram":
			rppbComps = result.GetDram()
		default:
			return nil, errors.Errorf("unknown category %s", category)
		}
		for _, comp := range rppbComps {
			comps = append(comps, comp)
		}
		return comps, nil
	}
	runtimeprobe.GenericTest(ctx, s, categories, getComponents, false)
}
