// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cbx

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/cbx/util"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FeatureEnabledLevelCheck,
		Desc:         "Check if a cbx device has correct feature level",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:cbx", "cbx_feature_enabled", "cbx_stable"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
	})
}

func FeatureEnabledLevelCheck(ctx context.Context, s *testing.State) {
	level, err := util.FeatureLevel(ctx)
	if err != nil {
		s.Fatal("Failed to get feature level: ", err)
	}
	if level < 1 {
		s.Fatal("Got unexpected feature level: ", level)
	}
}
