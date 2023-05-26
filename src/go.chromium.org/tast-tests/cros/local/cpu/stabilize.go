// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cpu

import (
	"context"
	"time"
)

// WaitUntilStabilized waits for the stabilization of the CPU.
// Currently, this waits for two conditions, one is the CPU's cooldown,
// and the other is the CPU idle.
func WaitUntilStabilized(ctx context.Context, cdConfig CoolDownConfig) (time.Duration, error) {
	time, err := WaitUntilCoolDown(ctx, cdConfig)
	if err != nil {
		return 0, err
	}
	if err := WaitUntilIdle(ctx); err != nil {
		return 0, err
	}
	return time, nil
}
