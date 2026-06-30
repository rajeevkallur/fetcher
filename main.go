// Command fetcher downloads the contents of a URL and writes it to a file
// or to standard output.
//
// Usage:
//
//	fetcher [-o output] [-timeout duration] [-c n] [-retries n] [-q] [-list file] [url]
//
// With a single url, the body is written to -o (standard output by default).
// With -list, each "url [output]" line in the file is downloaded concurrently.
// With no arguments, a built-in set of URLs is downloaded concurrently.
// Transient failures (network errors, 5xx) are retried up to -retries times
// with exponential backoff, and -q suppresses progress output.
//
// Examples:
//
//	fetcher https://example.com
//	fetcher -o page.html https://example.com
//	fetcher -c 8 -retries 3 -list urls.txt
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

// filedata records the outcome of downloading a single URL.
type filedata struct {
	url        string
	dst        string
	statusCode int
	filesize   int64
	err        error
}

// options holds the tunable settings for a run.
type options struct {
	timeout     time.Duration
	concurrency int
	retries     int
	quiet       bool
}

func main() {
	output := flag.String("o", "-", `output file ("-" for standard output)`)
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP request timeout")
	list := flag.String("list", "", `file of "url [output]" lines to download concurrently`)
	concurrency := flag.Int("c", 4, "maximum number of concurrent downloads")
	retries := flag.Int("retries", 0, "retries for transient failures (network errors, 5xx)")
	quiet := flag.Bool("q", false, "suppress per-file and summary output")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-o output] [-timeout duration] [-c n] [-retries n] [-q] [-list file] [url]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	opts := options{
		timeout:     *timeout,
		concurrency: *concurrency,
		retries:     *retries,
		quiet:       *quiet,
	}

	// Cancel all in-flight downloads on Ctrl-C (SIGINT).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// -list takes precedence: download every entry from the file concurrently.
	if *list != "" {
		targets, err := loadTargets(*list)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetcher: %v\n", err)
			os.Exit(1)
		}
		if err := fetchAll(ctx, targets, opts); err != nil {
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
		if err := fetchAll(ctx, targets, opts); err != nil {
			fmt.Fprintf(os.Stderr, "fetcher: %v\n", err)
			os.Exit(1)
		}
	case 1:
		fd := fetch(ctx, flag.Arg(0), *output, opts.timeout, opts.retries)
		if fd.err != nil {
			fmt.Fprintf(os.Stderr, "fetcher: %v\n", fd.err)
			os.Exit(1)
		}
		if *output != "-" && !opts.quiet {
			fmt.Fprintf(os.Stderr, "wrote %d bytes to %s (%d)\n", fd.filesize, fd.dst, fd.statusCode)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// fetchAll downloads every url in targets using a classic channel-based worker
// pool: a fixed set of concurrency worker goroutines pull jobs off the jobs
// channel and push their result onto the results channel. Every target is
// attempted regardless of individual failures; the errors are collected and
// returned together via errors.Join. Canceling ctx (e.g. on Ctrl-C) stops
// dispatching new jobs and aborts in-flight requests. It prints a summary to
// standard error.
func fetchAll(ctx context.Context, targets map[string]string, opts options) error {
	concurrency := opts.concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(targets) {
		concurrency = len(targets) // no point starting more workers than jobs
	}

	type job struct{ url, dst string }

	jobs := make(chan job)
	results := make(chan filedata)

	// Start the worker pool: each worker pulls jobs until the channel is closed.
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- fetch(ctx, j.url, j.dst, opts.timeout, opts.retries)
			}
		}()
	}

	// Producer: feed jobs to the workers, stopping early if ctx is canceled.
	go func() {
		defer close(jobs)
		for url, dst := range targets {
			select {
			case jobs <- job{url, dst}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Closer: once every worker has exited, no more results will be sent.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect every result until the results channel is closed.
	var (
		errs  []error
		stats []filedata
	)
	for fd := range results {
		stats = append(stats, fd)
		if fd.err != nil {
			errs = append(errs, fd.err)
		}
	}

	if !opts.quiet {
		for _, fd := range stats {
			if fd.err != nil {
				fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", fd.url, fd.err)
				continue
			}
			fmt.Fprintf(os.Stderr, "OK   %s -> %s (%d, %d bytes)\n", fd.url, fd.dst, fd.statusCode, fd.filesize)
		}
		succeeded := len(stats) - len(errs)
		fmt.Fprintf(os.Stderr, "done: %d succeeded, %d failed (of %d)\n", succeeded, len(errs), len(targets))
	}
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
// The download honors cancellation of ctx and retries transient failures up to
// retries times. The outcome is reported in the returned filedata.
func fetch(ctx context.Context, url, dst string, timeout time.Duration, retries int) filedata {
	fd := filedata{url: url, dst: dst}

	out := os.Stdout
	if dst != "-" {
		f, err := os.Create(dst)
		if err != nil {
			fd.err = fmt.Errorf("could not create %q: %w", dst, err)
			return fd
		}
		defer f.Close()
		out = f
	}

	fd.filesize, fd.statusCode, fd.err = download(ctx, out, url, timeout, retries)
	return fd
}

// download performs an HTTP GET on url and copies the response body to w,
// returning the number of bytes written and the HTTP status code. The request
// is bounded by timeout and is canceled if ctx is canceled. Transient failures
// (network errors or 5xx responses) are retried up to retries times with
// exponential backoff. No bytes are written to w until a successful response is
// received, so a retry never corrupts the output.
func download(ctx context.Context, w io.Writer, url string, timeout time.Duration, retries int) (int64, int, error) {
	var (
		n    int64
		code int
		err  error
	)
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 250 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			}
		}

		var retryable bool
		n, code, err, retryable = downloadOnce(ctx, w, url, timeout)
		if err == nil || !retryable || attempt >= retries {
			return n, code, err
		}
	}
}

// downloadOnce performs a single HTTP GET attempt. retryable reports whether the
// failure is worth retrying (network error or 5xx) and is only ever true before
// any bytes are written to w.
func downloadOnce(ctx context.Context, w io.Writer, url string, timeout time.Duration) (n int64, code int, err error, retryable bool) {
	// Per-request timeout that also respects cancellation of the parent ctx.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("could not build request for %q: %w", url, err), false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("could not get %q: %w", url, err), true
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return 0, resp.StatusCode, fmt.Errorf("unexpected HTTP status: %s", resp.Status), resp.StatusCode >= http.StatusInternalServerError
	}

	n, err = io.Copy(w, resp.Body)
	if err != nil {
		return n, resp.StatusCode, fmt.Errorf("error writing body: %w", err), false
	}
	return n, resp.StatusCode, nil, false
}
