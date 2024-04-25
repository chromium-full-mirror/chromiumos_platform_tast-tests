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

	"go.chromium.org/tast-tests/cros/common/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// ModuleVersion for figuring out the type of starfish setup.
type ModuleVersion uint32

// All the ModuleVersion values
const (
	ModuleVersion0 ModuleVersion = 0 // starfish
	ModuleVersion1 ModuleVersion = 1 // starfish+
)

// Version0Str and Version1Str are the string
// representations of the two types of starfish setups.
const (
	Version0Str = "starfish"
	Version1Str = "starfishplus"
)

// StarfishTypeVar indicates type of current starfish setup.
var starfishTypeVar = testing.RegisterVarString(
	"starfish.type",
	"",
	"starfish.type",
)

// StarfishCarrierVar indicates carrier from current active
// starfish slot.
var StarfishCarrierVar = testing.RegisterVarString(
	"starfish.carrier",
	"",
	"starfish.carrier",
)

// StarfishNotFound indicates missing startfish type, index and carrier
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
	sp        *shim
	devID     string
	fwVer     string
	index     int
	sfVersion ModuleVersion
	simSlots  map[int]struct{}
}

// NewStarfish creates a Starfish object and ensures that it is configured properly.
func NewStarfish(ctx context.Context, di *cellular.DUTInfo) (*Starfish, int, string, error) {
	ctx, st := timing.Start(ctx, "Starfish.NewStarfish")
	defer st.End()

	// If carrier is empty then assume we are not on a starfish DUT.
	carrier := StarfishCarrierVar.Value()
	if carrier == "" {
		testing.ContextLog(ctx, "No starfish carrier provided")
		return nil, -1, StarfishNotFound, nil
	}

	var sfVersionStr string
	// Determine starfish version from either starfishType var or from DUT's 'label-carrier' dimension.
	if starfishTypeVar.Value() != "" {
		sfVersionStr = starfishTypeVar.Value()
		testing.ContextLogf(ctx, "using starfish version: %q from cmdline var", sfVersionStr)
	} else if di == nil || di.CarrierName == "" {
		// If CarrierName is empty but CarrierVar is provided, then we are on a starfish DUT,
		// running Tast second class, default to type: Version0Str.
		sfVersionStr = Version0Str
		testing.ContextLogf(ctx, "starfish version not provided, defaulting to: %q", sfVersionStr)
	} else if strings.EqualFold(di.CarrierName, "STARFISH") {
		sfVersionStr = Version0Str
		testing.ContextLogf(ctx, "using starfish version: %q from dut config", sfVersionStr)
	} else if strings.EqualFold(di.CarrierName, "STARFISHPLUS") {
		sfVersionStr = Version1Str
		testing.ContextLogf(ctx, "using starfish version: %q from dut config", sfVersionStr)
	} else {
		// di.CarrierName is not empty and is not starfish -> We are sure we're not on a starfish DUT.
		testing.ContextLog(ctx, "not a starfish testbed")
		return nil, -1, StarfishNotFound, nil
	}

	var index int
	if StarfishIndexVar.Value() != StarfishNotFound {
		i, err := strconv.Atoi(StarfishIndexVar.Value())
		if err != nil {
			return nil, -1, StarfishNotFound, errors.Wrapf(err, "failed to parse starfish index: %q", StarfishIndexVar.Value())
		}
		index = i
		testing.ContextLogf(ctx, "using starfish index: %d from cmdline var", index)
	} else if di == nil {
		return nil, -1, StarfishNotFound, errors.Errorf("failed to find starfish index for carrier %q, no dutinfo provided", carrier)
	} else if i, err := di.GetSlotForStarfishCarrier(carrier); err == nil {
		index = int(i)
		testing.ContextLogf(ctx, "using starfish index: %d from dut config", index)
	} else {
		return nil, -1, StarfishNotFound, errors.Errorf("failed to find starfish index for carrier %q", carrier)
	}

	sh, logs, err := NewShim(ctx)
	if err != nil {
		return nil, -1, StarfishNotFound, errors.Wrap(err, "failed to create shim object")
	}
	sfish := Starfish{sp: sh}
	sfish.printLogs(ctx, logs)

	sfish.setModuleVersion(ctx, sfVersionStr)

	if err := sfish.deviceID(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to read DeviceID: ", err.Error())
	}
	if err := sfish.simStatus(ctx); err != nil {
		return nil, -1, StarfishNotFound, errors.Wrap(err, "failed to read SIM slots status")
	}
	if err := sfish.SimEject(ctx); err != nil {
		return nil, -1, StarfishNotFound, errors.Wrap(err, "failed sim eject command")
	}
	return &sfish, index, carrier, nil
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

// setModuleVersion sets the version/type of the Starfish module based on the string runtime variable.
func (s *Starfish) setModuleVersion(ctx context.Context, sfVersionStr string) {
	testing.ContextLog(ctx, "type: ", sfVersionStr)
	if sfVersionStr == Version1Str {
		s.sfVersion = ModuleVersion1
	} else {
		s.sfVersion = ModuleVersion0
	}
}

// GetModuleVersion gets the version/type of the Starfish module (0 - starfish, 1 - starfish+, etc).
func (s *Starfish) GetModuleVersion(ctx context.Context) ModuleVersion {
	return s.sfVersion
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
