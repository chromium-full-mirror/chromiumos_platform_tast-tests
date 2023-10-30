// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"crypto/md5"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/filemanager"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
		},
		Attr: []string{
			"group:mainline",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:      "advanced",
			Val:       "advancedVal",
			ExtraAttr: []string{"informational"},
		}, {
			Name: "basic",
			Val:  "basicVal",
		}, {
			Name: "fsp", // FSP = the fileSystemProvider API.
			Val:  "fspVal",
			ExtraData: []string{
				"fusebox_fsp_extension/manifest.json",
				"fusebox_fsp_extension/service-worker.js",
			},
		}},
	})
}

func Fusebox(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Logging into Chrome should launch Fusebox (via cros-disks).
	chromeOpts := []chrome.Option(nil)
	switch s.Param().(string) {
	case "fspVal":
		chromeOpts = []chrome.Option{
			chrome.UnpackedExtension(filepath.Dir(s.DataPath(
				"fusebox_fsp_extension/manifest.json"))),
		}
	}
	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}

	// Poll until the "fuse_status" file shows up. The "fuse_status" and "ok\n"
	// magic strings are defined in "platform2/fusebox/built_in.cc".
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		fuseStatusFilename := filepath.Join(filemanager.FuseboxDirPath, "built_in/fuse_status")
		if got, err := os.ReadFile(fuseStatusFilename); err != nil {
			return err
		} else if want := "ok\n"; string(got) != want {
			return testing.PollBreak(errors.Errorf("got %q, want %q", got, want))
		}
		return nil
	}, nil); err != nil {
		s.Fatal("ReadFile(fuse_status) failed: ", err)
	}

	switch s.Param().(string) {
	case "basicVal":
		exerciseFuseboxTempDir(ctx, cleanupCtx, s, cr, nil)
	case "advancedVal":
		exerciseFuseboxTempDir(ctx, cleanupCtx, s, cr, exerciseFuseboxAdvanced)
	case "fspVal":
		exerciseFuseboxFSP(ctx, s)
	}
}

