# fetcher

[![CI](https://github.com/rajeevkallur/fetcher/actions/workflows/ci.yml/badge.svg)](https://github.com/rajeevkallur/fetcher/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rajeevkallur/fetcher.svg)](https://pkg.go.dev/github.com/rajeevkallur/fetcher)

A small Go command that downloads the contents of a URL and writes it to a file
or to standard output.

## Install

```sh
go install github.com/rajeevkallur/fetcher@latest
```

## Usage

```sh
fetcher [-o output] [-timeout duration] [-c n] [-list file] [url]
```

- A single `url` writes the body to `-o` (standard output by default).
- `-list file` downloads every `url [output]` line in the file **concurrently**.
- With **no arguments**, a built-in set of URLs is downloaded concurrently.

Flags:

- `-o` — output file (`-` for standard output, the default)
- `-timeout` — HTTP request timeout (default `30s`)
- `-c` — maximum number of concurrent downloads (default `4`)
- `-list` — file of `url [output]` lines to download concurrently

### List file format

```text
# lines starting with # are comments; blank lines are ignored
https://example.com            example.html
https://go.dev                 go.html
https://pkg.go.dev             # output name derived from the URL
```

### Examples

```sh
fetcher https://example.com              # print to stdout
fetcher -o page.html https://example.com # save to a file
fetcher -timeout 5s https://example.com  # custom timeout
fetcher -c 8 -list urls.txt              # concurrent download from a list
fetcher                                  # concurrently fetch the built-in URL set
```

## License

Released under the [MIT License](LICENSE).
