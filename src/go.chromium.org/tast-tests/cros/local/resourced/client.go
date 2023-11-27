// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package resourced

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	rmpb "chromiumos/system_api/resource_manager_proto"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dbusInterface = "org.chromium.ResourceManager"
	dbusPath      = "/org/chromium/ResourceManager"
	dbusService   = "org.chromium.ResourceManager"

	// reseresourcedConnectTimeout limits how long we wait for a connection.
	resourcedConnectTimeout = 25 * time.Second

	// GameModeOff means no component managed by Resource Manager is in game
	// mode.
	GameModeOff = 0
	// GameModeBorealis means borealis is in game mode.
	GameModeBorealis = 1

	// ChromePressureLevelNone means no reduction of memory in chrome is needed.
	ChromePressureLevelNone = 0
	// ChromePressureLevelModerate means Chrome is advised to free buffers that
	// are cheap to re-allocate and are not immediately needed.
	ChromePressureLevelModerate = 1
	// ChromePressureLevelCritical means Chrome is advised to free all possible
	// memory.
	ChromePressureLevelCritical = 2

	// RTCAudioActiveOff means RTCAudioActive is off.
	RTCAudioActiveOff uint8 = 0
	// RTCAudioActiveOn means RTCAudioActive is on.
	RTCAudioActiveOn uint8 = 1

	// FullscreenVideoInactive means full screen video is not active.
	FullscreenVideoInactive uint8 = 0
	// FullscreenVideoActive means full screen video is active.
	FullscreenVideoActive uint8 = 1

	// QoSProcessNormal means Normal qos state for processes.
	QoSProcessNormal uint8 = 0
	// QoSProcessBackground means Background qos state for processes.
	QoSProcessBackground uint8 = 1

	// QoSThreadUrgentBursty means UrgentBursty qos state for threads.
	QoSThreadUrgentBursty uint8 = 0
	// QoSThreadUrgent means Urgent qos state for threads.
	QoSThreadUrgent uint8 = 1
	// QoSThreadBalanced means Balanced qos state for threads.
	QoSThreadBalanced uint8 = 2
	// QoSThreadEco means Eco qos state for threads.
	QoSThreadEco uint8 = 3
	// QoSThreadUtility means Utility qos state for threads.
	QoSThreadUtility uint8 = 4
	// QoSThreadBackground means Background qos state for threads.
	QoSThreadBackground uint8 = 5
)

// Client wraps D-Bus calls to make requests to the Resource Manager (resourced).
type Client struct {
	obj *dbusutil.DBusObject
}

// GameMode returns the result of the GetGameMode D-Bus method.
func (c *Client) GameMode(ctx context.Context) (uint8, error) {
	var result uint8
	if err := c.obj.Call(ctx, "GetGameMode").Store(&result); err != nil {
		return 0, errors.Wrap(err, "failed to call method GetGameMode")
	}
	return result, nil
}

// SetGameMode sets the game mode state in resourced.
func (c *Client) SetGameMode(ctx context.Context, mode uint8) error {
	if err := c.obj.Call(ctx, "SetGameMode", mode).Err; err != nil {
		return errors.Wrap(err, "failed to call method SetGameMode")
	}
	return nil
}

// SetGameModeWithTimeout sets the game mode state in resourced, the game mode will be reset after timeout seconds.
func (c *Client) SetGameModeWithTimeout(ctx context.Context, mode uint8, timeout uint32) error {
	if err := c.obj.Call(ctx, "SetGameModeWithTimeout", mode, timeout).Err; err != nil {
		return errors.Wrap(err, "failed to call method SetGameMode")
	}
	return nil
}

// AvailableMemoryKB returns the result of the GetAvailableMemoryKB D-Bus method.
func (c *Client) AvailableMemoryKB(ctx context.Context) (uint64, error) {
	var result uint64
	if err := c.obj.Call(ctx, "GetAvailableMemoryKB").Store(&result); err != nil {
		return 0, errors.Wrap(err, "failed to call method GetAvailableMemoryKB")
	}
	return result, nil
}

// ForegroundAvailableMemoryKB returns the result of the
// GetForegroundAvailableMemoryKB D-Bus method.
func (c *Client) ForegroundAvailableMemoryKB(ctx context.Context) (uint64, error) {
	var result uint64
	if err := c.obj.Call(ctx, "GetForegroundAvailableMemoryKB").Store(&result); err != nil {
		return 0, errors.Wrap(err, "failed to call method GetForegroundAvailableMemoryKB")
	}
	return result, nil
}

