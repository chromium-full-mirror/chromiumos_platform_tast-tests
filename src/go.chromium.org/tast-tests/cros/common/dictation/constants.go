// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dictation

// Event constants for Dictation Support.
// Please refer to https://storage.googleapis.com/chromeos-mgmt-public-extension/dictation_support/index.html.
// All events are used in the dictation support website.
const (
	EventInit          = "init"
	EventRequestDevice = "requestDevice"
	EventGetDevices    = "getDevices"
	EventSetEventMode  = "setEventMode"
	EventGetEventMode  = "getEventMode"
	EventSetSimpleLED  = "setSimpleLedState"
	EventSetLED        = "setLed"
)

// ReadyPosition is a constant for "Ready Position".
const ReadyPosition = "Ready Position"

// Button constants for Dictation Support.
const (
	// ButtonRecord is the button event key "RECORD".
	ButtonRecord = "RECORD"
	// ButtonStop is the button event key "STOP".
	ButtonStop = "STOP"
	// ButtonPlay is the button event key "PLAY".
	ButtonPlay = "PLAY"
	// ButtonRewind is the button event key "REWIND".
	ButtonRewind = "REWIND"
	// ButtonForward is the button event key "FORWARD".
	ButtonForward = "FORWARD"
	// ButtonTabForward is the button event key "TAB_FORWARD".
	ButtonTabForward = "TAB_FORWARD"
	// ButtonTabBackward is the button event key "TAB_BACKWARD".
	ButtonTabBackward = "TAB_BACKWARD"
	// ButtonEnterSelect is the button event key "ENTER_SELECT".
	ButtonEnterSelect = "ENTER_SELECT"
	// ButtonInstruction is the button event key "INSTR".
	ButtonInstruction = "INSTR"
	// ButtonTranscribe is the button event key "TRANSCRIBE".
	ButtonTranscribe = "TRANSCRIBE"
	// ButtonInsertOverwrite is the button event key "INS_OVR".
	ButtonInsertOverwrite = "INS_OVR"
	// ButtonEndOfLetterPriority is the button event key "EOL_PRIO".
	ButtonEndOfLetterPriority = "EOL_PRIO"
	// ButtonCommand is the button event key "COMMAND".
	ButtonCommand = "COMMAND"
)

const (
	// MotionEventPickedUp is the motion event key "PICKED_UP".
	MotionEventPickedUp = "PICKED_UP"
	// MotionEventLaidDown is the motion event key "LAYED_DOWN".
	MotionEventLaidDown = "LAYED_DOWN"
)

const (
	// FunctionF1A is the programmable function key "F1_A".
	FunctionF1A = "F1_A"
	// FunctionF2B is the programmable function key "F2_B".
	FunctionF2B = "F2_B"
	// FunctionF3C is the programmable function key "F3_C".
	FunctionF3C = "F3_C"
	// FunctionF4D is the programmable function key "F4_D".
	FunctionF4D = "F4_D"
)

// EventMode represents the event mode.
type EventMode string

const (
	// EventModeHid is the HID event mode.
	EventModeHid EventMode = "Hid"
	// EventModeKeyboard is the Keyboard event mode.
	EventModeKeyboard EventMode = "Keyboard"
	//EventModeBrowser is the Browser event mode.
	EventModeBrowser EventMode = "Browser"
	// EventModeWindowsSR is the WindowsSR event mode.
	EventModeWindowsSR EventMode = "WindowsSR"
	// EventModeDragonForMac is the DragonForMac event mode.
	EventModeDragonForMac EventMode = "DragonForMac"
	// EventModeDragonForWindows is the DragonForWindows event mode.
	EventModeDragonForWindows EventMode = "DragonForWindows"
)

// SimpleLEDState represents the simple LED state.
type SimpleLEDState string

