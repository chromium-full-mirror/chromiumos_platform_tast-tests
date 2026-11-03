// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import "github.com/google/go-tpm/tpm2"

// UndefineNVSpace undefines nvIndex using the specified authHandle (e.g.
// TPMRHOwner or TPMRHPlatform).
func (t *TpmHandle) UndefineNVSpace(authHandle, nvIndex tpm2.TPMHandle) error {
	nvUndefineSpace := tpm2.NVUndefineSpace{
		AuthHandle: authHandle,
		NVIndex: tpm2.NamedHandle{
			Handle: nvIndex,
			Name:   tpm2.TPM2BName{Buffer: []byte{0x00, 0x0b, 0x00, 0x00}},
		},
	}
	_, err := nvUndefineSpace.Execute(t)
	return err
}
