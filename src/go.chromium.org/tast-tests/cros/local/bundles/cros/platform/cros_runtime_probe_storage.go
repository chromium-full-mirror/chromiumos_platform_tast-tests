// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"

	rppb "chromiumos/system_api/runtime_probe_proto"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/platform/runtimeprobe"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CrosRuntimeProbeStorage,
		Desc: "Checks that storage probe results are expected",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"clarkchung@google.com",
		},
		BugComponent: "b:606088",
		Attr:         []string{"group:racc", "racc_config_installed"},
		SoftwareDeps: []string{"racc"},
		HardwareDeps: hwdep.D(hwdep.RuntimeProbeConfig()),
		Vars:         []string{"autotest_host_info_labels"},
	})
}

// CrosRuntimeProbeStorage checks if the storage names in cros-label are consistent with probed names from runtime_probe
func CrosRuntimeProbeStorage(ctx context.Context, s *testing.State) {
	categories := []string{"storage"}
	getComponents := func(result *rppb.ProbeResult, category string) ([]runtimeprobe.Component, error) {
		var comps []runtimeprobe.Component
		var rppbComps []*rppb.Storage
		switch category {
		case "storage":
			rppbComps = result.GetStorage()
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
