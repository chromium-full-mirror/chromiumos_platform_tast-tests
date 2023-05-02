// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package policyannotations contains shared code for policy annotations tests
package policyannotations

import (
	"go.chromium.org/tast-tests/cros/common/policy"
)

// AnnotationTestParams is used for annotation test scenarios
type AnnotationTestParams struct {
	Name                  string          // name is the subtest name.
	Policies              []policy.Policy // policies to check
	AnnotationLogExpected bool
}
