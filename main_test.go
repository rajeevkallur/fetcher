package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	n, err := download(&buf, srv.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("download returned error: %v", err)
	}
	if got := buf.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
	if n != int64(len(body)) {
		t.Errorf("n = %d, want %d", n, len(body))
	}
}

func TestDownloadNonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	_, err := download(&buf, srv.URL, 5*time.Second)
	if err == nil {
		t.Fatal("expected an error for 404 status, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to mention status 404", err)
	}
}

func TestDownloadBadURL(t *testing.T) {
	var buf bytes.Buffer
	_, err := download(&buf, "http://invalid.invalid.invalid", time.Second)
	if err == nil {
		t.Fatal("expected an error for an unreachable host, got nil")
	}
}

func TestFetchWritesFile(t *testing.T) {
	const body = "file contents"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.txt")
	if err := fetch(srv.URL, dst, 5*time.Second); err != nil {
		t.Fatalf("fetch returned error: %v", err)
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

	if err := fetchAll(targets, 5*time.Second, 2); err != nil {
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
	if err := fetchAll(targets, 5*time.Second, 1); err == nil {
		t.Fatal("expected an error when a download fails, got nil")
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
