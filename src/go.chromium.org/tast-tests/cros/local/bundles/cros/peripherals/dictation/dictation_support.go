// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dictation provides utility functions for running dictation tast tests.
package dictation

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dictationSupportURL = "https://storage.googleapis.com/chromeos-mgmt-public-extension/dictation_support/index.html"

	defaultTimeout = 10 * time.Second
)

var (
	// deviceTypeRegex is a regular expression to extract the value of the "type" field.
	deviceTypeRegex = regexp.MustCompile(`"type":"([^"]+)"`)
	// eventModeRegex is a regular expression to match "eventMode: [value]".
	eventModeRegex = regexp.MustCompile(`eventMode:\s*(\w+)`)
	// simpleLedStateRegex is a regular expression to match the "simpleLedState: [value]".
	simpleLedStateRegex = regexp.MustCompile(`simpleLedState:\s*(\d)`)
	// indexAndModeRegex is a regular expression to match the "index: [index] mode: [mode]".
	indexAndModeRegex = regexp.MustCompile(`index:\s*(\d+)\s*mode:\s*(\d+)`)
)

// Support holds the resources required for running dictation tests.
type Support struct {
	conn *chrome.Conn
	ui   *uiauto.Context
	// eventMessages contains all event messages after the dictation support page is opened.
	eventMessages string
	// lastEventMessage represents the latest event message currently received.
	lastEventMessage string
}

// NewSupport creates a new Support instance.
func NewSupport(ctx context.Context, cr *chrome.Chrome) (*Support, error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Test API connection")
	}
	conn, err := cr.NewConn(ctx, dictationSupportURL)
	if err != nil {
		return nil, err
	}

	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the page loaded")
	}

	ui := uiauto.New(tconn)

	return &Support{conn: conn, ui: ui}, nil
}

// Close closes the dictation support page.
func (s *Support) Close(ctx context.Context) error {
	defer s.conn.Close()
	if err := s.conn.CloseTarget(ctx); err != nil {
		return err
	}
	return nil
}

// ConnectToDevice inits and connects to the given device.
func (s *Support) ConnectToDevice(deviceName string) action.Action {
	ui := s.ui
	initButton := nodewith.Name(eventInit).Role(role.Button)
	requestDeviceButton := nodewith.Name(eventRequestDevice).Role(role.Button)
	deviceNameItem := nodewith.Name(deviceName).Role(role.GridCell)
	connectButton := nodewith.Name("Connect").Role(role.Button)
	return uiauto.NamedCombine(fmt.Sprintf("connect to device %q", deviceName),
		ui.DoDefault(initButton),
		s.WaitNewEvent(eventInit, defaultTimeout),
		ui.DoDefaultUntil(requestDeviceButton, ui.WithTimeout(5*time.Second).WaitUntilExists(deviceNameItem)),
		// Using DoDefault here may result in clicking on the wrong node, so use LeftClick instead.
		ui.LeftClickUntil(deviceNameItem, ui.WithTimeout(5*time.Second).WaitUntilExists(deviceNameItem.Focused())),
		ui.DoDefaultUntil(connectButton, ui.WithTimeout(5*time.Second).WaitUntilGone(connectButton)),
		s.WaitNewEvent(eventRequestDevice, defaultTimeout),
	)
}

// Devices returns the dictation devices.
func (s *Support) Devices(ctx context.Context) ([]string, error) {
	getDevicesButton := nodewith.Name(eventGetDevices).Role(role.Button).First()
	if err := uiauto.NamedCombine("get devices",
		s.ui.DoDefault(getDevicesButton),
		s.WaitNewEvent(eventGetDevices, defaultTimeout),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to get devices")
	}

	// Find all matches for the "type" field.
	matches := deviceTypeRegex.FindAllStringSubmatch(s.lastEventMessage, -1)
	var devices []string

	// Iterate through all matches and add the type values to devices.
	for _, match := range matches {
		devices = append(devices, match[1])
	}

	return devices, nil
}

// WaitNewEvent waits for a new event to occur and saves the last event message.
func (s *Support) WaitNewEvent(expectedEvent string, timeout time.Duration) action.Action {
	return func(ctx context.Context) error {
		ui := s.ui
		rootWebArea := nodewith.Name("Dictation support demo").Role(role.RootWebArea)
		initializedTextBox := nodewith.NameContaining("init() done").Role(role.StaticText).Ancestor(rootWebArea)
		return testing.Poll(ctx, func(ctx context.Context) error {
			eventInfo, err := ui.Info(ctx, initializedTextBox)
			if err != nil {
				return errors.Wrap(err, "failed to get event info")
			}
			eventMessages := eventInfo.Name
			if len(eventMessages) > len(s.eventMessages) {
				s.lastEventMessage = strings.TrimSpace(eventMessages[len(s.eventMessages):])
				s.eventMessages = eventMessages

				if strings.Contains(s.lastEventMessage, expectedEvent) {
					return nil
				}
			}
			return errors.Errorf("failed to wait for the event %q", expectedEvent)
		}, &testing.PollOptions{Timeout: timeout, Interval: time.Second})
	}
}

// EventMode returns the current event mode.
func (s *Support) EventMode(ctx context.Context) (EventMode, error) {
	ui := s.ui
	getEventModeButton := nodewith.Name(eventGetEventMode).Role(role.Button).First()
	if err := uiauto.NamedCombine("get event mode",
		ui.DoDefault(getEventModeButton),
		s.WaitNewEvent(eventGetEventMode, defaultTimeout),
	)(ctx); err != nil {
		return "", errors.Wrap(err, "failed to get event mode")
	}

	matches := eventModeRegex.FindStringSubmatch(s.lastEventMessage)
	if len(matches) < 2 {
		return "", errors.New("eventMode not found in the last event message")
	}

	return EventMode(matches[1]), nil
}

