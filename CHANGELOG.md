# Changelog

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- The comment posted when the rules fail now links the pre-defined rules to the block that defines them, pinned to the commit that ran.
  ([#58](https://github.com/sonolink/arbiterer/pull/58))
- The comment now has `Recheck`, `Unlink`, and `Delete my data` links. Each asks the author to sign in with GitHub first, and `Unlink` and `Delete my data` ask them to confirm.
  ([#73](https://github.com/sonolink/arbiterer/pull/73))

### Changed

- The comment now says when a recheck is running, and asks the author to link again after they unlink or delete their data.
  ([#73](https://github.com/sonolink/arbiterer/pull/73))

### Deprecated

### Removed

- The `/arbiterer recheck` command. Use the `Recheck` link in the comment instead.
  ([#73](https://github.com/sonolink/arbiterer/pull/73))

### Fixed

- A new run now resets the recheck cooldown, so the author can recheck it straight away.
  ([#73](https://github.com/sonolink/arbiterer/pull/73))

### Security

## [1.0.0] - 2026-10-21

### Added

- Initial release.

[Unreleased]: https://github.com/sonolink/arbiterer/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/sonolink/arbiterer/releases/tag/v1.0.0
