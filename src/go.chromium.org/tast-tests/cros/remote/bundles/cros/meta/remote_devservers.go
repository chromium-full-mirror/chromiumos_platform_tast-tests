// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteDevservers,
		Desc:         "Example of using CloudStorage.Devservers for remote tests",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
	})
}

func RemoteDevservers(ctx context.Context, s *testing.State) {
	cs := s.CloudStorage()
	devservers := cs.Devservers()
	s.Log("Devservers: ", devservers)
}