func exerciseFuseboxTempDir(ctx, cleanupCtx context.Context, s *testing.State, cr *chrome.Chrome,
	extraTests func(context.Context, *testing.State, tempDirData)) {

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

	if extraTests != nil {
		extraTests(ctx, s, tdd)
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

func exerciseFuseboxAdvanced(ctx context.Context, s *testing.State, tdd tempDirData) {
	// Run a bunch of commands that are roughly analogous to classic Unix
	// tools: ls, cat, mkdir, etc.
	//
	// Each "FFP" will be replaced by the tdd.FuseboxFilePath.
	commands := [][]string{
		{"ls", "FFP"},
		{"touch", "FFP/file"},
		{"cp", "FFP/wfru.txt", "FFP/copy"},
		{"cat", "FFP/copy"},
		{"cp", "FFP/wurf.txt", "FFP/copy"},
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
		// tdd.FuseboxFilePath.
		arg := func(i int) string {
			rawArg := command[i]
			if strings.HasPrefix(rawArg, "FFP") {
				return tdd.FuseboxFilePath + rawArg[3:]
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
				if err := removeAll(arg(i)); err != nil {
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

// removeAll is like os.RemoveAll but ignores EINTR.
//
// See https://github.com/golang/go/issues/57966
func removeAll(path string) error {
	for {
		err := os.RemoveAll(path)
		if pe, ok := err.(*os.PathError); !ok || (pe.Err != unix.EINTR) {
			return err
		}
	}
}

// renameFile is a rough approximation to running /usr/bin/mv on a Fusebox file.
func renameFile(srcName, dstName string) error {
	return os.Rename(srcName, dstName)
}

// readFileInChunks is like os.ReadFile but uses multiple "read" syscalls (each
// with a 32 KiB buffer). The os.ReadFile standard library routine is too
// helpful, issuing a "stat" syscall before its first "read" syscall, to then
// issue one (large) perfectly sized "read". Here, we would like to check that
// Fusebox (and the fileSystemProvider extension) correctly handles a non-zero
// "read" offset. We want multiple small "read"s instead of one large "read".
func readFileInChunks(filename string, finalSizeHint int) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	ret := make([]byte, 0, finalSizeHint)
	buf := [32 * 1024]byte{}
	for {
		n, err := f.Read(buf[:])
		ret = append(ret, buf[:n]...)
		if err == nil {
			continue
		} else if err == io.EOF {
			err = f.Close()
		} else {
			f.Close()
		}
		return ret, err
	}
}

// readFileAt combines os.Open and os.File.ReadAt in a single function.
func readFileAt(filename string, offset, size int64) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	ret := make([]byte, size)
	n, err := f.ReadAt(ret, offset)
	f.Close()
	return ret[:n], err
}

func exerciseFuseboxFSP(ctx context.Context, s *testing.State) {
	// Find the "/media/fuse/fusebox/fsp.1234etc" directory for the FSP-using
	// Chrome extension.
	fspDirName := ""
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		dirEntries, err := os.ReadDir(filemanager.FuseboxDirPath)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read fusebox dir"))
		}
		for _, dirEntry := range dirEntries {
			if !dirEntry.IsDir() {
				continue
			}
			name := dirEntry.Name()
			if !strings.HasPrefix(name, "fsp.") {
				continue
			}
			filename := filepath.Join(filemanager.FuseboxDirPath, name, "this-is-the-fusebox-fsp-extension.txt")
			if _, err := os.Stat(filename); err == nil {
				fspDirName = filepath.Join(filemanager.FuseboxDirPath, name)
				return nil
			}
		}
		return errors.New(`could not find "this-is-the-fusebox-fsp-extension.txt"`)
	}, nil); err != nil {
		s.Fatal("Could not find fspDirName: ", err)
	}

	// Check file contents.
	testCases := [...]struct {
		filename string
		want     string
	}{
		{"empty.txt", ""},
		{"roses.txt", "Roses are red.\n"},
		{"slow-2s.txt", "Two seconds delay.\n"},
	}
	for _, tc := range testCases {
		if got, err := os.ReadFile(filepath.Join(fspDirName, tc.filename)); err != nil {
			s.Fatalf(`ReadFile(%q): %v`, tc.filename, err)
		} else if string(got) != tc.want {
			s.Fatalf(`ReadFile(%q): got %q, want %q`, tc.filename, got, tc.want)
		}
	}

	// Check the directory with 123 files.
	if dirEntries, err := os.ReadDir(filepath.Join(fspDirName, "123files")); err != nil {
		s.Fatalf(`ReadDir(%q): %v`, "123files", err)
	} else {
		got := 0
		for _, dirEntry := range dirEntries {
			if name := dirEntry.Name(); strings.HasPrefix(name, "foo") {
				got++
			}
		}
		if want := 123; got != want {
			s.Fatalf(`ReadDir(%q): got %d, want %d`, "123files", got, want)
		}
	}

	// Check a FizzBuzz file.
	fizzBuzz0Filename := filepath.Join(fspDirName, "big-fizz-buzz-0.txt")
	if info, err := os.Stat(fizzBuzz0Filename); err != nil {
		s.Fatal("Stat(FizzBuzz0): ", err)
	} else if gotSize, wantSize := info.Size(), int64(1234567); gotSize != wantSize {
		s.Fatalf("Stat(FizzBuzz0): got %d, want %d", gotSize, wantSize)
	} else if contents, err := readFileInChunks(fizzBuzz0Filename, 1234567); err != nil {
		s.Fatal("ReadFile(FizzBuzz0): ", err)
	} else if len(contents) != 1234567 {
		s.Fatalf("ReadFile(FizzBuzz0): got %d, want %d", len(contents), 1234567)
	} else {
		gotMD5 := md5.Sum(contents)
		wantMD5 := [md5.Size]byte{ // https://go.dev/play/p/j2II3eUY_VV
			0x25, 0x77, 0x3b, 0xda, 0x73, 0x97, 0xea, 0x68, 0xd2, 0xea, 0xa5, 0x59, 0x17, 0x17, 0xcc, 0x2a,
		}
		if gotMD5 != wantMD5 {
			s.Errorf("md5(FizzBuzz0): got %02x, want %02x", gotMD5, wantMD5)
		}
	}

	// Check another FizzBuzz file. This time we read only a sub-slice of the
	// file, instead of reading the whole file from start to finish. Open a
	// different filename (with a "-1.txt" instead of "-0.txt" suffix) to avoid
	// any kernel caching.
	fizzBuzz1Filename := filepath.Join(fspDirName, "big-fizz-buzz-1.txt")
	if _, err := os.Stat(fizzBuzz1Filename); err != nil {
		s.Fatal("Stat(FizzBuzz1): ", err)
	} else if contents, err := readFileAt(fizzBuzz0Filename, 1200000, 98765); err != io.EOF {
		s.Fatalf(`ReadAt(FizzBuzz1): got %v, want %v`, err, io.EOF)
	} else if len(contents) != 34567 {
		s.Fatalf("ReadFile(FizzBuzz1): got %d, want %d", len(contents), 34567)
	} else {
		gotMD5 := md5.Sum(contents)
		wantMD5 := [md5.Size]byte{ // https://go.dev/play/p/j2II3eUY_VV
			0xef, 0x72, 0xd7, 0x35, 0xe1, 0x77, 0x99, 0xc2, 0x8b, 0xe9, 0xf2, 0x57, 0x6f, 0x8f, 0x2d, 0xee,
		}
		if gotMD5 != wantMD5 {
			s.Errorf("md5(FizzBuzz1): got %02x, want %02x", gotMD5, wantMD5)
		}
	}
}
