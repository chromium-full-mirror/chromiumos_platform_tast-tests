// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kioskmode

import "chromiumos/tast/common/policy"

// TestData is a data type that can be used by tests to parameterize them for
// Lacros launch.
type TestData struct {
	Policies []policy.Policy
	IsLacros bool
}
