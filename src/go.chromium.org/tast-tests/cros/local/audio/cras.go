// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package audio interacts with audio operation.
package audio

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast-tests/cros/local/audio/types"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dbusName      = "org.chromium.cras"
	dbusPath      = "/org/chromium/cras"
	dbusInterface = "org.chromium.cras.Control"
)

// StreamType is an alias.
type StreamType = types.StreamType

// Stream direction aliases.
const (
	InputStream  = types.InputStream
	OutputStream = types.OutputStream
)

// Cras is used to interact with the cras process over D-Bus.
// For detailed spec, please find src/third_party/adhd/cras/README.dbus-api.
type Cras struct {
	obj dbus.BusObject
}

// NewCras connects to CRAS via D-Bus and returns a Cras object.
func NewCras(ctx context.Context) (*Cras, error) {
	testing.ContextLogf(ctx, "Waiting for %s D-Bus service", dbusName)
	_, obj, err := dbusutil.Connect(ctx, dbusName, dbusPath)
	if err != nil {
		return nil, err
	}
	return &Cras{obj}, nil
}

// RestartCras restarts CRAS and wait for it to be online.
func RestartCras(ctx context.Context) (*Cras, error) {
	testing.ContextLog(ctx, "Restarting CRAS")
	if err := upstart.RestartJob(ctx, "cras"); err != nil {
		return nil, err
	}

	// Wait for CRAS to be online.
	cras, err := NewCras(ctx)
	if err == nil {
		testing.ContextLog(ctx, "CRAS restarted")
	} else {
		return nil, err
	}

	const waitDLCUpTo = 10 * time.Second
	// Wait until DLCs are initialized, for up to waitDLCUpTo.
	if err := testing.Poll(ctx,
		func(ctx context.Context) error {
			s2, err := cras.dumpS2(ctx)
			if err != nil {
				return errors.Wrap(err, "dump s2")
			}
			if !s2.Input.DLCManagerReady {
				return errors.New("CRAS DLC manager not ready")
			}
			return nil
		}, &testing.PollOptions{
			Timeout:  waitDLCUpTo,
			Interval: 1 * time.Second,
		},
	); err != nil {
		testing.ContextLog(ctx, "Waiting for DLC manager: ", err)
	}

	return cras, nil
}

// CrasNode is an alias.
type CrasNode = types.CrasNode

// VolumeState contains the metadata of volume state in Cras.
// Currently fields which are actually needed by tests are defined.
// Please find src/third_party/adhd/cras/dbus_bindings/org.chromium.cras.Control.xml
// for the meaning of each fields.
type VolumeState struct {
	OutputVol      int
	OutputMute     bool
	InputMute      bool
	OutputUserMute bool
}

// GetNodes calls cras.Control.GetNodes over D-Bus.
func (c *Cras) GetNodes(ctx context.Context) ([]CrasNode, error) {
	call := c.call(ctx, "GetNodes")
	if call.Err != nil {
		return nil, call.Err
	}

	// cras.Control.GetNodes D-Bus method's signature is not fixed.
	// Specifically, the number of output values depends on the actual
	// number of nodes.
	// That usage is not common practice, and it is less supported in
	// godbus. Here, instead, values are manually converted via
	// dbus.Variant.
	nodes := make([]CrasNode, len(call.Body))
	for i, n := range call.Body {
		mp := n.(map[string]dbus.Variant)
		if id, ok := mp["Id"]; !ok {
			return nil, errors.Errorf("'Id' not found: %v", mp)
		} else if nodes[i].ID, ok = id.Value().(uint64); !ok {
			return nil, errors.Errorf("'Id' is not uint64: %v", mp)
		}
		if nodeType, ok := mp["Type"]; !ok {
			return nil, errors.Errorf("'Type' not found: %v", mp)
		} else if nodes[i].Type, ok = nodeType.Value().(string); !ok {
			return nil, errors.Errorf("'Type' is not string: %v", mp)
		}
		if active, ok := mp["Active"]; !ok {
			return nil, errors.Errorf("'Active' not found: %v", mp)
		} else if nodes[i].Active, ok = active.Value().(bool); !ok {
			return nil, errors.Errorf("'Active' is not bool: %v", mp)
		}
		if isInput, ok := mp["IsInput"]; !ok {
			return nil, errors.Errorf("'IsInput' not found: %v", mp)
		} else if nodes[i].IsInput, ok = isInput.Value().(bool); !ok {
			return nil, errors.Errorf("'IsInput' is not bool: %v", mp)
		}
		if name, ok := mp["Name"]; !ok {
			return nil, errors.Errorf("'Name' not found: %v", mp)
		} else if nodes[i].Name, ok = name.Value().(string); !ok {
			return nil, errors.Errorf("'Name' is not string: %v", mp)
		}
		if deviceName, ok := mp["DeviceName"]; !ok {
			return nil, errors.Errorf("'DeviceName' not found: %v", mp)
		} else if nodes[i].DeviceName, ok = deviceName.Value().(string); !ok {
			return nil, errors.Errorf("'DeviceName' is not string: %v", mp)
		}
		if nodeVolume, ok := mp["NodeVolume"]; !ok {
			return nil, errors.Errorf("'NodeVolume' not found: %v", mp)
		} else if nodes[i].NodeVolume, ok = nodeVolume.Value().(uint64); !ok {
			return nil, errors.Errorf("'NodeVolume' is not uint64: %v", mp)
		}
	}
	return nodes, nil
}

