// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package amber

import (
	"context"
	"encoding/csv"
	"os"
	"strconv"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// singleMoveCmdNo is the command number of the Single Move request message.
	singleMoveCmdNo uint16 = 4
	// singleMoveReqLength is the length of the Single Move request.
	// The Single Move request contains the following component:
	// - command number: 2 bytes
	// - response length: 2 bytes
	// - counter: 4 bytes
	// - joint positions: 4 bytes * 8 joints
	// - duration: 4 bytes
	singleMoveReqLength uint16 = 44
	// degreeToARCScale is the constant transformed the degree to arc unit in the
	// control box of the amber robotic arm.
	degreeToARCScale = 0.017453292519943295
)

type singleMoveRequest []*armData

// newSingleMoveRequest generates a new "Single Move" request with given counter, positions and operation duration.
// More details can be found at https://github.com/MrAsana/UDP-Protocol-API/wiki/Robotic-Arm-API-based-on-UDP-Protocol#2-robotjoints-move-once.
func newSingleMoveRequest(counter uint32, positions []float32, duration float32) ([]*armData, error) {
	// Convert the positions from degree to ARC.
	var positionValues []any
	for _, value := range positions {
		positionValues = append(positionValues, value*degreeToARCScale)
	}
	req := singleMoveRequest{}
	for _, dataPair := range []struct {
		data  []*armData
		value []any
	}{
		{
			data:  newCommonData(),
			value: []any{singleMoveCmdNo, singleMoveReqLength, counter},
		},
		{
			data:  newJointPosData(),
			value: positionValues,
		},
		{
			data:  newTimeData(),
			value: []any{duration},
		},
	} {
		if err := setValuesToDataSlice(dataPair.data, dataPair.value); err != nil {
			return nil, errors.Wrap(err, "failed to set values to the arm data")
		}
		req = append(req, dataPair.data...)
	}
	return req, nil
}

// SingleMove moves the arm to the given position for |duration|.
// This function reads the position params in degree.
func (a *Arm) SingleMove(ctx context.Context, positions []float32, duration time.Duration) error {
	counter := a.getNextCounter()
	req, err := newSingleMoveRequest(counter, positions, float32(duration.Seconds()))
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}
	if err := a.sendRequest(req); err != nil {
		return errors.Wrap(err, "failed to send request")
	}

	res := newGeneralResponse()
	// Read the response from the single move.
	// Note that the response is returned immediately after the joint starts processing the request.
	if err := a.readResponse(generalResLength, res); err != nil {
		return errors.Wrap(err, "failed to read single move response")
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

	// GoBigSleepLint: Wait for the operation to complete.
	// Note that the response success only indicates that the request was received and action has started,
	// not that it has been executed. Therefore, a fixed sleep duration is needed to wait for the operation to finish.
	if err := testing.Sleep(ctx, duration); err != nil {
		return errors.Wrap(err, "failed to wait for operation executed")
	}
	return nil
}

// MultiMove iterates through the given positions and move the robotic arm accordingly.
// |duration| will be read as the duration of each "SingleMove" operation.
func (a *Arm) MultiMove(ctx context.Context, multiPositions [][]float32, duration time.Duration) error {
	for _, positions := range multiPositions {
		if err := a.SingleMove(ctx, positions, duration); err != nil {
			return errors.Wrap(err, "failed to move")
		}
	}
	return nil
}

// MoveToInitialPosition moves the arm to its initial position.
func (a *Arm) MoveToInitialPosition(ctx context.Context) error {
	// Initial position for 7 joints and 1 gripper.
	initPositions := []float32{0, 0, 0, 0, 0, 0, 0, 0}
	if err := a.SingleMove(ctx, initPositions, defaultMoveDuration); err != nil {
		return errors.Wrap(err, "failed to return to initial position")
	}
	return nil
}

// ParsePositionsFromCSV reads the positions from a csv file.
// Each line of the CSV file is expected to have 7 values corresponding
// to the position of the 7 joints of the robotic arm.
func ParsePositionsFromCSV(filePath string) ([][]float32, error) {
	// Open the CSV file.
	f, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open file %s", filePath)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read file %s", filePath)
	}

	// multiPositions represents multiple sets of joint positions.
	var multiPositions [][]float32
	for _, record := range records {
		// positions represents a set of joint positions.
		var positions []float32
		for _, singleJointData := range record {
			singleJointPos, err := strconv.ParseFloat(singleJointData, 32)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert data to float32")
			}
			positions = append(positions, float32(singleJointPos))
		}
		// Append the position data of the gripper.
		// The movement command does not support gripper control, so always set it as 0.
		positions = append(positions, 0)
		multiPositions = append(multiPositions, positions)
	}
	return multiPositions, nil
}
