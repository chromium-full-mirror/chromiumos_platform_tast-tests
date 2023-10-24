// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

// GscProperties advertises various aspects of the GSC currently under test.
type GscProperties interface {
	// HasFpmcuUart indicates whether the GSC has a third UART connected to the FPMCU.
	HasFpmcuUart() bool
	// ExpectedDidVidValue returns the value expected when reading the TPM DID_VID register.
	ExpectedDidVidValue() []byte
}