// GetNodeByMatcher returns the first node matching the matcher.
func (c *Cras) GetNodeByMatcher(ctx context.Context, matcher nodematch.Matcher) (*CrasNode, error) {
	nodes, err := c.GetNodes(ctx)
	if err != nil {
		return nil, err
	}

	for _, n := range nodes {
		if matcher.Match(&n) {
			return &n, nil
		}
	}

	return nil, errors.Errorf("failed to find a node matching %s", matcher)
}

// GetNodeByType returns the first node with given type.
func (c *Cras) GetNodeByType(ctx context.Context, t string) (*CrasNode, error) {
	return c.GetNodeByMatcher(ctx, nodematch.Type(t))
}

// call is a wrapper around CallWithContext for convenience.
func (c *Cras) call(ctx context.Context, method string, args ...interface{}) *dbus.Call {
	return c.obj.CallWithContext(ctx, dbusInterface+"."+method, 0, args...)
}

// SetActiveNode calls SetActiveInputNode or SetActiveOutputNode based on
// whether the node is an input or output node.
func (c *Cras) SetActiveNode(ctx context.Context, node CrasNode) error {
	if node.IsInput {
		return c.SetActiveInputNode(ctx, node.ID)
	}
	return c.SetActiveOutputNode(ctx, node.ID)
}

// SetActiveOutputNode calls cras.Control.SetActiveOutputNode over D-Bus.
func (c *Cras) SetActiveOutputNode(ctx context.Context, nodeID uint64) error {
	return c.call(ctx, "SetActiveOutputNode", nodeID).Err
}

// SetActiveInputNode calls cras.Control.SetActiveOutputNode over D-Bus.
func (c *Cras) SetActiveInputNode(ctx context.Context, nodeID uint64) error {
	return c.call(ctx, "SetActiveInputNode", nodeID).Err
}

// SetActiveNodeByMatcher sets node with specified matcher active.
func (c *Cras) SetActiveNodeByMatcher(ctx context.Context, matcher nodematch.Matcher) error {
	var node *CrasNode

	// Wait until the node with this type is existing.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		n, err := c.GetNodeByMatcher(ctx, matcher)
		node = n
		return err
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Errorf("failed to wait node %s", matcher)
	}

	if err := c.SetActiveNode(ctx, *node); err != nil {
		return errors.Errorf("failed to set node %s active", matcher)
	}

	// Wait until that node is active.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		n, err := c.GetNodeByMatcher(ctx, matcher)
		if err != nil {
			return err
		}
		if !n.Active {
			return errors.New("node is not active")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Errorf("failed to wait node %s to be active", matcher)
	}

	return nil
}

// SetActiveNodeByType sets node with specified type active.
func (c *Cras) SetActiveNodeByType(ctx context.Context, nodeType string) error {
	return c.SetActiveNodeByMatcher(ctx, nodematch.Type(nodeType))
}

// SetOutputNodeVolume calls cras.Control.SetOutputNodeVolume over D-Bus.
func (c *Cras) SetOutputNodeVolume(ctx context.Context, node CrasNode, volume int) error {
	return c.call(ctx, "SetOutputNodeVolume", node.ID, volume).Err
}

// GetVolumeState calls cras.Control.GetVolumeState over D-Bus.
func (c *Cras) GetVolumeState(ctx context.Context) (*VolumeState, error) {
	var vol int32
	var outputMute, inputMute, outputUserMute bool

	err := c.call(ctx, "GetVolumeState").Store(&vol, &outputMute, &inputMute, &outputUserMute)
	if err != nil {
		return nil, err
	}

	return &VolumeState{
		OutputVol:      int(vol),
		OutputMute:     outputMute,
		InputMute:      inputMute,
		OutputUserMute: outputUserMute,
	}, nil
}

