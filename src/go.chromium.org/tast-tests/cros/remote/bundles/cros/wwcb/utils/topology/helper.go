// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package topology contains tools to interact with PASIT topology components.
package topology

import (
	"context"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DeviceType is the type of device represented in the topology.
type DeviceType labapi.PasitHost_Device_Type

// DeviceType aliases for labapi types.
const (
	DeviceTypeDUT            = labapi.PasitHost_Device_DUT
	DeviceTypeSwitchFixture  = labapi.PasitHost_Device_SWITCH_FIXTURE
	DeviceTypeDockingStation = labapi.PasitHost_Device_DOCKING_STATION
	DeviceTypeMonitor        = labapi.PasitHost_Device_MONITOR
	DeviceTypeCamera         = labapi.PasitHost_Device_CAMERA
	DeviceTypeStorage        = labapi.PasitHost_Device_STORAGE
	DeviceTypeHID            = labapi.PasitHost_Device_HID
	DeviceTypeNetwork        = labapi.PasitHost_Device_NETWORK
	DeviceTypeHeadphone      = labapi.PasitHost_Device_HEADPHONE
	DeviceTypeSpeaker        = labapi.PasitHost_Device_SPEAKER
)

// Helper provides utilities for traversing the PASIT topology.
type Helper struct {
	topology *labapi.PasitHost
	// hostname of the DUT.
	hostname string
	// directed connection graph between devices.
	connections map[string][]*Connection
	// devices in the topology.
	devices map[string]*labapi.PasitHost_Device
}

// NewHelper creates a new PASIT topology helper for the given topology and host.
func NewHelper(topology *labapi.PasitHost, hostname string) *Helper {
	// Cache devices in the topology.
	devices := make(map[string]*labapi.PasitHost_Device)
	for _, d := range topology.GetDevices() {
		devices[d.GetId()] = d
	}

	// Cache connections between devices.
	connections := make(map[string][]*Connection)
	for _, c := range topology.GetConnections() {
		child := c.GetChildId()
		parent := c.GetParentId()

		manager := connectionManager(devices[parent], c)
		connections[parent] = append(connections[parent], newConnection(c, false, manager))
		connections[child] = append(connections[child], newConnection(c, true, manager))
	}

	return &Helper{
		topology:    topology,
		hostname:    hostname,
		connections: connections,
		devices:     devices,
	}
}

// connectionManager gets the correct switch wrapper for a given connection.
func connectionManager(parent *labapi.PasitHost_Device, conn *labapi.PasitHost_Connection) ConnectionManager {
	// If the parent device is not a switch then we have nothing to return.
	// Switches are always defined from parent device to child.
	if parent.GetType() != DeviceTypeSwitchFixture {
		return nil
	}

	// We only support alieon switches at the moment.
	return &allionConnectionManager{id: parent.GetId()}
}

// InitializeFixtures initializes the fixtures in the topology.
func (t *Helper) InitializeFixtures(ctx context.Context) error {
	if err := utils.InitFixture(ctx); err != nil {
		return errors.Wrap(err, "failed to initialize fixtures")
	}
	return nil
}

// CloseAll cleans up and releases any resources held open by the fixtures.
func (t *Helper) CloseAll(ctx context.Context) error {
	return utils.CloseAllFixture(ctx)
}

// ResetAll resets the fixture state to the default.
func (t *Helper) ResetAll(ctx context.Context) error {
	return utils.CloseAllFixture(ctx)
}

// devicePredicate is a function that returns true if this is the device that we're searching for.
type devicePredicate func(*labapi.PasitHost_Device) bool

// DevicesByType returns a list of devices with IDs matching the requested type.
func (t *Helper) DevicesByType(deviceType labapi.PasitHost_Device_Type) []string {
	var devices []string
	for _, d := range t.devices {
		if d.GetType() == deviceType {
			devices = append(devices, d.GetId())
		}
	}
	return devices
}

// PathToDeviceByType gets the connection path between the DUT and the first device of the requested type.
func (t *Helper) PathToDeviceByType(deviceType labapi.PasitHost_Device_Type) (string, ConnectionPath, error) {
	predicate := func(device *labapi.PasitHost_Device) bool {
		return device.GetType() == deviceType
	}
	devices, err := t.path(predicate)
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to find path to device with type: %v", deviceType)
	}
	return devices[len(devices)-1], t.connectionsInPath(devices), nil
}

