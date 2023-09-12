// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/errors"
)

// EffectState tells whether an effect is in use.
type EffectState int

// Effect states.
const (
	EffectUnavailable EffectState = iota
	EffectDisabled
	EffectEnabled
)

func (s EffectState) String() string {
	switch s {
	case EffectUnavailable:
		return "EffectUnavailable"
	case EffectDisabled:
		return "EffectDisabled"
	case EffectEnabled:
		return "EffectEnabled"
	default:
		return fmt.Sprintf("EffectState%d", int(s))
	}
}

func amixerCget(ctx context.Context, cardID, arg string) (EffectState, error) {
	bout, berr, err := testexec.CommandContext(ctx, "amixer", "-c"+cardID, "cget", arg).SeparatedOutput()
	stdout := string(bout)
	stderr := string(berr)
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok &&
			exitErr.ExitCode() == 1 &&
			strings.Contains(stderr, "Cannot find the given element from control") {
			return EffectUnavailable, nil
		}
		// Other exit codes are unexpected.
		return EffectUnavailable, errors.Wrapf(err, "cannot get %q; stderr: %q", arg, stderr)
	}
	if strings.Contains(stdout, ": values=off") {
		return EffectDisabled, nil
	}
	if strings.Contains(stdout, ": values=on") {
		return EffectEnabled, nil
	}
	return EffectUnavailable, errors.Errorf("cannot tell %q state from %q; stderr: %q", arg, stdout, stderr)
}

// DSPNoiseCancellationState tells whether DSP effects are active.
func DSPNoiseCancellationState(ctx context.Context) (EffectState, error) {
	cards, err := audio.GetSoundCards()
	if err != nil {
		return EffectUnavailable, errors.Wrap(err, "failed to GetSoundCards()")
	}

	hasCardID := func(id string) bool {
		for _, card := range cards {
			if card.ID == id {
				return true
			}
		}
		return false
	}

	// Control used by redrix and friends.
	if id := "sofrt5682"; hasCardID(id) {
		return amixerCget(ctx, id, "name='RTNR10.0 rtnr_enable_10'")
	}
	// Control used by dojo.
	if id := "sofm8195m983905"; hasCardID(id) {
		return amixerCget(ctx, id, "name='RTNR3.0 rtnr_enable_3'")
	}
	return EffectUnavailable, err
}

// CrasProcessingMonitor monitors the state of audio processing in CRAS.
type CrasProcessingMonitor struct {
	ctx        context.Context
	cancel     context.CancelCauseFunc
	snapshotCh chan CrasProcessingState
}

// CrasProcessingState tells the processing state.
type CrasProcessingState struct {
	// Tells whether any stream is using CRAS APM.
	CrasAPM EffectState
	// Tells whether any stream is using AP NC.
	APNC EffectState
}

// NewCrasProcessingMonitor returns a CrasProcessingState to monitor effects used in CRAS.
// It is only valid to start it when there are no audio streams.
// The caller should call Close() to close the monitor.
func NewCrasProcessingMonitor(ctx context.Context) (*CrasProcessingMonitor, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	cmd := exec.CommandContext(ctx, "croslog", "--identifier=cras_server", "--follow")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cannot create stdout pipe for croslog")
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "cannot start croslog")
	}

	monitor := &CrasProcessingMonitor{
		ctx:        ctx,
		cancel:     cancel,
		snapshotCh: make(chan CrasProcessingState),
	}
	updateCh := make(chan CrasProcessingState)

	// Help plumbing the snapshots.
	go func() {
		snapshot := CrasProcessingState{
			CrasAPM: EffectDisabled,
			APNC:    EffectDisabled,
		}

		for {
			select {
			case <-ctx.Done():
				close(monitor.snapshotCh)
				return
			case snapshot = <-updateCh:
			case monitor.snapshotCh <- snapshot:
			}
		}
	}()

	// Monitor croslog.
	go func() {
		err := func() error {
			effectCount := make(map[string]int)
			streamEffect := make(map[int]string)
			processorEventRE := regexp.MustCompile(`CrasProcessor #(\d+) (created|dropped)`)
			effectRE := regexp.MustCompile(`effect: (\w+)`)

			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				m := processorEventRE.FindStringSubmatch(scanner.Text())
				if m == nil {
					continue
				}
				id, _ := strconv.Atoi(m[1])
				switch m[2] {
				case "created":
					m := effectRE.FindStringSubmatch(scanner.Text())
					if m == nil {
						return errors.Errorf("unknown effect from %q", scanner.Text())
					}
					effectCount[m[1]]++
					streamEffect[id] = m[1]
				case "dropped":
					effectCount[streamEffect[id]]--
				default:
					panic("unexpected event from regexp match")
				}

				update := CrasProcessingState{
					CrasAPM: EffectDisabled,
					APNC:    EffectDisabled,
				}
				if effectCount["NoEffects"] > 0 {
					// An active CrasProcessor implies an active CRAS APM.
					update.CrasAPM = EffectEnabled
				}
				if effectCount["NoiseCancellation"] > 0 {
					update.CrasAPM = EffectEnabled
					update.APNC = EffectEnabled
				}
				updateCh <- update
			}
			if err := scanner.Err(); err != nil {
				return errors.Wrap(err, "cannot read from croslog stdout")
			}
			return cmd.Wait()
		}()
		if err != nil {
			monitor.cancel(err)
		}
		close(updateCh)
	}()

	return monitor, nil
}

// Snapshot returns the latest snapshot.
func (m *CrasProcessingMonitor) Snapshot(ctx context.Context) (CrasProcessingState, error) {
	select {
	case snap, ok := <-m.snapshotCh:
		if !ok {
			return CrasProcessingState{}, errors.Wrap(context.Cause(ctx), "cannot get latest snapshot, the monitor is possibly closed")
		}
		return snap, nil
	case <-ctx.Done():
		return CrasProcessingState{}, ctx.Err()
	}
}

// Close the monitor.
func (m *CrasProcessingMonitor) Close() error {
	m.cancel(nil)
	if err := context.Cause(m.ctx); err != context.Canceled {
		return err
	}
	return nil
}