// WaitForDeviceUntil waits until any cras node meets the given condition.
// condition is a function that takes a cras node as input and returns true if the node status
// satisfies the criteria.
func (c *Cras) WaitForDeviceUntil(ctx context.Context, condition func(*CrasNode) bool, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		nodes, err := c.GetNodes(ctx)
		if err != nil {
			return err
		}

		for _, n := range nodes {
			if condition(&n) {
				return nil
			}
		}
		return errors.New("cras node(s) not in requested condition")
	}, &testing.PollOptions{Timeout: timeout, Interval: 1 * time.Second})
}

// WaitForDevice waits for specified types of stream nodes to be active.
// There are 4 ways to select active audio device:
// 1. Stop UI and select with cras.SetActiveNodeByType.
// 2. Start Chrome with --use-fake-cras-audio-client-for-dbus and select with cras.SetActiveNodeByType
// 3. Start Chrome without --use-fake-cras-audio-client-for-dbus and select with UI automation
// 4. Start Chrome without --use-fake-cras-audio-client-for-dbus and use WaitForDevice just to ensure CRAS is ready
// Notice that if Chrome login breaks, tests rely on the Chrome may fail unrelated to audio functionality.
// You can pass the streamType as a bitmap to wait for both input and output
// nodes to be active. Ex: WaitForDevice(ctx, InputStream|OutputStream)
// It should be used to verify the target types of nodes exist and are
// active before the real test starts.
// Notice that some devices use their displays as an internal speaker
// (e.g. monroe). When a display is closed, the internal speaker is removed,
// too. For this case, we should call power.TurnOnDisplay to turn on a display
// to re-enable an internal speaker.
func WaitForDevice(ctx context.Context, streamType StreamType) error {
	cras, err := NewCras(ctx)
	if err != nil {
		return err
	}

	var active StreamType
	checkActiveNode := func(n *CrasNode) bool {
		if !n.Active {
			return false
		}
		if n.IsInput {
			active |= InputStream
		} else {
			active |= OutputStream
		}
		return streamType&active == streamType
	}

	return cras.WaitForDeviceUntil(ctx, checkActiveNode, 10*time.Second)
}

// SelectedInputDevice returns the active input device name and type.
func (c *Cras) SelectedInputDevice(ctx context.Context) (deviceName, deviceType string, err error) {
	node, err := c.SelectedInputNode(ctx)
	if err != nil {
		return
	}
	return node.DeviceName, node.Type, nil
}

// SelectedInputNode returns the active input node.
func (c *Cras) SelectedInputNode(ctx context.Context) (*CrasNode, error) {
	nodes, err := c.GetNodes(ctx)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if node.Active && node.IsInput {
			return &node, nil
		}
	}
	return nil, errors.New("no input node found")
}

// SelectedOutputDevice returns the active output device name and type.
func (c *Cras) SelectedOutputDevice(ctx context.Context) (deviceName, deviceType string, err error) {
	node, err := c.SelectedOutputNode(ctx)
	if err != nil {
		return
	}
	return node.DeviceName, node.Type, nil
}

// SelectedOutputNode returns the active output node.
func (c *Cras) SelectedOutputNode(ctx context.Context) (*CrasNode, error) {
	nodes, err := c.GetNodes(ctx)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if node.Active && !node.IsInput {
			return &node, nil
		}
	}
	return nil, errors.New("no output node found")
}

// GetNumberOfNonChromeOutputStreams returns the number of active output streams,
// excluding those from Chrome and LaCrOS.
func (c *Cras) GetNumberOfNonChromeOutputStreams(ctx context.Context) (int, error) {
	var numNonChromeOutputStreams int
	err := c.call(ctx, "GetNumberOfNonChromeOutputStreams").Store(&numNonChromeOutputStreams)
	return numNonChromeOutputStreams, err
}

// SetInputMute sets the input mute state
func (c *Cras) SetInputMute(ctx context.Context, mute bool) error {
	return c.call(ctx, "SetInputMute", mute).Err
}

// SetSpeakOnMuteDetection enables or disables speak-on-mute detection.
func (c *Cras) SetSpeakOnMuteDetection(ctx context.Context, enabled bool) error {
	return c.call(ctx, "SetSpeakOnMuteDetection", enabled).Err
}

// SetFlossEnabled sets whether the CRAS stack expects to use the floss
// bluetooth stack. Will expect floss if enabled, otherwise bluez.
func (c *Cras) SetFlossEnabled(ctx context.Context, enabled bool) error {
	return c.call(ctx, "SetFlossEnabled", enabled).Err
}

// SetVoiceIsolationUIEnabled enables or disables voice isolation effects.
func (c *Cras) SetVoiceIsolationUIEnabled(ctx context.Context, enabled bool) error {
	return c.call(ctx, "SetVoiceIsolationUIEnabled", enabled).Err
}

