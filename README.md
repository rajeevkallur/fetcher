# fetcher

A small Go command that downloads the contents of a URL and writes it to a file
or to standard output.

## Install

```sh
go install github.com/rajeevkallur/fetcher@latest
```

## Usage

```sh
fetcher [-o output] [-timeout duration] <url>
```

Flags:

- `-o` — output file (`-` for standard output, the default)
- `-timeout` — HTTP request timeout (default `30s`)

### Examples

```sh
fetcher https://example.com              # print to stdout
fetcher -o page.html https://example.com # save to a file
fetcher -timeout 5s https://example.com  # custom timeout
```

## License

Released under the [MIT License](LICENSE).
