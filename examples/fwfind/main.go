// fwfind is a an example program that is similar to POSIX find,
// but faster and worse (it's an example).
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/charlievieth/fastwalk"
)

const usageMsg = `Usage: %[1]s [-L] [-name] [PATH...]:

%[1]s is a poor replacement for the POSIX find utility

`

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, usageMsg, filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	pattern := flag.String("name", "", "Pattern to match file names against.")
	followLinks := flag.Bool("L", false, "Follow symbolic links")
	flag.Parse()

	// If no paths are provided default to the current directory: "."
	args := flag.Args()
	if len(args) == 0 {
		args = append(args, ".")
	}

	// Follow links if the "-L" flag is provided
	conf := fastwalk.Config{
		Follow: *followLinks,
	}

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			return nil // returning the error stops iteration
		}
		if *pattern != "" {
			if ok, err := filepath.Match(*pattern, d.Name()); !ok {
				// invalid pattern (err != nil) or name does not match
				return err
			}
		}
		_, err = fmt.Println(path)
		return err
	}
	for _, root := range args {
		if err := fastwalk.Walk(&conf, root, walkFn); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", root, err)
			os.Exit(1)
		}
	}
}