// IsStyleTransferSupported returns support status of the queried device.
func (c *Cras) IsStyleTransferSupported(ctx context.Context) (supported bool, err error) {
	err = c.call(ctx, "IsStyleTransferSupported").Store(&supported)
	return supported, err
}

// SetSidetoneEnabled enables or disables sidetone.
func (c *Cras) SetSidetoneEnabled(ctx context.Context, enabled bool) error {
	return c.call(ctx, "SetSidetoneEnabled", enabled).Err
}

// SetEwmaPowerReportEnabled enables or disables ewma power report.
func (c *Cras) SetEwmaPowerReportEnabled(ctx context.Context, enabled bool) error {
	return c.call(ctx, "SetEwmaPowerReportEnabled", enabled).Err
}

// GetFeatureFlagForTest returns the enabled status of the given feature as seen by CRAS.
func (c *Cras) GetFeatureFlagForTest(ctx context.Context, flagName string) (enabled bool, err error) {
	err = c.call(ctx, "GetFeatureFlagForTest", flagName).Store(&enabled)
	return enabled, err
}

// WaitUntilFeatureFlagHasValue waits until the feature flag named by flagName has the value expected.
func (c *Cras) WaitUntilFeatureFlagHasValue(ctx context.Context, flagName string, expected bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		flagValue, err := c.GetFeatureFlagForTest(ctx, flagName)
		if err != nil {
			return testing.PollBreak(err)
		}
		testing.ContextLogf(ctx, "GetAudioTestFeatureFlag(%q) = %v", flagName, flagValue)
		if flagValue != expected {
			return errors.Errorf("GetAudioTestFeatureFlag(%q) = %v; want %v", flagName, flagValue, expected)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: time.Second,
	})
}

// WaitForAudioEffectsReady waits for s2.Output.AudioEffectsReady to be true.
func (c *Cras) WaitForAudioEffectsReady(ctx context.Context) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		s2, err := c.dumpS2(ctx)
		if err != nil {
			return errors.Wrap(err, "dumpS2()")
		}
		if !s2.Output.AudioEffectsReady {
			return errors.Wrap(err, "audio effects not ready")
		}
		return nil
	},
		&testing.PollOptions{
			Timeout:  3 * time.Minute,
			Interval: 3 * time.Second,
		},
	)
}

// GetForceHFPSwbEnabled returns the response of the dbus method of the same name.
func (c *Cras) GetForceHFPSwbEnabled(ctx context.Context) (enabled bool, err error) {
	err = c.call(ctx, "GetForceHFPSwbEnabled").Store(&enabled)
	return enabled, err
}

// SetForceHFPSwbEnabled calls the dbus method of the same name.
func (c *Cras) SetForceHFPSwbEnabled(ctx context.Context, enabled bool) error {
	return c.call(ctx, "SetForceHFPSwbEnabled", enabled).Err
}

// GetDSPOffloadSupported returns whether the given node supports DSP offload to SOF.
func (c *Cras) GetDSPOffloadSupported(ctx context.Context, node CrasNode) (supported bool, err error) {
	err = c.call(ctx, "GetDSPOffloadSupported", node.ID).Store(&supported)
	return supported, err
}

// IsHfpMicSrSupported returns whether the given node supports hfp mic sr feature.
func (c *Cras) IsHfpMicSrSupported(ctx context.Context) (supported bool, err error) {
	err = c.call(ctx, "IsHfpMicSrSupported").Store(&supported)
	return supported, err
}

// VoiceIsolationPreferredEffect is the effect choice when voice isolation is enabled.
type VoiceIsolationPreferredEffect uint32

// EFFECT_TYPE from cras/common/src/types_internal.rs
const (
	VoiceIsolationEffectNoiseCancellation VoiceIsolationPreferredEffect = 1 << 0
	VoiceIsolationEffectStyleTransfer     VoiceIsolationPreferredEffect = 1 << 2
	VoiceIsolationEffectBeamforming       VoiceIsolationPreferredEffect = 1 << 3
)

// GetVoiceIsolationUIPreferredEffect gets the preferred effect for voice isolation.
func (c *Cras) GetVoiceIsolationUIPreferredEffect(ctx context.Context) (effect string, err error) {
	s2, err := c.dumpS2(ctx)
	return s2.Input.VoiceIsolationUIPreferredEffect, err
}

// SetVoiceIsolationUIPreferredEffect sets the preferred effect for voice isolation.
func (c *Cras) SetVoiceIsolationUIPreferredEffect(ctx context.Context, effect VoiceIsolationPreferredEffect) (err error) {
	return c.call(ctx, "SetVoiceIsolationUIPreferredEffect", effect).Err
}