// ActivateDeviceByType enables the first found component of the specified type and returns the device.
func (t *Helper) ActivateDeviceByType(ctx context.Context, deviceType labapi.PasitHost_Device_Type) (string, error) {
	device, path, err := t.PathToDeviceByType(deviceType)
	if err != nil {
		return "", errors.Wrap(err, "failed to get path to device")
	}
	testing.ContextLogf(ctx, "Found path to device: %v: %v", deviceType, path)

	if err := path.Activate(ctx); err != nil {
		return "", errors.Wrap(err, "failed to activate device path")
	}
	return device, nil
}

// DeactivateDeviceByType disables the first found component of the specified type.
func (t *Helper) DeactivateDeviceByType(ctx context.Context, deviceType labapi.PasitHost_Device_Type) (string, error) {
	device, path, err := t.PathToDeviceByType(deviceType)
	if err != nil {
		return "", errors.Wrap(err, "failed to get path to device")
	}
	testing.ContextLogf(ctx, "Found path to device: %v: %v", deviceType, path)

	if err := path.DisableLast(ctx); err != nil {
		return "", errors.Wrap(err, "failed to disable device path")
	}
	return device, nil
}

// PathToDeviceByID gets the connection path between the DUT and the device with the matching ID.
func (t *Helper) PathToDeviceByID(id string) (ConnectionPath, error) {
	predicate := func(device *labapi.PasitHost_Device) bool {
		return device.GetId() == id
	}
	devices, err := t.path(predicate)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find path to device with id: %q", id)
	}
	return t.connectionsInPath(devices), nil
}

// ActivateDeviceByID enables the component with the provided ID.
func (t *Helper) ActivateDeviceByID(ctx context.Context, id string) error {
	path, err := t.PathToDeviceByID(id)
	if err != nil {
		return errors.Wrap(err, "failed to get path to device")
	}
	testing.ContextLogf(ctx, "Found path to device: %q: %v", id, path)

	if err := path.Activate(ctx); err != nil {
		return errors.Wrap(err, "failed to activate device path")
	}
	return nil
}

// DeactivateDeviceByID disables the component with the provided ID.
func (t *Helper) DeactivateDeviceByID(ctx context.Context, id string) error {
	path, err := t.PathToDeviceByID(id)
	if err != nil {
		return errors.Wrap(err, "failed to get path to device")
	}
	testing.ContextLogf(ctx, "Found path to device: %q: %v", id, path)

	if err := path.DisableLast(ctx); err != nil {
		return errors.Wrap(err, "failed to disable device path")
	}
	return nil
}

// Helper stack implementation for traversing PASIT topology.
type stack []string

func (s *stack) pop() string {
	val := *s
	if len(val) == 0 {
		panic("stack is empty")
	}
	*s = val[:len(val)-1]
	return val[len(val)-1]
}

// path traverses the topology graph using a simple DFS to find the devices (nodes) between the
// host and the first device that matches devicePredicate.
func (t *Helper) path(predicate devicePredicate) ([]string, error) {
	// create a queue to traverse our graph
	currentPath := stack{}
	stack := stack{t.hostname}
	visited := map[string]bool{t.hostname: true}
	children := make(map[string]int)

	for len(stack) > 0 {
		current := stack.pop()
		currentPath = append(currentPath, current)

		if predicate(t.devices[current]) {
			return currentPath, nil
		}

		for _, node := range t.connections[current] {
			if visited[node.childID] {
				continue
			}
			stack = append(stack, node.childID)
			visited[node.childID] = true
			children[current]++
		}

		// We need to walk back up the current path and pop all items that did not lead to our
		// destination.
		for len(currentPath) > 1 && children[current] == 0 {
			currentPath.pop()
			current = currentPath[len(currentPath)-1]
			children[current]--
		}
	}

	return nil, errors.New("failed to find matching node")
}

// connectionsInPath returns the connections between the devices in the path.
func (t *Helper) connectionsInPath(path []string) ConnectionPath {
	var connections ConnectionPath
	for i := 0; i < len(path)-1; i++ {
		parentID := path[i]
		childID := path[i+1]
		for _, c := range t.connections[parentID] {
			if childID == c.childID {
				connections = append(connections, c)
				break
			}
		}
	}
	return connections
}
