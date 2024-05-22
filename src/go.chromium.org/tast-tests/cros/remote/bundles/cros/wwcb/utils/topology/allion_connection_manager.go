// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package topology contains tools to interact with PASIT topology components.
package topology

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
)

// allionConnectionManager is a simple ConnectionManager for communicating with allion fixtures/switches.
type allionConnectionManager struct {
	id string
}

func (a *allionConnectionManager) EnabledState(ctx context.Context, enabled bool) error {
	status := "on"
	if !enabled {
		status = "off"
	}

	return utils.ControlFixture(ctx, a.id, status)
}
