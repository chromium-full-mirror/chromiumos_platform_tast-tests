// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IsVariantKnown,
		Desc:         "Verifies that the variant is known",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active"},
	})
}

// IsVariantKnown Test
func IsVariantKnown(ctx context.Context, s *testing.State) {
	if err := cellular.IsVariantKnown(ctx); err != nil {
		s.Fatal("variant not known: ", err)
	}
}
