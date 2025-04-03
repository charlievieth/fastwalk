package fastwalk_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/charlievieth/fastwalk"
)

func CreateFiles(files map[string]string) (root string, cleanup func()) {
	tempdir, err := os.MkdirTemp("", "fastwalk-example-*")
	if err != nil {
		panic(err)
	}

	symlinks := map[string]string{}
	for path, contents := range files {
		file := filepath.Join(tempdir, "/src", path)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			panic(err)
		}
		if strings.HasPrefix(contents, "LINK:") {
			symlinks[file] = filepath.FromSlash(strings.TrimPrefix(contents, "LINK:"))
			continue
		}
		if err := os.WriteFile(file, []byte(contents), 0644); err != nil {
			panic(err)
		}
	}

	// Create symlinks after all other files. Otherwise, directory symlinks on
	// Windows are unusable (see https://golang.org/issue/39183).
	for file, dst := range symlinks {
		if err := os.Symlink(dst, file); err != nil {
			panic(err)
		}
	}

	return filepath.Join(tempdir, "src") + "/", func() { os.RemoveAll(tempdir) }
}

func PrettyPrintEntry(root, path string, de fs.DirEntry) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		panic(err)
	}
	rel = filepath.ToSlash(rel)
	if de.Type()&fs.ModeSymlink != 0 {
		dst, err := os.Readlink(path)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s: %s -> %s\n", de.Type(), rel, filepath.ToSlash(dst))
	} else {
		fmt.Printf("%s: %s\n", de.Type(), rel)
	}
}

// This example shows how the [fastwalk.Config] Follow field can be used to
// efficiently and safely follow symlinks. The below example contains a symlink
// loop ("bar/symloop"), which fastwalk detects and does not follow.
//
// NOTE: We still call the [fs.WalkDirFunc] on the symlink that creates a loop,
// but we do not follow/traverse it.
func ExampleWalk_follow() {
	// Setup
	root, cleanup := CreateFiles(map[string]string{
		"bar/bar.go":  "one",
		"bar/symlink": "LINK:bar.go",
		"foo/foo.go":  "two",
		"foo/symdir":  "LINK:../bar/",
		"foo/broken":  "LINK:nope.txt", // Broken symlink
		"foo/foo":     "LINK:../foo/",  // Symlink loop
	})
	defer cleanup()

	conf := fastwalk.Config{
		Follow: true,
	}
	err := fastwalk.Walk(&conf, root, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		PrettyPrintEntry(root, path, de)
		return nil
	})
	if err != nil {
		panic(err)
	}
	// Unordered output:
	// d---------: .
	// d---------: bar
	// ----------: bar/bar.go
	// L---------: bar/symlink -> bar.go
	// d---------: foo
	// L---------: foo/broken -> nope.txt
	// ----------: foo/foo.go
	// L---------: foo/foo -> ../foo/
	// L---------: foo/symdir -> ../bar/
	// L---------: foo/symdir/symlink -> bar.go
	// ----------: foo/symdir/bar.go
}

func ExampleWalk() {
	root, cleanup := CreateFiles(map[string]string{
		"bar/b.txt": "",
		"foo/f.txt": "",
		// Since Config.Follow is set to false, the symbolic link "link" will
		// be visited, but we will not traverse into it (since our walk func
		// does not return [fastwalk.ErrTraverseLink]).
		"link": "LINK:bar",
	})
	defer cleanup()

	err := fastwalk.Walk(nil, root, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		PrettyPrintEntry(root, path, de)
		return nil
	})
	if err != nil {
		panic(err)
	}
	// Unordered output:
	// d---------: .
	// d---------: bar
	// d---------: foo
	// L---------: link -> bar
	// ----------: bar/b.txt
	// ----------: foo/f.txt
}

func ExampleWalk_traverseLink() {
	root, cleanup := CreateFiles(map[string]string{
		"bar/b.txt": "",
		"foo/f.txt": "",
		// This link is followed since our walk func returns ErrTraverseLink
		// when a symlink that resolves to a directory is encountered.
		"link": "LINK:bar",
	})
	defer cleanup()

	err := fastwalk.Walk(nil, root, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		PrettyPrintEntry(root, path, de)

		// It is strongly recommended to use fastwalk.Config.Follow instead
		// of manually handling link traversal since this will fail if the
		// link points to a file (and not a directory).
		if de.Type()&fs.ModeSymlink != 0 {
			fi, err := fastwalk.StatDirEntry(path, de)
			if err != nil {
				return err
			}
			if fi.IsDir() {
				return fastwalk.ErrTraverseLink
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	// Unordered output:
	// d---------: .
	// d---------: bar
	// d---------: foo
	// L---------: link -> bar
	// ----------: bar/b.txt
	// ----------: foo/f.txt
	// ----------: link/b.txt
}
