// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package starfish provides functions for testing starfish module.
package starfish

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// StarfishCarrierVar indicates carrier from current active
// starfish slot.
var StarfishCarrierVar = testing.RegisterVarString(
	"starfish.carrier",
	"",
	"starfish.carrier",
)

// StarfishNotFound indicates missing startfish index and carrier
// variables.
const StarfishNotFound = "---"

// StarfishIndexVar indicates the index of currently
// active starfish slot.
var StarfishIndexVar = testing.RegisterVarString(
	"starfish.index",
	StarfishNotFound,
	"starfish.index",
)

// Various string used to parse responses
const (
	simStr     = "SIM "
	ejectResp  = "SIM Mux disabled"
	insertResp = "SIM Mux set to "
	fwVerResp  = "Firmware Version: "
	devIDResp  = "Device ID: "
	foundStr   = "Found"
	noneStr    = "None"
)

// NoSimIndex is returned if none of the SIMs is actively connected
const NoSimIndex = -1

var exists = struct{}{}

// MaxSimSlots is max the number of SIM slots a Starfish module supports: 8
const MaxSimSlots = 8

// Starfish contains data pertaining to the current state, SIM selected, serial port, etc
type Starfish struct {
	sp       *shim
	devID    string
	fwVer    string
	index    int
	simSlots map[int]struct{}
}

// NewStarfish creates a Starfish object and ensures that it is configured properly.
func NewStarfish(ctx context.Context) (*Starfish, error) {
	ctx, st := timing.Start(ctx, "Starfish.NewStarfish")
	defer st.End()

	carrier := StarfishCarrierVar.Value()
	indexVar := StarfishIndexVar.Value()
	if indexVar == StarfishNotFound {
		testing.ContextLog(ctx, "starfish setup not supported for carrier: ", carrier)
		return nil, nil
	}
	index, err := strconv.Atoi(indexVar)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse starfish config info: %s", indexVar)
	}
	testing.ContextLog(ctx, "starfish setup for carrier: ", carrier, " in slot: ", index)
	sh, logs, err := NewShim(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create shim object")
	}
	sfish := Starfish{sp: sh}
	sfish.printLogs(ctx, logs)

	if err := sfish.deviceID(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to read DeviceID: ", err.Error())
	}
	if err := sfish.simStatus(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to read SIM slots status")
	}
	if err := sfish.SimEject(ctx); err != nil {
		return nil, errors.Wrap(err, "failed sim eject command")
	}
	if err := sfish.SimInsert(ctx, index); err != nil {
		return nil, errors.Errorf("failed sim insert command: %s", err)
	}

	return &sfish, nil
}

// deviceID reads the DeviceID of the Starfish module.
func (s *Starfish) deviceID(ctx context.Context) error {
	responses, logs, err := s.sp.SendCommand(ctx, "info")
	s.printLogs(ctx, logs)
	if err != nil {
		return err
	}
	n := len(responses)
	if n < 2 {
		return errors.New("invalid response")
	}
	if !strings.HasPrefix(responses[n-2], fwVerResp) {
		return errors.Errorf("invalid response: %s", responses)
	}
	if !strings.HasPrefix(responses[n-1], devIDResp) {
		return errors.Errorf("invalid response: %s", responses)
	}
	s.fwVer = strings.TrimPrefix(responses[n-2], fwVerResp)
	s.devID = strings.TrimPrefix(responses[n-1], devIDResp)
	testing.ContextLog(ctx, "fw version: ", s.fwVer)
	testing.ContextLog(ctx, "device id: ", s.devID)
	return nil
}

// simStatus queries and indicates the list of populated SIM slots, [0-7]
func (s *Starfish) simStatus(ctx context.Context) error {
	responses, logs, err := s.sp.SendCommand(ctx, "sim status")
	s.printLogs(ctx, logs)
	if err != nil {
		return err
	}
	if len(responses) != MaxSimSlots {
		return errors.Errorf("invalid response length: %s", responses)
	}
	var list []int
	for i := 0; i < MaxSimSlots; i++ {
		pref := simStr + strconv.Itoa(i) + " = "
		if !strings.HasPrefix(responses[i], pref) {
			return errors.Errorf("invalid response: %s", responses)
		}
		x := strings.TrimPrefix(responses[i], pref)
		if x == foundStr {
			list = append(list, i)
		} else if x != noneStr {
			return errors.Errorf("invalid response: %s", responses)
		}
	}
	testing.ContextLog(ctx, "sims found: ", list)
	s.simSlots = make(map[int]struct{})
	for _, i := range list {
		s.simSlots[i] = exists
	}
	return nil
}

