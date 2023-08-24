// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/android"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AndroidCompanions,
		Desc:         "Demonstrate how to get Android companion devices information",
		Contacts:     []string{"tast-core@google.com", "seewaifu@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		VarDeps:      []string{android.CompanionsVarName},
	})
}

func AndroidCompanions(ctx context.Context, s *testing.State) {
	companions, err := android.Companions()
	if err != nil {
		s.Error("Failed to get Android companion devices: ", err)
	}
	for i, c := range companions {
		testing.ContextLogf(ctx, "Android companion %d: associate %q serial %q model %q",
			i, c.AssociatedHostname, c.SerialNumber, c.ModelName)
	}
}
