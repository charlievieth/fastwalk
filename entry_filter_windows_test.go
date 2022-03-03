// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fastwalk

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFixLongPath(t *testing.T) {
	// 248 is long enough to trigger the longer-than-248 checks in
	// fixLongPath, but short enough not to make a path component
	// longer than 255, which is illegal on Windows. (which
	// doesn't really matter anyway, since this is purely a string
	// function we're testing, and it's not actually being used to
	// do a system call)
	veryLong := "l" + strings.Repeat("o", 248) + "ng"
	for _, test := range []struct{ in, want string }{
		// Short; unchanged:
		{`C:\short.txt`, `C:\short.txt`},
		{`C:\`, `C:\`},
		{`C:`, `C:`},
		// The "long" substring is replaced by a looooooong
		// string which triggers the rewriting. Except in the
		// cases below where it doesn't.
		{`C:\long\foo.txt`, `\\?\C:\long\foo.txt`},
		{`C:/long/foo.txt`, `\\?\C:\long\foo.txt`},
		{`C:\long\foo\\bar\.\baz\\`, `\\?\C:\long\foo\bar\baz`},
		{`\\unc\path`, `\\unc\path`},
		{`long.txt`, `long.txt`},
		{`C:long.txt`, `C:long.txt`},
		{`c:\long\..\bar\baz`, `c:\long\..\bar\baz`},
		{`\\?\c:\long\foo.txt`, `\\?\c:\long\foo.txt`},
		{`\\?\c:\long/foo.txt`, `\\?\c:\long/foo.txt`},
	} {
		in := strings.ReplaceAll(test.in, "long", veryLong)
		want := strings.ReplaceAll(test.want, "long", veryLong)
		if got := fixLongPath(in); got != want {
			got = strings.ReplaceAll(got, veryLong, "long")
			t.Errorf("fixLongPath(%q) = %q; want %q", test.in, got, test.want)
		}
	}
}

func TestEntryFilterLongPath(t *testing.T) {
	tempdir := t.TempDir()
	veryLong := "l" + strings.Repeat("o", 248) + "ng"

	var files []string
	for i := 0; i <= 9; i++ {
		dir := filepath.Join(tempdir, strconv.Itoa(i))
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, veryLong)
		if err := os.WriteFile(name, []byte(strconv.Itoa(i)), 0644); err != nil {
			t.Fatal(err)
		}
		files = append(files, dir, name)
	}

	filter := NewEntryFilter()
	for _, name := range files {
		fi, err := os.Lstat(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []bool{false, true} {
			got := filter.Entry(name, fs.FileInfoToDirEntry(fi))
			if got != want {
				t.Errorf("filepath.Entry(%q) = %t want: %t", name, got, want)
			}
		}
	}
}
