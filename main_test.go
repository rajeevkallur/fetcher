package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadSuccess(t *testing.T) {
	const body = "hello, fetcher"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	n, status, err := download(context.Background(), &buf, srv.URL, 5*time.Second, 0)
	if err != nil {
		t.Fatalf("download returned error: %v", err)
	}
	if got := buf.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
	if n != int64(len(body)) {
		t.Errorf("n = %d, want %d", n, len(body))
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
}

func TestDownloadNonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	_, _, err := download(context.Background(), &buf, srv.URL, 5*time.Second, 0)
	if err == nil {
		t.Fatal("expected an error for 404 status, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to mention status 404", err)
	}
}

func TestDownloadBadURL(t *testing.T) {
	var buf bytes.Buffer
	_, _, err := download(context.Background(), &buf, "http://invalid.invalid.invalid", time.Second, 0)
	if err == nil {
		t.Fatal("expected an error for an unreachable host, got nil")
	}
}

func TestDownloadRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			http.Error(w, "later", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	n, status, err := download(context.Background(), &buf, srv.URL, 5*time.Second, 3)
	if err != nil {
		t.Fatalf("download with retries returned error: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
	if buf.String() != "ok" {
		t.Errorf("body = %q, want %q", buf.String(), "ok")
	}
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server called %d times, want 3", got)
	}
}

func TestDownloadRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	_, status, err := download(context.Background(), &buf, srv.URL, 5*time.Second, 2)
	if err == nil {
		t.Fatal("expected an error after retries are exhausted, got nil")
	}
	if status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}
}

func TestFetchWritesFile(t *testing.T) {
	const body = "file contents"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.txt")
	fd := fetch(context.Background(), srv.URL, dst, 5*time.Second, 0)
	if fd.err != nil {
		t.Fatalf("fetch returned error: %v", fd.err)
	}
	if fd.filesize != int64(len(body)) {
		t.Errorf("fd.filesize = %d, want %d", fd.filesize, len(body))
	}
	if fd.statusCode != http.StatusOK {
		t.Errorf("fd.statusCode = %d, want %d", fd.statusCode, http.StatusOK)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	if string(got) != body {
		t.Errorf("file = %q, want %q", got, body)
	}
}

func TestFetchAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "content for %s", r.URL.Path)
	}))
	defer srv.Close()

	dir := t.TempDir()
	targets := map[string]string{
		srv.URL + "/a": filepath.Join(dir, "a.txt"),
		srv.URL + "/b": filepath.Join(dir, "b.txt"),
		srv.URL + "/c": filepath.Join(dir, "c.txt"),
	}

	if err := fetchAll(context.Background(), targets, options{timeout: 5 * time.Second, concurrency: 2}); err != nil {
		t.Fatalf("fetchAll returned error: %v", err)
	}

	for url, dst := range targets {
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("reading %s: %v", dst, err)
		}
		want := "content for " + strings.TrimPrefix(url, srv.URL)
		if string(got) != want {
			t.Errorf("%s = %q, want %q", dst, got, want)
		}
	}
}

func TestFetchAllReportsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	targets := map[string]string{
		srv.URL: filepath.Join(dir, "out.txt"),
	}
	if err := fetchAll(context.Background(), targets, options{timeout: 5 * time.Second, concurrency: 1}); err == nil {
		t.Fatal("expected an error when a download fails, got nil")
	}
}

func TestFetchAllReportsAllErrors(t *testing.T) {
	// Return a different status code per path so each failure is distinct.
	status := map[string]int{
		"/a": http.StatusNotFound,
		"/b": http.StatusInternalServerError,
		"/c": http.StatusForbidden,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", status[r.URL.Path])
	}))
	defer srv.Close()

	dir := t.TempDir()
	targets := map[string]string{
		srv.URL + "/a": filepath.Join(dir, "a.txt"),
		srv.URL + "/b": filepath.Join(dir, "b.txt"),
		srv.URL + "/c": filepath.Join(dir, "c.txt"),
	}

	err := fetchAll(context.Background(), targets, options{timeout: 5 * time.Second, concurrency: 2})
	if err == nil {
		t.Fatal("expected an error when downloads fail, got nil")
	}

	// errors.Join reports every failure: all three statuses must be present.
	for _, code := range []string{"404", "500", "403"} {
		if !strings.Contains(err.Error(), code) {
			t.Errorf("error %q does not mention status %s", err, code)
		}
	}
	if got := strings.Count(err.Error(), "unexpected HTTP status"); got != len(targets) {
		t.Errorf("got %d status errors, want %d", got, len(targets))
	}
}

func TestLoadTargets(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "urls.txt")
	content := "# comment\n\nhttps://a.example.com out_a.html\nhttps://b.example.com/path\n"
	if err := os.WriteFile(listPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing list file: %v", err)
	}

	targets, err := loadTargets(listPath)
	if err != nil {
		t.Fatalf("loadTargets returned error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("len(targets) = %d, want 2", len(targets))
	}
	if got := targets["https://a.example.com"]; got != "out_a.html" {
		t.Errorf("explicit output = %q, want %q", got, "out_a.html")
	}
	if got := targets["https://b.example.com/path"]; got != "b.example.com_path.html" {
		t.Errorf("derived output = %q, want %q", got, "b.example.com_path.html")
	}
}

func TestLoadTargetsErrors(t *testing.T) {
	if _, err := loadTargets(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Error("expected an error for a missing file, got nil")
	}

	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, []byte("# only a comment\n"), 0o644); err != nil {
		t.Fatalf("writing empty list: %v", err)
	}
	if _, err := loadTargets(empty); err == nil {
		t.Error("expected an error for a list with no URLs, got nil")
	}
}

func TestOutputName(t *testing.T) {
	cases := map[string]string{
		"https://example.com":     "example.com.html",
		"https://example.com/a/b": "example.com_a_b.html",
		"not a url":               "download.out",
	}
	for in, want := range cases {
		if got := outputName(in); got != want {
			t.Errorf("outputName(%q) = %q, want %q", in, got, want)
		}
	}
}
