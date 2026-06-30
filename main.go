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
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	output := flag.String("o", "-", `output file ("-" for standard output)`)
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP request timeout")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-o output] [-timeout duration] <url>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	if err := fetch(flag.Arg(0), *output, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "fetcher: %v\n", err)
		os.Exit(1)
	}
}

// fetch retrieves url and writes the response body to dst. If dst is "-" the
// body is written to standard output; otherwise dst is treated as a file path.
func fetch(url, dst string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("could not get %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	out := os.Stdout
	if dst != "-" {
		f, err := os.Create(dst)
		if err != nil {
			return fmt.Errorf("could not create %q: %w", dst, err)
		}
		defer f.Close()
		out = f
	}

	n, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("error writing body: %w", err)
	}

	if dst != "-" {
		fmt.Fprintf(os.Stderr, "wrote %d bytes to %s\n", n, dst)
	}
	return nil
}
