// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/android/adb"
)

// RegexpPred returns a function to be passed to WaitForLogcat that returns true if a given regexp is matched in that line.
func RegexpPred(exp *regexp.Regexp) func(string) bool {
	return adb.RegexpPred(exp)
}

// WaitForLogcat keeps scanning logcat. The function pred is called on the logcat contents line by line. This function returns successfully if pred returns true. If pred never returns true, this function returns an error as soon as the context is done.
// An optional quitFunc will be polled at regular interval, which can be used to break early if for example the activity which is supposed to print the exp has crashed.
func (a *ARC) WaitForLogcat(ctx context.Context, pred func(string) bool, quitFunc ...func() bool) error {
	_, err := a.device.WaitForLogcat(ctx, pred, "", quitFunc...)
	return err
}

// WaitForLogcatLine keeps scanning logcat. The function pred is called on the logcat contents line by line. This function returns the candidate line successfully if pred returns true. If pred never returns true, this function returns an error as soon as the context is done.
// An optional quitFunc will be polled at regular interval, which can be used to break early if for example the activity which is supposed to print the exp has crashed.
func (a *ARC) WaitForLogcatLine(ctx context.Context, pred func(string) bool, quitFunc ...func() bool) (string, error) {
	return a.device.WaitForLogcat(ctx, pred, "", quitFunc...)
}

// WaitForLogcatSince keeps scanning logcat, starting at a specified time.
// The function pred is called on the logcat contents line by line. This function returns successfully if pred returns true. If pred never returns true, this function returns an error as soon as the context is done.
// An optional quitFunc will be polled at regular interval, which can be used to break early if for example the activity which is supposed to print the exp has crashed.
func (a *ARC) WaitForLogcatSince(ctx context.Context, pred func(string) bool, since adb.LogcatTimestampLong, quitFunc ...func() bool) error {
	_, err := a.device.WaitForLogcat(ctx, pred, since, quitFunc...)
	return err
}

// OutputLogcatGrep greps logcat with the given string and returns the output.
func (a *ARC) OutputLogcatGrep(ctx context.Context, grepArg string) ([]byte, error) {
	return a.device.OutputLogcatGrep(ctx, grepArg)
}

// ClearLogcat clears all logcat buffers.
func (a *ARC) ClearLogcat(ctx context.Context) error {
	return a.device.ClearLogcat(ctx)
}

// LogcatDeviceTime returns a logcat formatted timestamp of the current device
// time.
func (a *ARC) LogcatDeviceTime(ctx context.Context) (adb.LogcatTimestampLong, error) {
	return a.device.LogcatDeviceTime(ctx)
}

// DumpLogcat writes logcat dump to specified filePath.
func (a *ARC) DumpLogcat(ctx context.Context, filePath string) (error) {
	return a.device.DumpLogcat(ctx, filePath)
}
