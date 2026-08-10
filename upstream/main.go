// Command serve simulates the terminal output of `routatic-proxy serve` for
// testing routatic-proxy-mate through the pipe.
//
// It reads a sample log file (from the examples/ directory) and prints its
// lines to stdout in a loop, pausing a random interval between lines.  After
// every --every lines it pauses for an extra rest, mimicking a real server
// occasionally waiting on upstream.  With --once it emits a single pass
// through the file and then exits, which is handy for testing the EOF path
// (routatic-proxy-mate prints the summary).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"
)

func main() {
	minSec := flag.Float64("min", 0.5, "minimum seconds between lines")
	maxSec := flag.Float64("max", 1.5, "maximum seconds between lines")
	every := flag.Int("every", 10, "take an extra rest after every N lines")
	restMinSec := flag.Float64("rest-min", 3.0, "minimum extra rest seconds after --every lines")
	restMaxSec := flag.Float64("rest-max", 5.0, "maximum extra rest seconds after --every lines")
	once := flag.Bool("once", false, "emit a single pass through the file, then exit")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: serve [flags] <sample-log-file>")
		os.Exit(2)
	}
	if *minSec <= 0 || *maxSec < *minSec {
		fmt.Fprintln(os.Stderr, "serve: need 0 < --min <= --max")
		os.Exit(2)
	}
	if *every < 1 {
		fmt.Fprintln(os.Stderr, "serve: --every must be >= 1")
		os.Exit(2)
	}
	if *restMinSec <= 0 || *restMaxSec < *restMinSec {
		fmt.Fprintln(os.Stderr, "serve: need 0 < --rest-min <= --rest-max")
		os.Exit(2)
	}

	lines, err := readLines(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		os.Exit(1)
	}
	if len(lines) == 0 {
		fmt.Fprintln(os.Stderr, "serve: sample file is empty")
		os.Exit(1)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	span := *maxSec - *minSec
	restSpan := *restMaxSec - *restMinSec
	var emitted int
	for {
		for _, line := range lines {
			// os.Stdout is unbuffered in Go, so each line reaches the pipe
			// immediately (no explicit flush needed).
			fmt.Println(line)
			emitted++
			wait := time.Duration((*minSec + rng.Float64()*span) * float64(time.Second))
			time.Sleep(wait)

			// After every --every lines, add an extra rest to mimic a real
			// server occasionally pausing (e.g. waiting on upstream).
			if emitted%*every == 0 {
				rest := time.Duration((*restMinSec + rng.Float64()*restSpan) * float64(time.Second))
				time.Sleep(rest)
			}
		}
		if *once {
			return
		}
	}
}

// readLines returns the non-empty content of path as lines.  The line buffer
// is raised so long log lines (verbose errors) are handled.
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 256*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}
