package main

import (
	"flag"
	"fmt"
	"io"
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

func main() {
	count := flag.Int("count", 10, "Run each test and benchmark n times")
	compCmd := flag.String("comp", "benchstat", "Benchmark comparison command")
	flag.Parse()

	if _, err := exec.LookPath(*compCmd); err != nil {
		log.Fatalf("error: %v: %q\n", err, *compCmd)
	}

	tmpdir, err := os.MkdirTemp("", "fastwalk-bench.*")
	if err != nil {
		log.Fatal(err)
	}

	runTest := func(name string) error {
		fmt.Println("##", name)

		filename := filepath.Join(tmpdir, name)
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

	runTest("filepath")
	runTest("fastwalk")

	benchStat := func(from, to string) {
		fmt.Printf("## %s vs. %s\n", from, to)
		cmd := exec.Command(*compCmd, from, to)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Dir = tmpdir
		if err := cmd.Run(); err != nil {
			log.Fatalf("error running command: %q: %v\n", cmd.Args, err)
		}
		fmt.Print("\n")
	}

	fmt.Println("## Comparisons")
	fmt.Println("########################################################")
	fmt.Print("\n")
	benchStat("filepath", "fastwalk")

	fmt.Printf("Temp: %s\n", tmpdir)
}
