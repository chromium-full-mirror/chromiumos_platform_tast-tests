// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dictation

import (
	"time"
)

// TestParams is a struct that holds the parameters for the dictation test.
type TestParams struct {
	DeviceName     string
	MotionData     string
	MotionDataMap  map[string]int
	ButtonTestList []string
}

// PhilipsTestParams holds the parameters for the Philips dictation test.
var PhilipsTestParams = TestParams{
	DeviceName:     "Philips Speech Processing SpeechMike III",
	MotionData:     PhilipsMotionData,
	MotionDataMap:  philipsMotionDataMap,
	ButtonTestList: philipsButtonTestList,
}

const (
	// PhilipsMotionData is the motion data that controls the robotic arm to
	// operate the Philips dictation device.
	// The file should contain 12 positions in series:
	// 0 = Ready position
	// 1 = End-of-letter/Priority
	// 2 = Instruction
	// 3 = Insert/Overwrite
	// 4 = Record
	// 5 = Rewind
	// 6 = Forward
	// 7 = Play
	// 8 = F1_A
	// 9 = F2_B
	// 10 = F3_C
	// 11 = F4_D
	PhilipsMotionData = "dictation/philips_motion_data.csv"
	// MotionDuration is the duration of each motion.
	MotionDuration = 5 * time.Second
)

var (
	philipsMotionDataMap = map[string]int{
		ReadyPosition:             0,
		ButtonEndOfLetterPriority: 1,
		ButtonInstruction:         2,
		ButtonInsertOverwrite:     3,
		ButtonRecord:              4,
		ButtonRewind:              5,
		ButtonForward:             6,
		ButtonPlay:                7,
		FunctionF1A:               8,
		FunctionF2B:               9,
		FunctionF3C:               10,
		FunctionF4D:               11,
	}

	// ReadyPositionIndex is the index of the ready position in the motion data.
	ReadyPositionIndex = philipsMotionDataMap[ReadyPosition]

	philipsButtonTestList = []string{
		ButtonRecord,
		ButtonPlay,
		ButtonRewind,
		ButtonForward,
		ButtonInstruction,
		ButtonInsertOverwrite,
		ButtonEndOfLetterPriority,
		// TODO: The robotic arm needs to record the motion data of pressing the following buttons.
		// ButtonCommand,
		FunctionF1A,
		FunctionF2B,
		FunctionF3C,
		FunctionF4D,
	}
)
