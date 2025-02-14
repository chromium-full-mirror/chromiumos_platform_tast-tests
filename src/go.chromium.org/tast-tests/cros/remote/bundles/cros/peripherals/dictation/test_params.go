// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dictation

import (
	"time"

	dictationcommon "go.chromium.org/tast-tests/cros/common/dictation"
)

// TestParams is a struct that holds the parameters for the dictation test.
type TestParams struct {
	DeviceName       string
	MotionData       string
	MotionDataMap    map[string]int
	ButtonTestList   []string
	KeyboardTestList []string
}

// PhilipsTestParams holds the parameters for the Philips dictation test.
var PhilipsTestParams = TestParams{
	DeviceName:       "Philips Speech Processing SpeechMike III",
	MotionData:       PhilipsMotionData,
	MotionDataMap:    philipsMotionDataMap,
	ButtonTestList:   philipsButtonTestList,
	KeyboardTestList: philipsKeyboardTestList,
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
		dictationcommon.ReadyPosition:             0,
		dictationcommon.ButtonEndOfLetterPriority: 1,
		dictationcommon.ButtonInstruction:         2,
		dictationcommon.ButtonInsertOverwrite:     3,
		dictationcommon.ButtonRecord:              4,
		dictationcommon.ButtonRewind:              5,
		dictationcommon.ButtonForward:             6,
		dictationcommon.ButtonPlay:                7,
		dictationcommon.FunctionF1A:               8,
		dictationcommon.FunctionF2B:               9,
		dictationcommon.FunctionF3C:               10,
		dictationcommon.FunctionF4D:               11,
	}

	// ReadyPositionIndex is the index of the ready position in the motion data.
	ReadyPositionIndex = philipsMotionDataMap[dictationcommon.ReadyPosition]

	philipsButtonTestList = []string{
		dictationcommon.ButtonRecord,
		dictationcommon.ButtonPlay,
		dictationcommon.ButtonRewind,
		dictationcommon.ButtonForward,
		dictationcommon.ButtonInstruction,
		dictationcommon.ButtonInsertOverwrite,
		dictationcommon.ButtonEndOfLetterPriority,
		// TODO: The robotic arm needs to record the motion data of pressing the following buttons.
		// dictationcommon.ButtonCommand,
		dictationcommon.FunctionF1A,
		dictationcommon.FunctionF2B,
		dictationcommon.FunctionF3C,
		dictationcommon.FunctionF4D,
	}

	philipsKeyboardTestList = []string{
		dictationcommon.ButtonEndOfLetterPriority,
		dictationcommon.ButtonInsertOverwrite,
		dictationcommon.ButtonForward,
		dictationcommon.ButtonInstruction,
		dictationcommon.FunctionF1A,
		dictationcommon.FunctionF2B,
	}
)
