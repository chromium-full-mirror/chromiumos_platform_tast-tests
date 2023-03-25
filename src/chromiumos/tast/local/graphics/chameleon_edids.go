// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"encoding/hex"
	"strings"

	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/errors"
)

// EdidType describes the types like DP or HDMI for Edids
type EdidType int

const (
	// EdidDP type for Edids
	EdidDP EdidType = iota
	// EdidHDMI type for Edids
	EdidHDMI
)

// EdidID describes ID for EDID record
type EdidID int

// Edid structure is used to describe the structure of Edid
type Edid struct {
	ID          EdidID   // Unique identifier for the EDID
	Name        string   // Name of the display
	Width       int      // Display width in pixels
	Height      int      // Display height in pixels
	RefreshRate int      // Display refresh rate in Hz
	HDR         bool     // True if the display supports HDR, False otherwise
	Edid        string   // Raw EDID string data
	EdidBytes   []byte   // Raw EDID data as a byte slice
	Type        EdidType // Enum representing the type of EDID (e.g., EdidDP, EdidHDMI)
}

// Enumerate all monitor constants
const (
	HPZR2440w = iota
	HPSpectre32
	HPM27fd
)

func addEdid(edids map[EdidID]Edid, edid Edid) error {
	edidString := removeWhiteSpacesFromEdid(edid.Edid)

	// Decode the hex-encoded EDID string into bytes.
	edidBytes, err := hex.DecodeString(edidString)
	if err != nil {
		return errors.Wrap(err, "failed to decode EDID string into bytes")
	}

	// Pad the EDID bytes to 256 bytes.
	if len(edidBytes) < 256 {
		padding := make([]byte, 256-len(edidBytes))
		edidBytes = append(edidBytes, padding...)
	}

	edid.EdidBytes = edidBytes
	edids[edid.ID] = edid

	return nil
}

// ChameleonGetEdids will return a map of all EDIDs
func ChameleonGetEdids() map[EdidID]Edid {
	Edids := make(map[EdidID]Edid)

	edid1 := Edid{
		ID:          HPZR2440w,
		Name:        "HP ZR2440w",
		Width:       1920,
		Height:      1080,
		RefreshRate: 60,
		HDR:         false,
		Edid: `0ffffffffffff0022f0562901010101
		0b170103803420782afc81a4554d9d25
		125054210800d1c081c0814081809500
		a940b3000101283c80a070b023403020
		360006442100001a000000fd00183c18
		5011000a202020202020000000fc0048
		50205a5232343430770a2020000000ff
		00434e34333131315042560a2020015c
		02031ff14c901f051404130302070612
		0165030c001000230907078301000002
		3a801871382d40582c45000644210000
		1e023a80d072382d40102c4580064421
		00001e011d007251d01e206e28550006
		442100001e011d00bc52d01e20b82855
		4006442100001e8c0ad08a20e02d1010
		3e9600064421000018000000000000c1`,
		Type: EdidHDMI,
	}

	edid2 := Edid{
		ID:          HPSpectre32,
		Name:        "HP Spectre 32",
		Width:       3840,
		Height:      2160,
		RefreshRate: 60,
		HDR:         false,
		Edid: `0FFFFFFFFFFFF0022F01A3200000000
		2E180104B54728783A87D5A8554D9F25
		0E5054210800D1C0A9C081C0D100B300
		9500A94081804DD000A0F0703E803020
		3500C48F2100001A000000FD00183C1E
		873C000A202020202020000000FC0048
		502053706563747265203332000000FF
		00434E43393430303030310A2020018F
		020318F14B101F041303120211010514
		2309070783010000A36600A0F0701F80
		30203500C48F2100001A565E00A0A0A0
		295030203500C48F2100001AEF5100A0
		F070198030203500C48F2100001AB339
		00A080381F4030203A00C48F2100001A
		283C80A070B0234030203600C48F2100
		001A00000000000000000000000000C4`,
		Type: EdidDP,
	}

	edid3 := Edid{
		ID:          HPM27fd,
		Name:        "HP M27fd",
		Width:       1920,
		Height:      1080,
		RefreshRate: 60,
		HDR:         false,
		Edid: `00ffffffffffff00220e123701010101
		091f0103803d24782a4815a756529c27
		0f5054a10800d1c0a9c081c0b3009500
		810081800101023a801871382d40582c
		450055502100001e000000fd00304b1e
		5612000a202020202020000000fc0048
		50204d32376664204648440a000000ff
		0033434d3130393030475920202001bc
		02032fb149901f041303120211016703
		0c001000002467d85dc4012480016d1a
		00000201304bed0000000000e2006b02
		3a80d072382d40102c45805550210000
		1e011d007251d01e206e285500555021
		00001e011d00bc52d01e20b828554055
		502100001e2a4480a070382740302035`,
		Type: EdidHDMI,
	}

	addEdid(Edids, edid1)
	addEdid(Edids, edid2)
	addEdid(Edids, edid3)

	return Edids
}

func removeWhiteSpacesFromEdid(edidString string) string {
	// Remove all whitespace characters from the EDID string.
	edidString = strings.ReplaceAll(edidString, " ", "")
	edidString = strings.ReplaceAll(edidString, "\n", "")
	edidString = strings.ReplaceAll(edidString, "\t", "")

	return edidString
}

// ChameleonSetEdid sets EDIDs for the Chameleon
func ChameleonSetEdid(ctx context.Context, cham chameleon.Chameleond, port chameleon.PortID) error {
	edids := ChameleonGetEdids()
	//Todo(kenil): monitorname we can pass dynamic
	edid, found := edids[HPM27fd]

	if !found {
		return errors.New("failed to retrive EDID by ID")
	}

	edidID, err := cham.CreateEdid(ctx, edid.EdidBytes)
	if err != nil {
		return errors.Wrapf(err, "failed to create internal Edids for %s monitor", edid.Name)
	}

	return cham.ApplyEdid(ctx, port, edidID)
}
