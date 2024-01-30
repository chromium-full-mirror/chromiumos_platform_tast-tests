// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crastestclient provides functions to interact with the
// cras_test_client CLI.
// Source: adhd::cras/src/tools/cras_test_client/cras_test_client.c
//
// Note: The cras_test_client CLI, while still present on the DUT, has been
// deprecated in favor of the newer cras_tests CLI. The utilities in this
// package are only meant to assist in the migration to cras_tests and should
// not be used long-term.
package crastestclient
