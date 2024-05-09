// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"regexp"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	biometricsManagerName  = "CrosFpBiometricsManager"
	biodClientToolLogReg   = `\[biod_client_tool.cc\([0-9]+\)\] `
	biodClientMatchSuccess = "Authentication: Success"
	biodClientMatchLog     = "Authentication:"
)

// BiodClient is used to interact with biod through the biod_client_tool CLI tool.
type BiodClient struct {
	runner CmdRunner
}

// NewBiodClient is a factory function to create a BiodClient instance.
func NewBiodClient(r CmdRunner) *BiodClient {
	return &BiodClient{r}
}

func (c *BiodClient) runWithStreamingStderr(ctx context.Context, onStderrLine StderrLineFunc, args ...string) error {
	return c.runner.RunWithInteractiveStderr(ctx, onStderrLine, "biod_client_tool", args...)
}

// Enroll calls "biod_client_tool enroll CrosFpBiometricsManager <user> <label>".
func (c *BiodClient) Enroll(ctx context.Context, sanitizedUsername, label string) error {
	args := []string{"enroll", biometricsManagerName, sanitizedUsername, label}
	re := regexp.MustCompile(biodClientToolLogReg)
	// For enrollment, we currently don't need explicit control or set expectation of the flow.
	// Let's just print every line of log from biod_client_tool and let the user interact with
	// the CLI.
	onStderrLine := func(line []byte) error {
		split := re.Split(string(line), -1)
		if len(split) > 1 {
			testing.ContextLog(ctx, split[len(split)-1])
		}
		return nil
	}
	return c.runWithStreamingStderr(ctx, onStderrLine, args...)
}

// MatchExpectSuccess calls "biod_client_tool authenticate CrosFpBiometricsManager",
// and expects successful result. Due to the nature of fingerprint matching, the result
// won't be 100% success even if the correct finger is pressed. Therefore, allow |maxAttempts|
// attempts before we return an error due to match failure.
func (c *BiodClient) MatchExpectSuccess(ctx context.Context, maxAttempts int) error {
	args := []string{"authenticate", biometricsManagerName}
	count := 0
	matched := false
	re := regexp.MustCompile(biodClientToolLogReg)
	onStderrLine := func(line []byte) error {
		split := re.Split(string(line), -1)
		if len(split) <= 1 {
			return nil
		}
		log := split[len(split)-1]
		testing.ContextLog(ctx, log)
		if log == biodClientMatchSuccess {
			matched = true
			// The biod_client CLI doesn't exit after successful match. It expects the client to
			// shut it down. Return an error here and use |matched| to determine whether the command
			// should be treated as successful later.
			return errors.New("successfully matched fingerprint")
		} else if strings.HasPrefix(log, biodClientMatchLog) {
			count++
		}
		if count == maxAttempts {
			return errors.Errorf("exceeded max failure attempts: %d", maxAttempts)
		}
		return nil
	}
	err := c.runWithStreamingStderr(ctx, onStderrLine, args...)
	if matched {
		return nil
	}
	return err
}

// MatchExpectFailure calls "biod_client_tool authenticate CrosFpBiometricsManager",
// and expects failure. Due to the nature of fingerprint matching, the result might
// be successful even if the wrong finger is pressed. However, the probablility should
// be really low, such that we can treat 1 successful match as failing the expectation.
func (c *BiodClient) MatchExpectFailure(ctx context.Context) error {
	args := []string{"authenticate", biometricsManagerName}
	matchPerformed := false
	matchFailed := false
	re := regexp.MustCompile(biodClientToolLogReg)
	onStderrLine := func(line []byte) error {
		split := re.Split(string(line), -1)
		if len(split) <= 1 {
			return nil
		}
		log := split[len(split)-1]
		testing.ContextLog(ctx, log)
		if strings.HasPrefix(log, biodClientMatchLog) {
			matchPerformed = true
			matchFailed = (log != biodClientMatchSuccess)
			// The biod_client CLI doesn't exit after successful match. It expects the client to
			// shut it down. Return an error here and use |matchPerformed| and |matchFailed| to
			// determine whether the match succeeded, failed, or encountered other internal errors.
			return errors.New("performed 1 fingerprint match")
		}
		return nil
	}
	err := c.runWithStreamingStderr(ctx, onStderrLine, args...)
	if matchFailed {
		return nil
	}
	// Some other failures occurred.
	if !matchPerformed {
		return err
	}
	// The match somehow succeeded, which is unexpected.
	return errors.New("fingerprint match succeeded unexpectedly")
}
