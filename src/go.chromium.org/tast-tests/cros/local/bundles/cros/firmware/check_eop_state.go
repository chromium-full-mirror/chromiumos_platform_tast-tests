// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CheckEOPState,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Validates that the ME has been told by firmware that POST is done",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.IsIntelUarchEqualOrNewerThan(hwdep.IntelUarchs{IntelAtomOrderList: []hwdep.IntelAtomOrder{hwdep.Gracemont}, IntelBigCoreOrderList: []hwdep.IntelBigCoreOrder{hwdep.TigerLake}})),
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "get_boot_state",
				Val:               false,
				ExtraHardwareDeps: hwdep.D(hwdep.IsIntelUarchEqualOrNewerThan(hwdep.IntelUarchs{IntelBigCoreOrderList: []hwdep.IntelBigCoreOrder{hwdep.MeteorLake}})),
			},
			{
				Name:              "get_eop_state",
				Val:               true,
				ExtraHardwareDeps: hwdep.D(hwdep.IsIntelUarchOlderThan(hwdep.IntelUarchs{IntelBigCoreOrderList: []hwdep.IntelBigCoreOrder{hwdep.MeteorLake}})),
			},
		},
	})
}

func CheckEOPState(ctx context.Context, s *testing.State) {
	getEOPState := s.Param().(bool)
	const (
		meDevicePath = "/dev/mei0"
	)

	heciMKHI, err := uuid.Parse("{8e6a6715-9abc-4043-88ef-9e39c6f63e0f}")
	if err != nil {
		s.Fatal("Failed to parse: ", err)
	}

	s.Logf("Checking if ME device exists: %s", meDevicePath)
	if _, err := os.Stat(meDevicePath); err != nil {
		s.Fatal("Failed to find ME device, probably old kernel: ", err)
	}

	s.Log("Opening ME device on DUT")
	file, err := os.OpenFile(meDevicePath, os.O_RDWR, 0666)
	if err != nil {
		s.Fatal("Failed to open ME device on DUT: ", err)
	}
	defer file.Close()

	heciMKHIBinary, err := heciMKHI.MarshalBinary()
	if err != nil {
		s.Fatal("Failed to marshal heciMKHI: ", err)
	}
	heciMKHIToLe := heciMKHIToLe(heciMKHIBinary)

	s.Log("Connecting to MKHI by sending IOCTL_MEI_CONNECT_CLIENT command")
	if err := input.SendMEIConnectClientCmd(ctx, file, &heciMKHIToLe); err != nil {
		s.Fatal("Failed to send IOCTL_MEI_CONNECT_CLIENT command: ", err)
	}

	maxMsgLengthStr := hex.EncodeToString(heciMKHIToLe[:4])
	maxMsgLength, err := strconv.Atoi(maxMsgLengthStr)
	if err != nil {
		s.Fatal("Failed to convert maxMsgLengthStr to int: ", err)
	}

	protocolVersion := int(heciMKHIToLe[4])

	s.Logf("ME protocol version: %d, maxMsgLength: %d", protocolVersion, maxMsgLength)

	if protocolVersion < 2 {
		file.Close()
		s.Fatal("ME protocol too old. Not checking for EOP: ", err)
	}

	var (
		groupID, command        byte
		eopMask, expectedIBNCnt int
	)

	switch getEOPState {
	case false:
		groupID = 0xff
		command = 0x0a
		expectedIBNCnt = 12
		eopMask = 0x1
	case true:
		groupID = 0xff
		command = 0x1d
		expectedIBNCnt = 8
		eopMask = 0xff
	default:
		s.Fatal("Unknown eop command")
	}

	s.Log("Getting EOP info from ME with GEN_GET_BOOT_STATE command")
	packResult := []byte{groupID, command, 0, 0}
	if _, err := file.Write(packResult); err != nil {
		s.Fatal("Failed to write: ", err)
	}

	const maxBufSize = 1<<31 - 1
	var data [maxBufSize]byte
	inb, err := input.SysRead(ctx, file, &data)
	if err != nil {
		s.Fatal("Failed to read file: ", err)
	}

	if inb != uintptr(expectedIBNCnt) {
		s.Fatal("Unknown response by ME")
	}

	groupIDResp := data[0]
	if groupIDResp != groupID {
		s.Fatal("ME didn't respond to GEN_GET_BOOT_STATE")
	}

	commandPlus80 := data[1]
	result := data[3]

	eopStateStr := hex.EncodeToString(data[4:8])
	eopState, err := strconv.Atoi(eopStateStr)
	if err != nil {
		s.Fatal("Failed to convert eopStateStr to int: ", err)
	}
	s.Logf("Command Response: GroupID=%x, CmdPlus0x80=%x, Result=%x, EOPState=%x", groupIDResp, commandPlus80, result, eopState)

	if (groupIDResp != groupID) || (commandPlus80 != (command | 0x80)) {
		s.Fatal("ME didn't respond to Query EOP State")
	}
	if result == 0x8d {
		s.Fatal("ME didn't understand Query EOP State")
	}
	if result == 0x8e {
		s.Fatal("ME reported failure on Query EOP State")
	}
	if result != 0 {
		s.Fatal("ME gave unknown response to Query EOP State")
	}

	eopState = (eopState & eopMask)
	s.Log("EOP State: ", eopState == 0)
}

func heciMKHIToLe(b []byte) [16]byte {
	/*
		uuid is formed by:
			time_low: 4 bytes
			time_mid: 2 bytes
			time_hi_and_version: 2 bytes
			clock_seq_hi_and_res/clock_seq_low: 2 bytes
			node: 6 bytes

		This function converts time_low, time_mid and time_hi_and_version to
		little endian and remains clock_seq_hi_and_res/clock_seq_low and node
		in big endian.
	*/
	var bs bytes.Buffer

	timeLow := binary.BigEndian.Uint32(b[0:4])
	timeMid := binary.BigEndian.Uint16(b[4:6])
	timeHiAndVersion := binary.BigEndian.Uint16(b[6:8])
	others := binary.BigEndian.Uint64(b[8:])

	binary.Write(&bs, binary.LittleEndian, timeLow)
	binary.Write(&bs, binary.LittleEndian, timeMid)
	binary.Write(&bs, binary.LittleEndian, timeHiAndVersion)
	binary.Write(&bs, binary.BigEndian, others)

	return [16]byte(bs.Bytes())
}