// Margins holds the memory margins returned from Resource Manager.
type Margins struct {
	ModerateKB, CriticalKB uint64
}

// MemoryMarginsKB returns the result of the GetMemoryMarginsKB D-Bus method.
func (c *Client) MemoryMarginsKB(ctx context.Context) (Margins, error) {
	var m Margins
	if err := c.obj.Call(ctx, "GetMemoryMarginsKB").Store(&m.CriticalKB, &m.ModerateKB); err != nil {
		return m, errors.Wrap(err, "failed to call method GetMemoryMarginsKB")
	}
	return m, nil
}

// ComponentMemoryMargins holds the component margins returned from Resource Manager.
type ComponentMemoryMargins struct {
	ChromeCriticalKB, ChromeModerateKB, ArcVMForegroundKB, ArcVMPerceptibleKB, ArcVMCachedKB uint64
}

// ComponentMemoryMarginsKB returns the result of the GetComponentMemoryMarginsKB D-Bus method.
func (c *Client) ComponentMemoryMarginsKB(ctx context.Context) (ComponentMemoryMargins, error) {
	var m ComponentMemoryMargins
	mapResult := make(map[string]uint64)
	if err := c.obj.Call(ctx, "GetComponentMemoryMarginsKB").Store(&mapResult); err != nil {
		return m, errors.Wrap(err, "failed to call method GetComponentMemoryMarginsKB")
	}

	m.ChromeCriticalKB = mapResult["ChromeCritical"]
	m.ChromeModerateKB = mapResult["ChromeModerate"]
	m.ArcVMForegroundKB = mapResult["ArcvmForeground"]
	m.ArcVMPerceptibleKB = mapResult["ArcvmPerceptible"]
	m.ArcVMCachedKB = mapResult["ArcvmCached"]

	return m, nil
}

// ChromePressureSignal holds values created from the MemoryPressureChrome D-Bus
// signal.
type ChromePressureSignal struct {
	Level uint8
	Delta uint64
}

func parseChromePressureSignal(sig *dbus.Signal) (ChromePressureSignal, error) {
	res := ChromePressureSignal{}
	if len(sig.Body) != 2 {
		return res, errors.Errorf("expected 2 params, got %d", len(sig.Body))
	}
	level, ok := sig.Body[0].(uint8)
	if !ok || level > ChromePressureLevelCritical {
		return res, errors.Errorf("unable to convert level from %v", sig.Body[0])
	}
	delta, ok := sig.Body[1].(uint64)
	if !ok {
		return res, errors.Errorf("unable to convert delta from %v", sig.Body[1])
	}
	res.Delta = delta
	res.Level = level
	return res, nil
}

// ChromePressureWatcher converts the MemoryPressureChrome D-Bus signal to a
// channel of ChromePressureSignal.
type ChromePressureWatcher struct {
	watcher *dbusutil.SignalWatcher
	err     error
	Signals chan ChromePressureSignal
}

// Close stops the MemoryPressureChrome D-Bus signal, and closes Signals, the
// channel of ChromePressureSignal.
func (pw *ChromePressureWatcher) Close(ctx context.Context) error {
	if err := pw.watcher.Close(ctx); err != nil {
		if pw.err == nil {
			return errors.Wrap(err, "failed to close SignalWatcher")
		}
		// Log the error, we will return the earlier error.
		testing.ContextLog(ctx, "Failed to close SignalWatcher after another failure: ", err)
	}
	return pw.err
}

