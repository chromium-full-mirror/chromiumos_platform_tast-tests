// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Fusebox,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Mount fusebox daemon and verify it responds to requests",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@chromium.org",
			"nigeltao@chromium.org",
			"noel@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "basic",
			Val:  false,
		}, {
			Name: "advanced",
			Val:  true,
		}},
	})
}

func Fusebox(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Logging into Chrome should launch Fusebox (via cros-disks).
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}

	// Poll until the "fuse_status" file shows up. The "fuse_status" and "ok\n"
	// magic strings are defined in "platform2/fusebox/built_in.cc".
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		const fuseStatusFilename = "/media/fuse/fusebox/built_in/fuse_status"
		if got, err := os.ReadFile(fuseStatusFilename); err != nil {
			return err
		} else if want := "ok\n"; string(got) != want {
			return testing.PollBreak(errors.Errorf("got %q, want %q", got, want))
		}
		return nil
	}, nil); err != nil {
		s.Fatal("ReadFile(fuse_status) failed: ", err)
	}

	// Make a temporary directory.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}
	tdd := tempDirData{}
	if err := tconn.Call(ctx, &tdd, `tast.promisify(chrome.autotestPrivate.makeFuseboxTempDir)`); err != nil {
		s.Fatal("makeFuseboxTempDir failed: ", err)
	} else if tdd.FuseboxFilePath == "" {
		s.Fatal("FuseboxFilePath is empty")
	}
	defer tconn.Call(cleanupCtx, nil, `chrome.autotestPrivate.removeFuseboxTempDir`, tdd.FuseboxFilePath)

	// That temporary directory has two names: a fusebox one at
	// "/media/fuse/fusebox/tmp.foo" and an underlying one at "/tmp/.foo".
	// Creating "wfru.txt" in the first should be visible in the second and
	// vice versa for "wurf.txt".
	checkFuseboxRoundTrip(s, tdd.FuseboxFilePath, tdd.UnderlyingFilePath,
		"wfru.txt", "write fusebox; read underlying")
	checkFuseboxRoundTrip(s, tdd.UnderlyingFilePath, tdd.FuseboxFilePath,
		"wurf.txt", "write underlying; read fusebox")

	// The checkFuseboxRoundTrip calls above exercised basic read-a-file and
	// write-a-file functionality. Now exercise something more advanced.
	if advanced := s.Param().(bool); advanced {
		exerciseAdvancedFuseboxIO(s, tdd.FuseboxFilePath, "wfru.txt", "wurf.txt")
	}
}

func checkFuseboxRoundTrip(s *testing.State, writeFilePath, readFilePath, baseName, data string) {
	writeFilename := filepath.Join(writeFilePath, baseName)
	readFilename := filepath.Join(readFilePath, baseName)
	if err := os.WriteFile(writeFilename, []byte(data), 0777); err != nil {
		s.Fatalf("WriteFile(%q) failed: %v", writeFilename, err)
	} else if got, err := os.ReadFile(readFilename); err != nil {
		s.Fatalf("ReadFile(%q) failed: %v", readFilename, err)
	} else if string(got) != data {
		s.Fatalf("ReadFile(%q): got %q, want %q", readFilename, got, data)
	}
}

type tempDirData struct {
	FuseboxFilePath    string `json:"fuseboxFilePath"`
	UnderlyingFilePath string `json:"underlyingFilePath"`
}

func exerciseAdvancedFuseboxIO(s *testing.State, fuseboxFilePath, filename0, filename1 string) {
	// Run a bunch of commands that are roughly analogous to classic Unix
	// tools: ls, cat, mkdir, etc.
	//
	// Each "FFP" will be replaced by the fuseboxFilePath.
	commands := [][]string{
		{"ls", "FFP"},
		{"touch", "FFP/file"},
		{"cp", "FFP/" + filename0, "FFP/copy"},
		{"cat", "FFP/copy"},
		{"cp", "FFP/" + filename1, "FFP/copy"},
		{"ls", "FFP"},
		{"mkdir", "FFP/d0"},
		{"mkdir", "FFP/d0/d1"},
		{"cp", "FFP/copy", "FFP/d0/anotherCopy"},
		{"ls", "FFP/d0/anotherCopy"},
		{"mv", "FFP/file", "FFP/move"},
		{"ls", "FFP"},
		{"cat", "FFP/d0/anotherCopy"},
		{"rm -rf", "FFP/copy", "FFP/d0", "FFP/move"},
	}

	for _, command := range commands {
		// arg returns command[i], replacing a leading "FFP" with the
		// fuseboxFilePath.
		arg := func(i int) string {
			rawArg := command[i]
			if strings.HasPrefix(rawArg, "FFP") {
				return fuseboxFilePath + rawArg[3:]
			}
			return rawArg
		}

		switch command[0] {
		case "cat":
			if err := catFile(arg(1)); err != nil {
				s.Fatalf("exercise %q: catFile: %v", command, err)
			}

		case "cp":
			if err := copyFile(arg(1), arg(2)); err != nil {
				s.Fatalf("exercise %q: copyFile: %v", command, err)
			}

		case "ls":
			if info, err := os.Stat(arg(1)); err != nil {
				s.Fatalf("exercise %q: Stat: %v", command, err)
			} else if !info.IsDir() {
				// No-op.
			} else if _, err := os.ReadDir(arg(1)); err != nil {
				s.Fatalf("exercise %q: ReadDir: %v", command, err)
			}

		case "mkdir":
			if err := os.Mkdir(arg(1), 0777); err != nil {
				s.Fatalf("exercise %q: Mkdir: %v", command, err)
			}

		case "mv":
			if err := renameFile(arg(1), arg(2)); err != nil {
				s.Fatalf("exercise %q: renameFile: %v", command, err)
			}

		case "rm -rf":
			for i := 1; i < len(command); i++ {
				if err := os.RemoveAll(arg(i)); err != nil {
					s.Fatalf("exercise %q: RemoveAll, i=%d: %v", command, i, err)
				}
			}

		case "touch":
			if f, err := os.Create(arg(1)); err != nil {
				s.Fatalf("exercise %q: Create: %v", command, err)
			} else if err = f.Close(); err != nil {
				s.Fatalf("exercise %q: Close: %v", command, err)
			}

		default:
			s.Fatalf("exercise %q: unrecognized command", command)
		}
	}
}

// catFile is a rough approximation to running /usr/bin/cat on a Fusebox file.
// "Rough approximation" means that it just reads (and discards) a file's
// contents. Unlike /usr/bin/cat, it does not write those contents to stdout.
func catFile(srcName string) error {
	src, err := os.Open(srcName)
	if err != nil {
		return err
	}
	_, cErr := io.Copy(io.Discard, src)
	sErr := src.Close()
	if cErr != nil {
		return cErr
	}
	return sErr
}

func copyFile(srcName, dstName string) error {
	dst, err := os.Create(dstName)
	if err != nil {
		return err
	}
	src, err := os.Open(srcName)
	if err != nil {
		dst.Close()
		return err
	}
	_, cErr := io.Copy(dst, src)
	sErr := src.Close()
	dErr := dst.Close()
	if cErr != nil {
		return cErr
	} else if sErr != nil {
		return sErr
	}
	return dErr
}

// renameFile is a rough approximation to running /usr/bin/mv on a Fusebox file.
//
// TODO(b/255520194): this should just be a call to os.Rename, once the Fusebox
// client and server support that. Until then, fake it as a copy and delete.
func renameFile(srcName, dstName string) error {
	if err := copyFile(srcName, dstName); err != nil {
		return err
	}
	return os.Remove(srcName)
}
