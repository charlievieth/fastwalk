package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

var Tests = []string{
	"filepath",
	"fastwalk",
}

func main() {
	count := flag.Int("count", 5, "Run each test and benchmark n times")
	compCmd := flag.String("comp", "benchstat", "Benchmark comparison command")
	flag.Parse()

	if _, err := exec.LookPath(*compCmd); err != nil {
		log.Fatalf("error: %v: %q\n", err, *compCmd)
	}

	tmpdir, err := ioutil.TempDir("", "fastwalk-bench.*")
	if err != nil {
		log.Fatal(err)
	}

	runTest := func(name string) error {
		fmt.Println("##", name)

		filename := filepath.Join(tmpdir, name+".txt")
		f, err := os.Create(filename)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		args := []string{
			"test",
			"-run", `^$`,
			"-bench", `^BenchmarkWalkComparison$`,
			"-benchmem",
			"-count", strconv.Itoa(*count),
			"github.com/charlievieth/fastwalk",
			"-walkfunc", name,
		}

		cmd := exec.Command("go", args...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = io.MultiWriter(os.Stdout, f)

		if err := cmd.Run(); err != nil {
			log.Fatalf("error running command: %q: %v\n", cmd.Args, err)
		}

		if err := f.Close(); err != nil {
			log.Fatal(err)
		}

		fmt.Print("\n")
		return nil
	}

	for _, name := range Tests {
		runTest(name)
	}

	benchStat := func(from, to string) {
		fmt.Printf("## %s vs. %s\n", from, to)
		cmd := exec.Command(*compCmd,
			filepath.Join(tmpdir, from+".txt"),
			filepath.Join(tmpdir, to+".txt"),
		)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			log.Fatalf("error running command: %q: %v\n", cmd.Args, err)
		}
		fmt.Print("\n")
	}

	fmt.Println("## Comparisons")
	fmt.Println("########################################################")
	fmt.Print("\n")
	benchStat("filepath", "fastwalk")
	benchStat("godirwalk", "fastwalk")

	fmt.Printf("Temp: %s\n", tmpdir)
}
