// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import "time"

const (
	// CooldownTimeout is the maximum time allowed for cooldown.
	CooldownTimeout = 15 * time.Minute
)
