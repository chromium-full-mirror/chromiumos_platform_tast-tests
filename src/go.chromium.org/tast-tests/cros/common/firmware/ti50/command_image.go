// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"regexp"
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
	roBoot      *regexp.Regexp = regexp.MustCompile(`Ravn4\|`)
)

// CommandImage displays a prompt and responds to cli commands.
type CommandImage struct {
	board           DevBoard
	promptCmd       string
	promptPattern   string
	promptPatternRe *regexp.Regexp
}

// NewCommandImage creates a new CommandImage. Typical examples of promptCmd and
// promptPattern are "\n" and "> " respectively.
func NewCommandImage(ctx context.Context, board DevBoard, promptCmd, promptPattern string) (*CommandImage, error) {
	// Ensure that the board is open with the correct context (b/298714011)
	if err := board.Open(ctx); err != nil {
		return nil, errors.Wrap(err, "failed NewCommandImage board open")
	}
	return &CommandImage{board, promptCmd, promptPattern, regexp.MustCompile(promptPattern)}, nil
}

// RawCommand sends a command to the image and waits for the regex to be matched
// before returning the captured groups.  It does not append the promptCmd as
// opposed to Command.
func (i *CommandImage) RawCommand(ctx context.Context, rawCmd string, re *regexp.Regexp) ([]string, error) {
	if err := i.board.WriteSerial(ctx, []byte(rawCmd)); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	match, err := i.board.ReadSerialSubmatch(ctx, re)
	if err != nil {
		return nil, err
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
	for {
		ctxInt, cancel := context.WithTimeout(ctx, interval)
		defer cancel()
		j := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := i.GetPrompt(ctx); err != nil {
				break
			}
			j++
			if j == promptSuccesses {
				return nil
			}
		}
		select {
		case <-ctxInt.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// SendConsoleRebootCmd issues the reboot command but does not listen for a response since the GSC
// is expected to reboot. Note that this does not detect if reboot was not performed because
// CCD wasn't open.
func (i *CommandImage) SendConsoleRebootCmd(ctx context.Context) error {
	if err := i.board.WriteSerial(ctx, []byte("reboot")); err != nil {
		return err
	}
	return nil
}

// WaitUntilNormalSleep waits until gsc goes into deep sleep via monitoring print statement
func (i *CommandImage) WaitUntilNormalSleep(ctx context.Context, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, normalSleep)
		return err
	}, &pOpts)
}

// WaitUntilDeepSleep waits until gsc goes into deep sleep via monitoring print statement
func (i *CommandImage) WaitUntilDeepSleep(ctx context.Context, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, deepSleep)
		return err
	}, &pOpts)
}

// WaitUntilAnySleep waits until gsc goes into deep or normal sleep via monitoring print statement
func (i *CommandImage) WaitUntilAnySleep(ctx context.Context, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, anySleep)
		return err
	}, &pOpts)
}

// WaitUntilRoBoot waits until initial RO console messages are printed which happens right after
// reboot or deep sleep resume.
func (i *CommandImage) WaitUntilRoBoot(ctx context.Context, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, roBoot)
		return err
	}, &pOpts)
}

// WaitUntilMatch waits until specified match is present
func (i *CommandImage) WaitUntilMatch(ctx context.Context, re *regexp.Regexp, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, re)
		return err
	}, &pOpts)
}

// GetPrompt gets a fresh prompt from the image by  the prompt.
func (i *CommandImage) GetPrompt(ctx context.Context) error {
	if err := i.board.ClearInput(ctx); err != nil {
		return err
	}
	_, err := i.Command(ctx, "")
	return err
}
