// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dbusutil provides additional functionality on top of the godbus/dbus package.
package dbusutil

import (
	"container/list"
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// The CalledMethod struct represents a method call and arguments that was observed by DbusEventMonitor.
type CalledMethod struct {
	MethodName string
	Arguments  []interface{}
}

// ErrEventWatcherClosed is returned for EventWatcher.Err() when the EventWatcher is closed by EventWatcher.Close().
var ErrEventWatcherClosed = errors.New("EventWatcher closed")

// EventWatcher watches for events in the system bus.
// Called methods are returned via Events().
type EventWatcher struct {
	// Channel for matched events.
	ch chan *CalledMethod
	// The context associated with the EventWatcher.
	ctx context.Context
	// The cancel function for ctx.
	cancel context.CancelCauseFunc
}

// Events returns a channel that emits matched events.
// Events is closed when either ew.Close is called or an error occurred.
func (ew *EventWatcher) Events() <-chan *CalledMethod {
	return ew.ch
}

// Close stops the event watcher.
// After Close is called, Events will be closed and Err will return EventWatcherClosed.
func (ew *EventWatcher) Close() {
	ew.cancel(ErrEventWatcherClosed)
}

// Err returns nil if w is still running, i.e. when Events is not closed.
// If Events is closed, Err returns a non-nil error explaining why.
func (ew *EventWatcher) Err() error {
	return context.Cause(ew.ctx)
}

// NewEventWatcher constructs an EventWatcher that matches D-Bus events that matches MatchSpec.
// The returned EventWatcher should be closed with EventWatcher.Close().
func NewEventWatcher(ctx context.Context, specs []MatchSpec) (*EventWatcher, error) {
	conn, err := SystemBusPrivate()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to system bus")
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "failed to authenticate the system bus")
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "failed to send the Hello call to the system bus")
	}

	var rules []string
	var allowlistDBusCmd []string
	for _, spec := range specs {
		rules = append(rules, spec.String())
		allowlistDBusCmd = append(allowlistDBusCmd, spec.Member)
	}
	call := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.Monitoring.BecomeMonitor", 0, rules, uint(0))
	if call.Err != nil {
		return nil, errors.Wrap(call.Err, "failed to become monitor")
	}

	ctx, cancel := context.WithCancelCause(ctx)
	ew := &EventWatcher{
		ch:     make(chan *CalledMethod),
		ctx:    ctx,
		cancel: cancel,
	}
	go func() {
		// Conn.Eavesdrop drops incoming messages if the given channel is full.
		// Set up an unbounded buffer so our monitoring is reliable.
		incoming := make(chan *dbus.Message, 10)
		buffer := list.New()
		conn.Eavesdrop(incoming)

		handleOneMsg := func(msg *dbus.Message) {
			dbusCmd, err := dbusCallMember(msg, allowlistDBusCmd)
			if err != nil {
				testing.ContextLog(ctx, "Ignoring error in dbusCallMember: ", err)
				return
			}
			buffer.PushBack(&CalledMethod{dbusCmd, msg.Body})
		}

		for {
			if front := buffer.Front(); front != nil {
				select {
				case <-ctx.Done():
					close(ew.ch)
					return
				case msg := <-incoming:
					handleOneMsg(msg)
				case ew.ch <- front.Value.(*CalledMethod):
					buffer.Remove(front)
				}
			} else {
				// Get one element.
				select {
				case <-ctx.Done():
					close(ew.ch)
					return
				case msg := <-incoming:
					handleOneMsg(msg)
				}
			}
		}
	}()
	return ew, nil
}

// DbusEventMonitor monitors the system message bus for the D-Bus calls we want to observe as specified in |specs|.
// It returns a stop function and error. The stop function stops the D-Bus monitor and return the called methods and/or error.
func DbusEventMonitor(ctx context.Context, specs []MatchSpec) (func() ([]CalledMethod, error), error) {
	// Fake signal to mark termination.
	const fakeEndSignal = "FakeEndSignal"

	specs = append(specs, MatchSpec{
		Type:      "signal",
		Interface: "com.fake",
		Member:    fakeEndSignal,
	})
	ew, err := NewEventWatcher(ctx, specs)
	if err != nil {
		return nil, errors.Wrap(err, "cannot set up NewEventWatcher")
	}

	stop := func() ([]CalledMethod, error) {
		// Send a fake dbus signal to stop the Eavesdrop.
		connect, err := SystemBus()
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to system bus")
		}
		if err := connect.Emit("/", fmt.Sprintf("com.fake.%s", fakeEndSignal)); err != nil {
			return nil, errors.Wrap(err, "failed sending fake signal to stop Eavesdrop")
		}

		var calledMethods []CalledMethod
		for e := range ew.Events() {
			if e.MethodName == fakeEndSignal {
				break
			}
			calledMethods = append(calledMethods, *e)
		}

		ew.Close()
		if err := ew.Err(); err != ErrEventWatcherClosed {
			return calledMethods, errors.Wrap(err, "unexpected ew.Err()")
		}
		return calledMethods, nil
	}

	return stop, nil
}

// dbusCallMember returns the member name of the D-Bus call.
func dbusCallMember(dbusMessage *dbus.Message, allowlistDBusCmd []string) (string, error) {
	v, ok := dbusMessage.Headers[dbus.FieldMember]
	if !ok {
		return "", errors.Errorf("failed dbus message doesn't have field member: %s", dbusMessage)
	}
	msg := fmt.Sprintf(v.String()[1 : len(v.String())-1])
	for _, cmd := range allowlistDBusCmd {
		if msg == cmd {
			return cmd, nil
		}
	}
	return "", errors.Errorf("failed found unexpected call: got %s, want %v", msg, allowlistDBusCmd)
}
