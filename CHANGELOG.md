# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.10.0]

### Added

- `-retries` flag: transient failures (network errors and `5xx` responses) are
  retried with exponential backoff.
- `-q` flag: suppress per-file and summary progress output.
- golangci-lint job in CI.
- This changelog.

### Changed

- Consolidated download bookkeeping into a single `filedata` struct that records
  the numeric HTTP status code and file size.
- Run settings are now grouped in an `options` struct.

## [v0.9.0]

### Added

- Track each download's byte size and HTTP status via a `fileData` struct.

## [v0.8.0]

### Changed

- Replaced the `errgroup` dependency with an explicit channel-based worker pool;
  the module now has no external dependencies.

## [v0.7.0]

### Added

- Aggregate every download error with `errors.Join` instead of failing fast.

## [v0.6.0]

### Added

- Cancel in-flight downloads on Ctrl-C via `signal.NotifyContext`.

## [v0.5.1]

### Fixed

- Re-tagged release so the checksum database could index the now-public module.

## [v0.5.0]

### Added

- `-list` flag to download a file of `url [output]` lines concurrently.
- Bounded worker pool via the `-c` flag.

## [v0.4.0]

### Added

- Concurrent download of a built-in set of URLs when no argument is given.

## [v0.3.0]

### Added

- Unit tests and a GitHub Actions CI workflow.

## [v0.2.0]

### Added

- Configurable `-o` (output) and `-timeout` flags.

## [v0.1.0]

### Added

- Initial release: download a URL to a file or standard output.
