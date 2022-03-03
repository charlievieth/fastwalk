// fwwc is a an example program that recursively walks directories and
// prints the number of lines in each file it encounters.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/charlievieth/fastwalk"
)

var newLine = []byte{'\n'}

// countLinesInFile returns the number of newlines ('\n') in file name.
func countLinesInFile(name string) (int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf := make([]byte, 16*1024)
	var lines int64
	for {
		n, e := f.Read(buf)
		if n > 0 {
			lines += int64(bytes.Count(buf[:n], newLine))
		}
		if e != nil {
			if e != io.EOF {
				err = e
			}
			break
		}
	}
	return lines, err
}

func LineCount(root string, followLinks bool) error {
	countLinesWalkFn := func(path string, d fs.DirEntry, err error) error {
		// We wrap this with fastwalk.IgnorePermissionErrors so we know the
		// error is not a permission error (common when walking outside a users
		// home directory) and is likely something worse so we should return it
		// and abort the walk.
		//
		// A common error here is "too many open files", which can occur if the
		// walkFn opens, but does not close, files.
		if err != nil {
			return err
		}

		// If the entry is a symbolic link get the type of file that
		// it references.
		typ := d.Type()
		if typ&fs.ModeSymlink != 0 {
			if fi, err := fastwalk.StatDirEntry(path, d); err == nil {
				typ = fi.Mode().Type()
			}
		}

		// Skip dot (".") files (but allow "." / PWD as the path)
		if path != "." && typ.IsDir() {
			name := d.Name()
			if name == "" || name[0] == '.' || name[0] == '_' {
				return fastwalk.SkipDir
			}
			return nil
		}
		if typ.IsRegular() {
			lines, err := countLinesInFile(path)
			if err == nil {
				fmt.Printf("%8d %s\n", lines, path)
			} else {
				// Print but do not return the error.
				fmt.Fprintf(os.Stderr, "%s: %s\n", path, err)
			}
		}
		return nil
	}

	// Ignore permission errors traversing directories.
	//
	// Note: this only ignores permission errors when traversing directories.
	// Permission errors may still be encountered when accessing files.
	walkFn := fastwalk.IgnorePermissionErrors(countLinesWalkFn)

	conf := fastwalk.Config{
		// Safely follow symbolic links. This can also be achieved by
		// wrapping walkFn with fastwalk.FollowSymlinks().
		Follow: followLinks,

		// If NumWorkers is ≤ 0 the default is used, which is sufficient
		// for most use cases.
	}
	// Note: Walk can also be called with a nil Config, in which case
	// fastwalk.DefaultConfig is used.
	if err := fastwalk.Walk(&conf, root, walkFn); err != nil {
		return fmt.Errorf("walking directory %s: %w", root, err)
	}
	return nil
}

const UsageMsg = `Usage: %[1]s [-L] [PATH...]:

%[1]s prints the number of lines in each file it finds,
ignoring directories that start with '.' or '_'.

`

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, UsageMsg, filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	followLinks := flag.Bool("L", false, "Follow symbolic links")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		args = append(args, ".")
	}
	for _, root := range args {
		// fmt.Println("ROOT:", root)
		if err := LineCount(root, *followLinks); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}
}
