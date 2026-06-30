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
fetcher [-o output] [-timeout duration] [url]
```

With a single `url`, the body is written to `-o` (standard output by default).
With **no arguments**, fetcher downloads a built-in set of URLs **concurrently**,
saving each to its own output file.

Flags:

- `-o` — output file (`-` for standard output, the default)
- `-timeout` — HTTP request timeout (default `30s`)

### Examples

```sh
fetcher https://example.com              # print to stdout
fetcher -o page.html https://example.com # save to a file
fetcher -timeout 5s https://example.com  # custom timeout
fetcher                                  # concurrently fetch the built-in URL set
```

## License

Released under the [MIT License](LICENSE).
