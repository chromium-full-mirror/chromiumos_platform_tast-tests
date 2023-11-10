// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package geekbenchcuj

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/local/ui/geekbench"
	"go.chromium.org/tast/core/testing"
)

// Run runs and collects Geekbench benchmark scores.
func Run(ctx context.Context, s *testing.State) {
	gbInfo := s.Param().(geekbench.GBInfo)

	if gbInfo.Name == "" {
		s.Fatal("Failed to parse valid geekbenchInfo struct")
	}

	var fixtValue interface{}

	if strings.Contains(gbInfo.Name, "crostini") {
		fixtValue = s.PreValue()
	} else {
		fixtValue = s.FixtValue()
	}

	stateVars := make(map[string]string)

	geekbenchPlar := fmt.Sprintf("geekbench%d.plar", gbInfo.Version)
	stateVars[geekbenchPlar] = s.DataPath(geekbenchPlar)

	geekbenchX86 := fmt.Sprintf("geekbench%d_x86_64", gbInfo.Version)
	stateVars[geekbenchX86] = s.DataPath(geekbenchX86)

	if strings.Contains(gbInfo.Name, "custom") {
		stateVars["geekbench_custom"] = s.DataPath("geekbench_custom")
	}

	if gbInfo.Version == 6 {
		stateVars["geekbench6-workload.plar"] = s.DataPath("geekbench6-workload.plar")
	}

	if err := geekbench.Run(ctx, gbInfo, fixtValue, stateVars); err != nil {
		s.Fatal("Failed to run GeekbenchCUJL: ", err)
	}
}
