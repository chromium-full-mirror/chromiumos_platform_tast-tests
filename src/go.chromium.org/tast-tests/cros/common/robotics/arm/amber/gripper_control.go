// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package amber

import (
	"context"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// gripperControlCmdNo is the command number of the Gripper Control request message.
	gripperControlCmdNo uint16 = 9
	// gripperControlLength is the length of the Gripper Control request.
	// The Gripper Control request contains the following component:
	// - command number: 2 bytes
	// - response length: 2 bytes
	// - counter: 4 bytes
	// - action: 2 bytes
	// - intensity: 2 bytes
	// - gripper id: 1 byte
	gripperControlLength uint16 = 13
	// defaultIntensity is the default intensity used when it's out of valid range.
	defaultIntensity uint16 = 10
	minIntensity     uint16 = 1
	maxIntensity     uint16 = 20
	// defaultGripperID specifies the default gripper version to use.
	defaultGripperID uint8 = 0
)

// Action constants for gripper control.
const (
	ReleaseGripper uint16 = iota // 0: Release the gripper.
	HoldGripper                  // 1: Hold the gripper.
)

type gripperControlRequest []*armData

// newGripperControlRequest generates a new "Gripper Control" request with given counter, action and intensity.
// More details can be found at https://github.com/MrAsana/UDP-Protocol-API/wiki/Robotic-Arm-API-based-on-UDP-Protocol#1-gripper-control.
func newGripperControlRequest(counter uint32, action, intensity uint16) ([]*armData, error) {
	req := gripperControlRequest{}
	for _, dataPair := range []struct {
		data  []*armData
		value []any
	}{
		{
			data:  newCommonData(),
			value: []any{gripperControlCmdNo, gripperControlLength, counter},
		},
		{
			data:  newGripperActionData(),
			value: []any{action, intensity, defaultGripperID},
		},
	} {
		if err := setValuesToDataSlice(dataPair.data, dataPair.value); err != nil {
			return nil, errors.Wrap(err, "failed to set values to the arm data")
		}
		req = append(req, dataPair.data...)
	}
	return req, nil
}

// gripperControl controls the gripper to the given action and intensity.
func (a *Arm) gripperControl(ctx context.Context, action, intensity uint16) error {
	counter := a.getNextCounter()
	req, err := newGripperControlRequest(counter, action, intensity)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}
	if err := a.sendRequest(req); err != nil {
		return errors.Wrap(err, "failed to send request")
	}

	res := newGeneralResponse()
	// Read the response from the gripper control.
	// Note that the response is returned immediately after the gripper starts processing the request.
	if err := a.readResponse(generalResLength, res); err != nil {
		return errors.Wrap(err, "failed to read gripper control response")
	}
	status, err := newGeneralOperationStatus(res)
	if err != nil {
		return errors.Wrap(err, "failed to get operation status")
	}
	if status.Counter != counter {
		return errors.Errorf("unmatched counter from response: got %d, want %d", status.Counter, counter)
	}
	// Check if the response indicates success in receiving the request.
	// Note that Success here only means the request was received and does not confirm the action itself is complete.
	if status.Respond != responseSuccess {
		return errors.Errorf("unexpected arm response: got %v, want %v", status.Respond, responseSuccess)
	}

	// GoBigSleepLint: Wait for the gripper action to complete.
	// Note that the response success only indicates that the request was received and action has started,
	// not that it has been executed. Therefore, a fixed sleep duration is needed to wait for the operation to finish.
	if err := testing.Sleep(ctx, defaultGripperDuration); err != nil {
		return errors.Wrap(err, "failed to wait for operation executed")
	}
	return nil
}

// ReleaseGripper releases the gripper with the default release intensity.
func (a *Arm) ReleaseGripper(ctx context.Context) error {
	return a.gripperControl(ctx, ReleaseGripper, defaultIntensity)
}

// HoldGripper holds the gripper with the specified intensity.
// This function checks whether the provided intensity is within the valid range
// [1, 20], and returns an error if it's out of range.
func (a *Arm) HoldGripper(ctx context.Context, intensity uint16) error {
	// Check if the intensity is within the valid range.
	if intensity < minIntensity || intensity > maxIntensity {
		return errors.Errorf("holding intensity %d is out of valid range [%d, %d]", intensity, minIntensity, maxIntensity)
	}
	return a.gripperControl(ctx, HoldGripper, intensity)
}
