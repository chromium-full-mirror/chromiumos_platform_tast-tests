// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cbx

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FeatureCheck,
		Desc:         "Check if a cbx device has all the required features",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:cbx", "cbx_feature_enabled", "cbx_stable"},
	})
}

func FeatureCheck(ctx context.Context, s *testing.State) {
}
