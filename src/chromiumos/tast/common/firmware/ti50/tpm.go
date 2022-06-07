// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

// Constants representing TPM registers, for use with `OpenTitanToolCommand()`.
const (
	// TpmRegAccess is a TPM register
	TpmRegAccess = "ACCESS"

	// TpmRegIntEnable is a TPM register
	TpmRegIntEnable = "INT_ENABLE"

	// TpmRegIntVector is a TPM register
	TpmRegIntVector = "INT_VECTOR"

	// TpmRegIntStatus is a TPM register
	TpmRegIntStatus = "INT_STATUS"

	// TpmRegIntfCapability is a TPM register
	TpmRegIntfCapability = "INTF_CAPABILITY"

	// TpmRegSts is a TPM register
	TpmRegSts = "STS"

	// TpmRegDataFifo is a TPM register
	TpmRegDataFifo = "DATA_FIFO"

	// TpmRegInterfaceID is a TPM register
	TpmRegInterfaceID = "INTERFACE_ID"

	// TpmRegXdataFifo is a TPM register
	TpmRegXdataFifo = "XDATA_FIFO"

	// TpmRegDidVid is a TPM register
	TpmRegDidVid = "DID_VID"

	// TpmRegRid is a TPM register
	TpmRegRid = "RID"
)
