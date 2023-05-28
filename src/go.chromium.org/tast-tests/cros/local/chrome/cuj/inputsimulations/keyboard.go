// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputsimulations

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// RepeatKeyPressFor presses the specified key repeatedly, with a |delay| between key presses.
func RepeatKeyPressFor(ctx context.Context, kw *input.KeyboardEventWriter, key string, delay, duration time.Duration) error {
	return runActionFor(ctx, duration, action.Combine(
		"press ["+key+"] and sleep",
		kw.AccelAction(key),
		action.Sleep(delay)))
}

// RepeatKeyPress presses the specified key |n| times, with a |delay| between key presses.
func RepeatKeyPress(ctx context.Context, kw *input.KeyboardEventWriter, key string, delay time.Duration, n int) error {
	return uiauto.Repeat(
		n,
		action.Combine("press "+key+" and sleep",
			kw.AccelAction(key),
			action.Sleep(delay),
		),
	)(ctx)
}

// RepeatKeyHold holds a key down for |keyHoldDuration| |n| times,
// with a |delay| between each key release and the next key press.
func RepeatKeyHold(ctx context.Context, kw *input.KeyboardEventWriter, key string, keyHoldDuration, delay time.Duration, n int) error {
	return uiauto.Repeat(
		n,
		action.Combine("press and hold "+key+" and sleep",
			kw.AccelPressAction(key),
			action.Sleep(keyHoldDuration),
			kw.AccelReleaseAction(key),
			action.Sleep(delay),
		),
	)(ctx)
}

// TypeSequenceWPMAction types the sequence of characters at the
// provided WPM (words per minute). WPM is defined as the total number
// of typable characters divided by 5.
//
// |wpm| cannot be >= 120. kw.TypeAction at minimum takes 100ms,
// because the type function alone contains two calls to sleep for 50ms
// each. If we were to type at 120 WPM, we would have to sleep between
// presses for at most 100ms (i.e, 120 words, or 600 characters, in a
// single minute, would be a character every 100ms). Therefore, any
// higher WPM will simply cap out at 120 WPM anyway.
func TypeSequenceWPMAction(ctx context.Context, kw *input.KeyboardEventWriter, wpm int, seq []string) action.Action {
	if wpm > 120 {
		testing.ContextLogf(ctx, "%d WPM is too high, will cap out at 120 WPM", wpm)
	}

	// totalSleep is the length of time that has to pass from first
	// pressing one key to pressing another key, to properly type at
	// |wpm| words per minute. This is derived by the following:
	// WPM = len(typeable characters) / 5
	totalSleep := time.Minute / time.Duration(wpm*5)

	// Use a separate getter function due to how Go handles closures
	// within loops. If we directly made an action.Action that used
	// the loop variable |key| within the closure, the actual value for
	// |key| is ambiguous when evaluating the action. |key| looks at a
	// memory location, and that location is the same for every
	// iteration in the loop, making it hard to use within the closure.
	getKeyPressAction := func(key string) action.Action {
		return func(ctx context.Context) error {
			start := time.Now()
			if err := kw.Type(ctx, key); err != nil {
				return errors.Wrapf(err, "failed to type %s", key)
			}
			return testing.Sleep(ctx, totalSleep-time.Since(start))
		}
	}

	var actions []action.Action
	for _, key := range seq {
		actions = append(actions, getKeyPressAction(key))
	}
	return action.Combine("type sequence", actions...)
}
