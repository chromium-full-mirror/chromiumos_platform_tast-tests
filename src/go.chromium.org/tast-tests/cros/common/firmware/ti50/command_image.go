// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// promptSuccesses is the number of consecutive successful prompts for
	// the image to be considered fully booted.
	promptSuccesses = 3
)

var (
	normalSleep *regexp.Regexp = regexp.MustCompile(`Entering normal sleep`)
	deepSleep   *regexp.Regexp = regexp.MustCompile(`Entering deep sleep zzz`)
	anySleep    *regexp.Regexp = regexp.MustCompile(`Entering (deep|normal) sleep( zzz)?`)
	roBoot      *regexp.Regexp = regexp.MustCompile(`(Starting ROM_EXT|Ravn4\|)`)

	// FatalMsg is a regular expression.  If it ever matches any serial console output from
	// the GSC, it should be reported as a test failure.
	FatalMsg *regexp.Regexp = regexp.MustCompile(
		`(Kernel panicked|WATCHDOG RESET IMMINENT|app exit|app panic|FIXME).*\n`)
)

// CommandImage displays a prompt and responds to cli commands.
type CommandImage struct {
	SerialChannel
	promptCmd       string
	promptPattern   string
	promptPatternRe *regexp.Regexp
}

// OpenCommandImage creates a new CommandImage. Typical examples of promptCmd and
// promptPattern are "\n" and "> " respectively.
func OpenCommandImage(ctx context.Context, console SerialChannel, promptCmd, promptPattern string) (*CommandImage, error) {
	// Ensure that the board is open with the correct context (b/298714011)
	if err := console.Open(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to open image console")
	}
	return &CommandImage{console, promptCmd, promptPattern, regexp.MustCompile(promptPattern)}, nil
}

// RawCommand sends a command to the image and waits for the regex to be matched
// before returning the captured groups.  It does not append the promptCmd as
// opposed to Command.
func (i *CommandImage) RawCommand(ctx context.Context, rawCmd string, re *regexp.Regexp) ([]string, error) {
	if err := i.WriteSerial(ctx, []byte(rawCmd)); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	whichRegularExpression, match, err := i.ReadSerialSubmatch(ctx, FatalMsg, re)
	if err != nil {
		return nil, err
	}
	if whichRegularExpression == 0 {
		// GSC printed a fatal message, report error to caller.
		return nil, errors.New(strings.TrimRight(string(match[0]), "\r\n"))
	}
	ret := make([]string, 0)
	for _, m := range match {
		ret = append(ret, string(m))
	}
	return ret, nil
}

// Command sends the command to the image and returns the output from after the
// command up to the next prompt.
func (i *CommandImage) Command(ctx context.Context, cmd string) (string, error) {
	rawCmd := cmd + i.promptCmd
	// (?s) for dot matches newline, for multiple lines of output within (.*)
	// (?m) for ^ matches start of each line, for ^ in promptPattern.
	matches, err := i.RawCommand(ctx, rawCmd, regexp.MustCompile("(?sm)"+regexp.QuoteMeta(cmd)+"(.*)"+i.promptPattern))
	if err != nil {
		return "", err
	}
	return matches[1], nil
}

// WaitUntilBooted by checking that prompts are consistently displayed.
func (i *CommandImage) WaitUntilBooted(ctx context.Context, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, interval)
	defer cancel()
	j := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		success, err := i.getPrompt(ctx)
		if err != nil {
			return err
		}
		if !success {
			continue
		}
		j++
		if j == promptSuccesses {
			return nil
		}
	}
}

// WaitUntilMatch waits until specified match is present
func (i *CommandImage) WaitUntilMatch(ctx context.Context, re *regexp.Regexp, interval time.Duration) (output [][]byte, err error) {
	pOpts := testing.PollOptions{Timeout: interval}
	err = testing.Poll(ctx, func(ctx context.Context) error {
		whichRegularExpression, match, err := i.ReadSerialSubmatch(ctx, FatalMsg, re)
		if err != nil {
			return err
		}
		if whichRegularExpression == 0 {
			// GSC printed a fatal message, report error to caller.
			return errors.New(strings.TrimRight(string(match[0]), "\r\n"))
		}
		output = match
		return nil
	}, &pOpts)
	return output, err
}

// getPrompt gets a fresh prompt from the image by the prompt.  Returns `true` if prompt was
// detected, `false` on timeout without any prompt, and sets `err` in case of fatal errors.
func (i *CommandImage) getPrompt(ctx context.Context) (success bool, err error) {
	if err := i.ClearInput(ctx); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err = i.Command(ctx, "")
	if err == nil {
		return true, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// This is an "expected" error, indicating that the GSC is simply not yet
		// responsive.  This will be reported through the first return value, reserving
		// `err` for truly unexpected cases, such as GSC panic, or HyperDebug malfunction,
		// either of which should result in the test immediately aborting.
		return false, nil
	}
	return false, err
}