// SimInsert emulates insertion of SIM into slot n [0-7]
func (s *Starfish) SimInsert(ctx context.Context, n int) error {
	if n < 0 || n >= MaxSimSlots {
		return errors.Errorf("invalid sim slot index: %d", n)
	}
	if _, ok := s.simSlots[n]; !ok {
		return errors.Errorf("inactive sim slot index: %d", n)
	}
	if s.index == n {
		testing.ContextLog(ctx, "sim already inserted ", n)
		return nil
	}
	if s.index != NoSimIndex {
		testing.ContextLog(ctx, "ejecting active sim first")
		if err := s.SimEject(ctx); err != nil {
			return err
		}
	}
	var command string = fmt.Sprintf("sim connect -n %d", n)
	responses, logs, err := s.sp.SendCommand(ctx, command)
	s.printLogs(ctx, logs)
	if err != nil {
		// todo: remove this once the misplaced <err> flag is removed in starfish FW
		if responses[0] != fmt.Sprintf("New state %d", n) {
			return err
		}
	}
	if len(responses) < 1 {
		return errors.New("invalid response")
	}
	if !strings.Contains(responses[0], fmt.Sprintf("%s%d", insertResp, n)) {
		return errors.Errorf("invalid response: %s", responses)
	}
	s.index = n
	testing.ContextLog(ctx, "sim inserted ", n)
	return nil
}

// SimEject emulates ejection of the active SIM slot
func (s *Starfish) SimEject(ctx context.Context) error {
	if s.index == NoSimIndex {
		testing.ContextLog(ctx, "sim already ejected ")
		return nil
	}
	responses, logs, err := s.sp.SendCommand(ctx, "sim eject")
	s.printLogs(ctx, logs)
	if err != nil {
		return err
	}
	if len(responses) < 1 {
		s.index = NoSimIndex
		testing.ContextLog(ctx, "sim eject: warning, empty response received")
		return nil
	}
	if (responses[0] != "") && (!strings.Contains(responses[0], ejectResp)) {
		return errors.Errorf("invalid response: %s", responses[0])
	}
	t := s.index
	s.index = NoSimIndex
	if responses[0] == "" {
		testing.ContextLog(ctx, "No sim present to be ejected")
	} else {
		testing.ContextLog(ctx, "sim ejected ", t)
	}
	return nil
}

// Teardown handles the close of the module
func (s *Starfish) Teardown(ctx context.Context) error {
	testing.ContextLog(ctx, "starfish teardown")
	if err := s.SimEject(ctx); err != nil {
		testing.ContextLog(ctx, "Failed sim eject command: ", err)
	}
	return s.sp.Close(ctx)
}

// ActiveSimSlot indicates the current active SIM slot, [0-7], -1 indicates no active SIM.
func (s *Starfish) ActiveSimSlot(ctx context.Context) (int, bool) {
	return s.index, s.index != NoSimIndex
}

// AvailableSimSlots indicates the cached list of populated SIM slots, [0-7] and active SIM, -1 indicates no active SIM.
func (s *Starfish) AvailableSimSlots(ctx context.Context) ([]int, int) {
	l := make([]int, 0, MaxSimSlots)
	for i := range s.simSlots {
		l = append(l, i)
	}
	sort.Ints(l)
	return l, s.index
}

// printLogs prints logs from the Starfish module
func (s *Starfish) printLogs(ctx context.Context, logs []string) {
	if logs == nil {
		return
	}
	for _, line := range logs {
		testing.ContextLog(ctx, "--Starfish:~$ ", line)
	}
}

// ParseStarfishSlotMapping builds a map of slot to carrier mapping from
// starfish slot mapping auto label.
func (s *Starfish) ParseStarfishSlotMapping(slotCarrierMapping string) map[int]string {
	slots := strings.Split(slotCarrierMapping, ",")
	slotCarrierMap := make(map[int]string)
	for _, slot := range slots {
		parts := strings.Split(slot, "_")
		if len(parts) == 2 {
			index, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			slotCarrierMap[index] = parts[1]
		}
	}
	return slotCarrierMap
}
