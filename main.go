// Command fetcher downloads the contents of a URL and writes it to a file
// or to standard output.
//
// Usage:
//
//	fetcher [-o output] [-timeout duration] <url>
//
// Examples:
//
//	fetcher https://example.com
//	fetcher -o page.html https://example.com
//	fetcher -timeout 5s -o - https://example.com
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

func main() {
	output := flag.String("o", "-", `output file ("-" for standard output)`)
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP request timeout")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-o output] [-timeout duration] [url]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	switch flag.NArg() {
	case 0:
		// No URL given: download a built-in set of URLs concurrently,
		// each saved to its own output file.
		targets := map[string]string{
			"https://example.com": "example.html",
			"https://go.dev":      "go.html",
			"https://pkg.go.dev":  "pkg.html",
		}
		if err := fetchAll(targets, *timeout); err != nil {
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

// fetchAll downloads every url in targets concurrently, writing each response
// body to its associated output file path. It waits for all downloads to
// finish and returns a combined error describing any that failed.
func fetchAll(targets map[string]string, timeout time.Duration) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for url, dst := range targets {
		wg.Add(1)
		go func(url, dst string) {
			defer wg.Done()
			if err := fetch(url, dst, timeout); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(url, dst)
	}

	wg.Wait()
	return errors.Join(errs...)
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
