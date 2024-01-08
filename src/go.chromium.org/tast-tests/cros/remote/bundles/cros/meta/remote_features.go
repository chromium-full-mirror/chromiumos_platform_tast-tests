// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteFeatures,
		Desc:         "Example to access DUT features from a remote test",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:hw_agnostic"},
	})
}

func RemoteFeatures(ctx context.Context, s *testing.State) {
	dutFeatures := s.Features("")
	s.Logf("DUT Features: %+v", dutFeatures)
}
