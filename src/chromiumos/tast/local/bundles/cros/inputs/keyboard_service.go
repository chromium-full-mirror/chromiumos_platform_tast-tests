// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/local/common"
	"chromiumos/tast/local/input"
	pb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var keyboardService KeyboardService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			keyboardService = KeyboardService{sharedObject: common.SharedObjectsForServiceSingleton, s: s}
			pb.RegisterKeyboardServiceServer(srv, &keyboardService)
		},
		GuaranteeCompatibility: true,
	})
}

// KeyboardService implements tast.cros.inputs.KeyboardService.
type KeyboardService struct {
	sharedObject *common.SharedObjectsForService
	s            *testing.ServiceState
	mutex        sync.Mutex
	kb           *input.KeyboardEventWriter
}

// Type injects key events suitable for generating the string s.
// Only characters that can be typed using a QWERTY keyboard are supported,
// and the current keyboard layout must be QWERTY. The left Shift key is automatically
// pressed and released for uppercase letters or other characters that can be typed
// using Shift.
func (svc *KeyboardService) Type(ctx context.Context, req *pb.TypeRequest) (*empty.Empty, error) {
	svc.mutex.Lock()
	defer svc.mutex.Unlock()

	if err := svc.initKeyboard(ctx); err != nil {
		return nil, err
	}

	if err := svc.kb.Type(ctx, req.Key); err != nil {
		return nil, errors.Wrapf(err, "failed to type %v", req.Key)
	}
	return &empty.Empty{}, nil
}

// Accel injects a sequence of key events simulating the accelerator (a.k.a. hotkey) described by s being typed.
// Accelerators are described as a sequence of '+'-separated, case-insensitive key characters or names.
// In addition to non-whitespace characters that are present on a QWERTY keyboard, the following key names may be used:
//
//	Modifiers:     "Ctrl", "Alt", "Search", "Shift"
//	Whitespace:    "Enter", "Space", "Tab", "Backspace"
//	Function keys: "F1", "F2", ..., "F12"
//
// "Shift" must be included for keys that are typed using Shift; for example, use "Ctrl+Shift+/" rather than "Ctrl+?".
func (svc *KeyboardService) Accel(ctx context.Context, req *pb.AccelRequest) (*empty.Empty, error) {
	svc.mutex.Lock()
	defer svc.mutex.Unlock()

	if err := svc.initKeyboard(ctx); err != nil {
		return nil, err
	}

	if err := svc.kb.Accel(ctx, req.Key); err != nil {
		return nil, errors.Wrapf(err, "failed to call Accel %v", req.Key)
	}
	return &empty.Empty{}, nil
}

// AccelPress injects a sequence of key events simulating pressing the accelerator (a.k.a. hotkey) described by s.
func (svc *KeyboardService) AccelPress(ctx context.Context, req *pb.AccelPressRequest) (*empty.Empty, error) {
	svc.mutex.Lock()
	defer svc.mutex.Unlock()

	if err := svc.initKeyboard(ctx); err != nil {
		return nil, err
	}

	if err := svc.kb.AccelPress(ctx, req.Key); err != nil {
		return nil, errors.Wrapf(err, "failed to call AccelPress %v", req.Key)
	}
	return &empty.Empty{}, nil
}

// AccelRelease injects a sequence of key events simulating release the accelerator (a.k.a. hotkey) described by s.
func (svc *KeyboardService) AccelRelease(ctx context.Context, req *pb.AccelReleaseRequest) (*empty.Empty, error) {
	svc.mutex.Lock()
	defer svc.mutex.Unlock()

	if err := svc.initKeyboard(ctx); err != nil {
		return nil, err
	}

	if err := svc.kb.AccelRelease(ctx, req.Key); err != nil {
		return nil, errors.Wrapf(err, "failed to call AccelRelease %v", req.Key)
	}
	return &empty.Empty{}, nil
}

func (svc *KeyboardService) initKeyboard(ctx context.Context) error {
	if svc.kb == nil {
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get keyboard handle")
		}
		svc.kb = kb

		// Ensure that the keyboard is closed when the service is shut down.
		go func(ctx context.Context) {
			select {
			case <-ctx.Done():
				svc.closeKeyboard(ctx)
			}
		}(svc.s.ServiceContext())
	}
	return nil
}

func (svc *KeyboardService) closeKeyboard(ctx context.Context) error {
	if svc.kb != nil {
		if err := svc.kb.Close(ctx); err != nil {
			return errors.Wrap(err, "failed to close keyboard handle")
		}
		svc.kb = nil
	}
	return nil
}
