// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package iioservice contains some shared constants among tast tests.
package iioservice

const (
	// OnErrorOccurredText is the log printed by `iioservice_` executables
	// when error occurs.
	OnErrorOccurredText = "OnErrorOccurred:"

	// SucceedReadingText is the log printed by `iioservice_` executables
	// when all reads are completed.
	SucceedReadingText = "Number of success reads"
)
