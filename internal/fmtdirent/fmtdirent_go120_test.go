//go:build !go1.21

// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Backport fs.FormatDirEntry tests from go1.21. We don't test
// the go1.21+ FormatDirEntry function since it just calls the
// stdlib and we don't want changes in its output to break our
// tests.

package fmtdirent_test

import (
	. "io/fs"
	"testing"
	"time"

	"github.com/charlievieth/fastwalk/internal/fmtdirent"
)

// formatTest implements FileInfo to test FormatFileInfo,
// and implements DirEntry to test FormatDirEntry.
type formatTest struct {
	name    string
	size    int64
	mode    FileMode
	modTime time.Time
	isDir   bool
}

func (fs *formatTest) Name() string {
	return fs.name
}

func (fs *formatTest) Size() int64 {
	return fs.size
}

func (fs *formatTest) Mode() FileMode {
	return fs.mode
}

func (fs *formatTest) ModTime() time.Time {
	return fs.modTime
}

func (fs *formatTest) IsDir() bool {
	return fs.isDir
}

func (fs *formatTest) Sys() any {
	return nil
}

func (fs *formatTest) Type() FileMode {
	return fs.mode.Type()
}

func (fs *formatTest) Info() (FileInfo, error) {
	return fs, nil
}

var formatTests = []struct {
	input        formatTest
	wantDirEntry string
}{
	{
		formatTest{
			name:    "hello.go",
			size:    100,
			mode:    0o644,
			modTime: time.Date(1970, time.January, 1, 12, 0, 0, 0, time.UTC),
			isDir:   false,
		},
		"- hello.go",
	},
	{
		formatTest{
			name:    "home/gopher",
			size:    0,
			mode:    ModeDir | 0o755,
			modTime: time.Date(1970, time.January, 1, 12, 0, 0, 0, time.UTC),
			isDir:   true,
		},
		"d home/gopher/",
	},
	{
		formatTest{
			name:    "big",
			size:    0x7fffffffffffffff,
			mode:    ModeIrregular | 0o644,
			modTime: time.Date(1970, time.January, 1, 12, 0, 0, 0, time.UTC),
			isDir:   false,
		},
		"? big",
	},
	{
		formatTest{
			name:    "small",
			size:    -0x8000000000000000,
			mode:    ModeSocket | ModeSetuid | 0o644,
			modTime: time.Date(1970, time.January, 1, 12, 0, 0, 0, time.UTC),
			isDir:   false,
		},
		"S small",
	},
}

func TestFormatDirEntry(t *testing.T) {
	for i, test := range formatTests {
		got := fmtdirent.FormatDirEntry(&test.input)
		if got != test.wantDirEntry {
			t.Errorf("%d: FormatDirEntry(%#v) = %q, want %q", i, test.input, got, test.wantDirEntry)
		}
	}

}
