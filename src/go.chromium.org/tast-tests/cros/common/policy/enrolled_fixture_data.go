// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

// EnrolledFixtureData is returned by the Enrolled fixture.
type EnrolledFixtureData struct {
	Errors           []string
	FakeDMSDirectory string
}
