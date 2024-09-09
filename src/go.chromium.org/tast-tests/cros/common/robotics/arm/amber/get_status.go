// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package amber

import (
	"go.chromium.org/tast/core/errors"
)

const (
	// getStatusCmdNo is the command number of the Get Status request message.
	getStatusCmdNo uint16 = 1
	// getStatusReqLength is the length of the Get Status request.
	// The Get Status request contains the following component:
	// - command number: 2 bytes
	// - response length: 2 bytes
	// - counter: 4 bytes
	getStatusReqLength uint16 = 8
	// getStatusResLength is the length of the Get Status response.
	// The Get Status response contains the following component:
	// - command number: 2 bytes
	// - response length: 2 bytes
	// - counter: 4 bytes
	// - joint positions: 4 bytes * 8 joints
	// - joint speeds: 4 bytes * 8 joints
	// - cartesian positions: 4 bytes * 6 coordinates
	// - cartesian speeds: 4 bytes * 6 coordinates
	// - arm angle: 4 bytes
	getStatusResLength = 124
)

// ArmStatus is the status of the arm, including positions, speed, arm angle.
// The names of the fields should align with the name of the data defined in arm_data.go.
type ArmStatus struct {
	CmdNo        uint16
	Length       uint16
	Counter      uint32
	Joint1Pos    float32
	Joint2Pos    float32
	Joint3Pos    float32
	Joint4Pos    float32
	Joint5Pos    float32
	Joint6Pos    float32
	Joint7Pos    float32
	GripperPos   float32
	Joint1Speed  float32
	Joint2Speed  float32
	Joint3Speed  float32
	Joint4Speed  float32
	Joint5Speed  float32
	Joint6Speed  float32
	Joint7Speed  float32
	GripperSpeed float32
	XPos         float32
	YPos         float32
	ZPos         float32
	RollPos      float32
	PitchPos     float32
	YawPos       float32
	XSpeed       float32
	YSpeed       float32
	ZSpeed       float32
	RollSpeed    float32
	PitchSpeed   float32
	YawSpeed     float32
	ArmAngle     float32
}

// newArmStatus transforms a slice of armData into arm status.
func newArmStatus(dataSlice getStatusResponse) (*ArmStatus, error) {
	status := &ArmStatus{}
	if err := parseStatus(dataSlice, status); err != nil {
		return nil, errors.Wrap(err, "failed to parse the arm status")
	}
	return status, nil
}

type getStatusRequest []*armData

// newGetStatusRequest generates a new "Get Status" request with given counter.
// More details can be found at https://github.com/MrAsana/UDP-Protocol-API/wiki/Robotic-Arm-API-based-on-UDP-Protocol#1get-joints-status.
func newGetStatusRequest(counter uint32) (getStatusRequest, error) {
	commonValues := []any{getStatusCmdNo, getStatusReqLength, counter}
	commonData := newCommonData()
	if err := setValuesToDataSlice(commonData, commonValues); err != nil {
		return nil, errors.Wrap(err, "failed to generate common unit")
	}
	return commonData, nil
}

type getStatusResponse []*armData

// newGetStatusResponse generates an "Get Status" response.
func newGetStatusResponse() getStatusResponse {
	res := getStatusResponse{}
	resComponents := [][]*armData{
		newCommonData(),
		newJointPosData(),
		newJointSpeedData(),
		newCartesianPosData(),
		newCartesianSpeedData(),
		newArmAngleData(),
	}
	for _, data := range resComponents {
		res = append(res, data...)
	}
	return res
}

// GetStatus gets the status of the arms.
func (a *Arm) GetStatus() (*ArmStatus, error) {
	counter := a.getNextCounter()
	req, err := newGetStatusRequest(counter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}
	if err := a.sendRequest(req); err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}

	res := newGetStatusResponse()
	if err := a.readResponse(getStatusResLength, res); err != nil {
		return nil, errors.Wrap(err, "failed to read status response")
	}

	status, err := newArmStatus(res)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get arm status")
	}
	if status.Counter != counter {
		return nil, errors.Errorf("unmatched counter from response: got %d, want %d", status.Counter, counter)
	}
	return status, nil
}