// NewChromePressureWatcher starts listening for MemoryPressureChrome D-Bus
// signals. Call ChromePressureWatcher.Close when finished.
func (c *Client) NewChromePressureWatcher(ctx context.Context) (*ChromePressureWatcher, error) {
	pw := &ChromePressureWatcher{}
	var err error
	pw.watcher, err = dbusutil.NewSignalWatcherForSystemBus(ctx, dbusutil.MatchSpec{
		Type:      "signal",
		Path:      dbusPath,
		Interface: dbusInterface,
		Member:    "MemoryPressureChrome",
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to watch MemoryPressureChrome signal")
	}
	// Map D-Bus signals into ChromePressureSignal.
	pw.Signals = make(chan ChromePressureSignal)
	go func() {
		for dbusSig := range pw.watcher.Signals {
			pressureSig, err := parseChromePressureSignal(dbusSig)
			if err != nil {
				// NB: set err before close, so it will be set by the time a
				// consumer unblocks.
				pw.err = err
				close(pw.Signals)
				return
			}
			pw.Signals <- pressureSig
		}
		close(pw.Signals)
	}()
	return pw, nil
}

// RTCAudioActive returns the result of the GetRTCAudioActive D-Bus method.
func (c *Client) RTCAudioActive(ctx context.Context) (uint8, error) {
	var result uint8
	if err := c.obj.Call(ctx, "GetRTCAudioActive").Store(&result); err != nil {
		return 0, errors.Wrap(err, "failed to call method GetRTCAudioActive")
	}
	return result, nil
}

// SetRTCAudioActive sets the RTC audio active state in resourced.
func (c *Client) SetRTCAudioActive(ctx context.Context, mode uint8) error {
	if err := c.obj.Call(ctx, "SetRTCAudioActive", mode).Err; err != nil {
		return errors.Wrap(err, "failed to call method SetRTCAudioActive")
	}
	return nil
}

// FullscreenVideo returns the result of the GetFullscreenVideo D-Bus method.
func (c *Client) FullscreenVideo(ctx context.Context) (uint8, error) {
	var result uint8
	if err := c.obj.Call(ctx, "GetFullscreenVideo").Store(&result); err != nil {
		return 0, errors.Wrap(err, "failed to call method GetFullscreenVideo")
	}
	return result, nil
}

// SetFullscreenVideoWithTimeout sets the full screen video state in resourced, the state will be reset after timeout seconds.
func (c *Client) SetFullscreenVideoWithTimeout(ctx context.Context, fullscreenVideo uint8, timeout uint32) error {
	if err := c.obj.Call(ctx, "SetFullscreenVideoWithTimeout", fullscreenVideo, timeout).Err; err != nil {
		return errors.Wrap(err, "failed to call method SetFullscreenVideoWithTimeout")
	}
	return nil
}

// PowerSupplyChange notifies resourced to update the power preference.
func (c *Client) PowerSupplyChange(ctx context.Context) error {
	if err := c.obj.Call(ctx, "PowerSupplyChange").Err; err != nil {
		return errors.Wrap(err, "failed to call method PowerSupplyChange")
	}
	return nil
}

// SetMemoryMarginsBps sets the memory margins measured in basis points.
func (c *Client) SetMemoryMarginsBps(ctx context.Context, critical, moderate uint32) error {
	if err := c.obj.Call(ctx, "SetMemoryMarginsBps", critical, moderate).Err; err != nil {
		return errors.Wrap(err, "failed to call method SetMemoryMarginsBps")
	}
	return nil
}

// SetProcessState sets the qos state for a process.
func (c *Client) SetProcessState(ctx context.Context, pid uint32, state uint8) error {
	if err := c.obj.Call(ctx, "SetProcessState", pid, state).Err; err != nil {
		return errors.Wrap(err, "failed to call method SetProcessState")
	}
	return nil
}

// SetThreadState sets the qos state for a thread.
func (c *Client) SetThreadState(ctx context.Context, pid, tid uint32, state uint8) error {
	if err := c.obj.Call(ctx, "SetThreadState", pid, tid, state).Err; err != nil {
		return errors.Wrap(err, "failed to call method SetThreadState")
	}
	return nil
}

// ReportBackgroundProcesses reports the list of the background processes of a component (Ash or Lacros).
func (c *Client) ReportBackgroundProcesses(ctx context.Context, component rmpb.ReportBackgroundProcesses_Component, pids []int32) error {
	if err := dbusutil.CallProtoMethod(ctx, c.obj.Obj(), dbusInterface+".ReportBackgroundProcesses", &rmpb.ReportBackgroundProcesses{Component: component, Pids: pids}, nil); err != nil {
		return errors.Wrap(err, "failed to call method ReportBackgroundProcesses")
	}
	return nil
}

// NewClient makes a new D-Bus wrapper object for communicating with Resource
// Manager.
func NewClient(ctx context.Context) (*Client, error) {
	connectCtx, cancel := context.WithTimeout(ctx, resourcedConnectTimeout)
	defer cancel()
	obj, err := dbusutil.NewDBusObject(connectCtx, dbusService, dbusInterface, dbusPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Resource Manager")
	}
	return &Client{obj}, nil
}
