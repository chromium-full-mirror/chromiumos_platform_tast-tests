// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cbx

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/cbx/util"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FeatureEnabledCheck,
		Desc:         "Check if a cbx device has all the required features",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:cbx", "cbx_feature_enabled", "cbx_stable"},
	})
}

func FeatureEnabledCheck(ctx context.Context, s *testing.State) {
	features := util.CbxFeatures()
	for _, name := range features {
		enabled, err := util.Feature(ctx, fmt.Sprintf("FeatureManagement%s", name))
		if err != nil {
			s.Fatalf("Failed to check %q Feature: %v", name, err)
		}
		if !enabled {
			s.Errorf("Did not get expected %q feature", name)
		}
	}
}