const (
	// SimpleLEDStateOff is the LED state "Off".
	SimpleLEDStateOff SimpleLEDState = "Off"
	// SimpleLEDStateRecordInsert is the LED state "RecordInsert".
	SimpleLEDStateRecordInsert SimpleLEDState = "RecordInsert"
	// SimpleLEDStateRecordOverwrite is the LED state "RecordOverwrite".
	SimpleLEDStateRecordOverwrite SimpleLEDState = "RecordOverwrite"
	// SimpleLEDStateStandbyInsert is the LED state "StandbyInsert".
	SimpleLEDStateStandbyInsert SimpleLEDState = "StandbyInsert"
	// SimpleLEDStateStandbyOverwrite is the LED state "StandbyOverwrite".
	SimpleLEDStateStandbyOverwrite SimpleLEDState = "StandbyOverwrite"
)

// LEDStateToNumber is a map from SimpleLEDState to its number.
var LEDStateToNumber = map[SimpleLEDState]int{
	SimpleLEDStateOff:              0,
	SimpleLEDStateRecordInsert:     1,
	SimpleLEDStateRecordOverwrite:  2,
	SimpleLEDStateStandbyInsert:    3,
	SimpleLEDStateStandbyOverwrite: 4,
}

// LEDIndex represents the index of the LED state.
type LEDIndex string

const (
	// LEDRecordGreen is the index "RECORD_LED_GREEN".
	LEDRecordGreen LEDIndex = "RECORD_LED_GREEN"
	// LEDRecordRed is the index "RECORD_LED_RED".
	LEDRecordRed LEDIndex = "RECORD_LED_RED"
	// LEDInstrctionGreen is the index "INSTRUCTION_LED_GREEN".
	LEDInstrctionGreen LEDIndex = "INSTRUCTION_LED_GREEN"
	// LEDInstrctionRed is the index "INSTRUCTION_LED_RED".
	LEDInstrctionRed LEDIndex = "INSTRUCTION_LED_RED"
	// LEDInsOwrButtonGreen is the index "INS_OWR_BUTTON_LED_GREEN".
	LEDInsOwrButtonGreen LEDIndex = "INS_OWR_BUTTON_LED_GREEN"
	// LEDInsOwrButtonRed is the index "INS_OWR_BUTTON_LED_RED".
	LEDInsOwrButtonRed LEDIndex = "INS_OWR_BUTTON_LED_RED"
	// LEDF1Button is the index "F1_BUTTON_LED".
	LEDF1Button LEDIndex = "F1_BUTTON_LED"
	// LEDF2Button is the index "F2_BUTTON_LED".
	LEDF2Button LEDIndex = "F2_BUTTON_LED"
	// LEDF3Button is the index "F3_BUTTON_LED".
	LEDF3Button LEDIndex = "F3_BUTTON_LED"
	// LEDF4Button is the index "F4_BUTTON_LED".
	LEDF4Button LEDIndex = "F4_BUTTON_LED"
)

// LEDIndexToNumber is a map from LEDIndex to its number.
var LEDIndexToNumber = map[LEDIndex]int{
	LEDRecordGreen:       0,
	LEDRecordRed:         1,
	LEDInstrctionGreen:   2,
	LEDInstrctionRed:     3,
	LEDInsOwrButtonGreen: 4,
	LEDInsOwrButtonRed:   5,
	LEDF1Button:          6,
	LEDF2Button:          7,
	LEDF3Button:          8,
	LEDF4Button:          9,
}

// LEDMode represents the mode of the LED state.
type LEDMode string

const (
	// LEDModeOff is the LED mode "Off".
	LEDModeOff LEDMode = "Off"
	// LEDModeBlinkSlow is the LED mode "BlinkSlow".
	LEDModeBlinkSlow LEDMode = "BlinkSlow"
	// LEDModeBlinkFast is the LED mode "BlinkFast".
	LEDModeBlinkFast LEDMode = "BlinkFast"
	// LEDModeOn is the LED mode "On".
	LEDModeOn LEDMode = "On"
)

// LEDModeToNumber is a map from LEDMode to its number.
var LEDModeToNumber = map[LEDMode]int{
	LEDModeOff:       0,
	LEDModeBlinkSlow: 1,
	LEDModeBlinkFast: 2,
	LEDModeOn:        3,
}

// LEDState represents the LED state.
type LEDState struct {
	Index LEDIndex
	Mode  LEDMode
}
