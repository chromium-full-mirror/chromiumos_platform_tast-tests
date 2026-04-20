// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gameapp

import (
	"context"
	"time"
)

const (
	defaultUITimeout = 15 * time.Second
	longUITimeout    = 30 * time.Second // Used for situations where UI response might be slow.
)

// GameApp contains user's operation in game application.
type GameApp interface {
	// Install installs the game app via play store.
	Install(ctx context.Context) error
	// Uninstall uninstalls the game app if it has been installed.
	// If Install is called, Uninstall needs to be called before the end of the case.
	Uninstall(ctx context.Context) error
	// Launch launches the game app.
	// It should be called after the game app is installed.
	Launch(ctx context.Context) error
	// IsLaunched returns whether the game app is launched.
	IsLaunched() bool
	// EnterGameScene enters the game scene.
	// It should be called after the game app is launched.
	EnterGameScene(ctx context.Context) error
	// Play plays the game for play time.
	// It should be called after entering the game scene.
	Play(ctx context.Context, playTime time.Duration) error
	// End closes the game app if it is launched.
	// If Lauch is called, End needs to be called before the end of the case.
	End(ctx context.Context) error
}
