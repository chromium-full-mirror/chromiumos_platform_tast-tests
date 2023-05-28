// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import "time"

// TimeParams defines the time interval used in power.NewRecorder
// and the total sleep duration in each power test.
type TimeParams struct {
	Interval time.Duration
	Total    time.Duration
}
