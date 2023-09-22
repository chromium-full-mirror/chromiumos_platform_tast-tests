// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

// pdControlMsgType specifies the PD control message value as a string
type pdControlMsgType string

// TCPMv2 flags bitmask values.  Source of truth for these values comes from the EC source [1].
// [1] https://source.chromium.org/chromiumos/chromiumos/codesearch/+/HEAD:src/platform/ec/include/usb_pd.h
const (
	PartnerDualRolePower = 1 << 1
	PartnerDualRoleData  = 1 << 2
	VCONNEnabled         = 1 << 12
)

// PD control message types
const (
	PDCtrlReserved pdControlMsgType = "Reserved"
	PDCtrlGoodCRC  pdControlMsgType = "GoodCRC"
	PDCtrlAccept   pdControlMsgType = "Accept"
	PDCtrlReject   pdControlMsgType = "Reject"
)

const (
	// PdControlMsgMask is a bitmask to extract the message type from PD control messages.
	PdControlMsgMask uint64 = 0x1f
)

// PDControlMsg map of the control message field integer to PD control message type
// Source of values: USB-PD Spec Rev 3.1, Ver 1.6, Table 6-5
var pdControlMsg = map[int]pdControlMsgType{
	0x00: PDCtrlReserved,
	0x01: PDCtrlGoodCRC,
	0x03: PDCtrlAccept,
	0x04: PDCtrlReject,
}