// SetEventMode sets the event mode to the given event mode.
func (s *Support) SetEventMode(eventMode EventMode) action.Action {
	ui := s.ui
	setEventModeButton := nodewith.Name(eventSetEventMode).Role(role.Button).First()
	eventModeItem := nodewith.NameContaining(string(eventMode)).Role(role.MenuListOption).First()
	return uiauto.NamedCombine(fmt.Sprintf("set event mode to %q", eventMode),
		ui.DoDefault(eventModeItem),
		ui.DoDefault(setEventModeButton),
		s.WaitNewEvent(eventSetEventMode, defaultTimeout),
	)
}

// SetSimpleLEDState sets the simple LED state to the given state.
func (s *Support) SetSimpleLEDState(state SimpleLEDState) action.Action {
	ui := s.ui
	simpleLEDStateItem := nodewith.NameContaining(string(state)).Role(role.MenuListOption).First()
	setSimpleLEDStateButton := nodewith.Name(eventSetSimpleLED).Role(role.Button).First()
	verifyEventMessage := func(ctx context.Context) error {
		expectedNumber := ledStateToNumber[state]

		match := simpleLedStateRegex.FindStringSubmatch(s.lastEventMessage)
		if len(match) < 2 {
			return errors.New("simpleLedState not found in the last event message: " + s.lastEventMessage)
		}
		ledStateNumStr := match[1]
		ledStateNum, err := strconv.Atoi(ledStateNumStr)
		if err != nil {
			return errors.Wrap(err, "failed to convert led state to number")
		}
		if ledStateNum != expectedNumber {
			return errors.Errorf("unexpected led state, got: %v, want: %v", ledStateNum, expectedNumber)
		}
		return nil
	}
	return uiauto.NamedCombine(fmt.Sprintf("set simple led state to %q", state),
		ui.DoDefaultUntil(simpleLEDStateItem, s.waitUntilSelected(simpleLEDStateItem)),
		ui.DoDefault(setSimpleLEDStateButton),
		s.WaitNewEvent(eventSetSimpleLED, defaultTimeout),
		verifyEventMessage,
	)
}

// SetLEDState sets the LED state with given index and mode.
func (s *Support) SetLEDState(index LEDIndex, mode LEDMode) action.Action {
	ui := s.ui
	indexOption := nodewith.NameContaining(string(index)).Role(role.MenuListOption)
	speechMikeGroup := nodewith.NameContaining("SpeechMike").Role(role.Group)
	// The third menu list in the SpeechMike group is the mode menu.
	modeMenuList := nodewith.Role(role.MenuListPopup).Ancestor(speechMikeGroup).Nth(3)
	modeOption := nodewith.NameContaining(string(mode)).Role(role.MenuListOption).Ancestor(modeMenuList).First()
	setLEDButton := nodewith.Name(eventSetLED).Role(role.Button).First()
	verifyEventMessage := func(ctx context.Context) error {
		expectedLEDIndex := ledIndexToNumber[index]
		expectedLEDMode := ledModeToNumber[mode]

		matches := indexAndModeRegex.FindStringSubmatch(s.lastEventMessage)
		if len(matches) < 3 {
			return errors.New("setLed() not found in the last event message: " + s.lastEventMessage)
		}

		ledIndexNumStr := matches[1]
		ledModeNumStr := matches[2]
		ledIndexNum, err := strconv.Atoi(ledIndexNumStr)
		if err != nil {
			return errors.Wrap(err, "failed to convert LED index to number")
		}
		ledModeNum, err := strconv.Atoi(ledModeNumStr)
		if err != nil {
			return errors.Wrap(err, "failed to convert LED mode to number")
		}
		if ledIndexNum != expectedLEDIndex {
			return errors.Errorf("unexpected LED index, got: %v, want: %v", ledIndexNum, expectedLEDIndex)
		}
		if ledModeNum != expectedLEDMode {
			return errors.Errorf("unexpected LED mode, got: %v, want: %v", ledModeNum, expectedLEDMode)
		}
		return nil
	}
	return uiauto.NamedCombine(fmt.Sprintf("set led state to index %q, mode %q", index, mode),
		uiauto.IfFailThen(s.selected(indexOption),
			ui.DoDefaultUntil(indexOption, s.waitUntilSelected(indexOption))),
		uiauto.IfFailThen(s.selected(modeOption),
			ui.DoDefaultUntil(modeOption, s.waitUntilSelected(modeOption))),
		ui.DoDefault(setLEDButton),
		s.WaitNewEvent(eventSetLED, defaultTimeout),
		verifyEventMessage,
	)
}

func (s *Support) selected(finder *nodewith.Finder) uiauto.Action {
	return func(ctx context.Context) error {
		nodeInfo, err := s.ui.Info(ctx, finder)
		if err != nil {
			return err
		}
		if !nodeInfo.Selected {
			return errors.Wrapf(err, "%q selected state is not true", nodeInfo.Name)
		}
		return nil
	}
}

func (s *Support) waitUntilSelected(finder *nodewith.Finder) uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			return s.selected(finder)(ctx)
		}, &testing.PollOptions{Timeout: 2 * time.Second})
	}
}
