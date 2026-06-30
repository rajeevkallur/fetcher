// Command fetcher downloads the contents of a URL and writes it to a file
// or to standard output.
//
// Usage:
//
//	fetcher [-o output] [-timeout duration] [-c n] [-list file] [url]
//
// With a single url, the body is written to -o (standard output by default).
// With -list, each "url [output]" line in the file is downloaded concurrently.
// With no arguments, a built-in set of URLs is downloaded concurrently.
//
// Examples:
//
//	fetcher https://example.com
//	fetcher -o page.html https://example.com
//	fetcher -c 8 -list urls.txt
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	output := flag.String("o", "-", `output file ("-" for standard output)`)
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP request timeout")
	list := flag.String("list", "", `file of "url [output]" lines to download concurrently`)
	concurrency := flag.Int("c", 4, "maximum number of concurrent downloads")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-o output] [-timeout duration] [-c n] [-list file] [url]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	// -list takes precedence: download every entry from the file concurrently.
	if *list != "" {
		targets, err := loadTargets(*list)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetcher: %v\n", err)
			os.Exit(1)
		}
		if err := fetchAll(targets, *timeout, *concurrency); err != nil {
			fmt.Fprintf(os.Stderr, "fetcher: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch flag.NArg() {
	case 0:
		// No URL given: download a built-in set of URLs concurrently,
		// each saved to its own output file.
		targets := map[string]string{
			"https://example.com": "example.html",
			"https://go.dev":      "go.html",
			"https://pkg.go.dev":  "pkg.html",
		}
		if err := fetchAll(targets, *timeout, *concurrency); err != nil {
			fmt.Fprintf(os.Stderr, "fetcher: %v\n", err)
			os.Exit(1)
		}
	case 1:
		if err := fetch(flag.Arg(0), *output, *timeout); err != nil {
			fmt.Fprintf(os.Stderr, "fetcher: %v\n", err)
			os.Exit(1)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// fetchAll downloads every url in targets, running at most concurrency downloads
// at a time. Each response body is written to its associated output file path.
// It prints a summary to standard error and returns a combined error describing
// any downloads that failed.
func fetchAll(targets map[string]string, timeout time.Duration, concurrency int) error {
	if concurrency < 1 {
		concurrency = 1
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	sem := make(chan struct{}, concurrency)

	for url, dst := range targets {
		wg.Add(1)
		sem <- struct{}{} // acquire a worker slot
		go func(url, dst string) {
			defer wg.Done()
			defer func() { <-sem }() // release the slot
			if err := fetch(url, dst, timeout); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(url, dst)
	}

	wg.Wait()

	failed := len(errs)
	succeeded := len(targets) - failed
	fmt.Fprintf(os.Stderr, "done: %d succeeded, %d failed (of %d)\n", succeeded, failed, len(targets))
	return errors.Join(errs...)
}

// loadTargets reads URL/output-file pairs from path. Each non-empty, non-comment
// (#) line contains a URL optionally followed by whitespace and an output file
// path; when the output is omitted, a name is derived from the URL.
func loadTargets(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open list %q: %w", path, err)
	}
	defer f.Close()

	targets := make(map[string]string)
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		switch len(fields) {
		case 1:
			targets[fields[0]] = outputName(fields[0])
		case 2:
			targets[fields[0]] = fields[1]
		default:
			return nil, fmt.Errorf("%s:%d: expected \"url [output]\", got %q", path, line, text)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading list %q: %w", path, err)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no URLs found in %q", path)
	}
	return targets, nil
}

// outputName derives a reasonable output file name from a URL.
func outputName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "download.out"
	}
	name := u.Host
	if p := strings.Trim(u.Path, "/"); p != "" {
		name += "_" + strings.ReplaceAll(p, "/", "_")
	}
	return name + ".html"
}

// fetch retrieves url and writes the response body to dst. If dst is "-" the
// body is written to standard output; otherwise dst is treated as a file path.
func fetch(url, dst string, timeout time.Duration) error {
	out := os.Stdout
	if dst != "-" {
		f, err := os.Create(dst)
		if err != nil {
			return fmt.Errorf("could not create %q: %w", dst, err)
		}
		defer f.Close()
		out = f
	}

	n, err := download(out, url, timeout)
	if err != nil {
		return err
	}

	if dst != "-" {
		fmt.Fprintf(os.Stderr, "wrote %d bytes to %s\n", n, dst)
	}
	return nil
}

// download performs an HTTP GET on url and copies the response body to w,
// returning the number of bytes written. It returns an error if the request
// fails or the server responds with a non-success status code.
func download(w io.Writer, url string, timeout time.Duration) (int64, error) {
	client := &http.Client{Timeout: timeout}

	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("could not get %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("error writing body: %w", err)
	}
	return n, nil
}
