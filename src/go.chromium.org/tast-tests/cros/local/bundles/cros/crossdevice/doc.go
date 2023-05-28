// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crossdevice contains a temporary local Tast test that exercises a ChromeOS Cross Device onboarding fixture failure.
// When this test fails, the other tests in the cross device suite will be able to pass by re-running the fixture.
// TODO(b/230401333): Remove this package after root cause of onboarding failure is fixed.
package crossdevice
