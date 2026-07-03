# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.2.0] - 2026-07-02

### Added

- **`!env` directive** — load `.env` files directly in YAML using `joho/godotenv`. Place `!env ./.env` at the top of your YAML to load env vars before `${VAR}` resolution. Multiple `!env` lines supported.

### Added

- **Dot-path variable referencing** — reference nested YAML values via `$a.b.c` syntax. Access values anywhere in the document using dot notation, with a constraint that references cannot be used from inside the target root's subtree.
- **EnvLoader interface** — types can implement `LoadEnv() error` to load environment variables (e.g. from `.env` files) before `${VAR}` placeholders are resolved. Runs automatically if implemented, zero overhead otherwise.
- `Version` constant exported from the package.

### Changed

- **Performance** — dot-path resolution optimized with lazy path map building and reusable stack slices. Up to 32% faster for dot-path workloads, 26% faster for large YAML docs.

### Fixed

- Enum validation tests and benchmarks used comma-separated values (`${VAR:,a,b,c}`) instead of pipe-separated (`${VAR:|a|b|c}`), causing test failures and panics.
- `hasCustomDirectives` was splitting yaml tags on `|` instead of `,`, causing yaml.v3 to panic on unrecognized directives like `required`.

## [0.1.0] - 2026-07-01

### Added

- Environment variable substitution: `${VAR}`, `${VAR:-default}`, `${VAR:?}`, `${VAR:|a|b|c}`
- Inline variables: `$var` references resolved top-to-bottom
- File includes: `!include`, `!include ./file.yaml:?`, `!include ./file.yaml:-./fallback.yaml`
- Conditionals: `!if "$var" == "production" 443 else 8080`
- Post-unmarshal validation: `Validator` interface with `Validate() error`
- Struct tag validation: `required`, `default=`, `enum=`, `min=`, `max=` directives
- Functional options: `SkipEnvVars()`, `SkipValidation()`, `WithVars()`, `SkipIf()`, `SkipIncludes()`
- `UnmarshalWithTiming` for per-phase performance tracking

[Unreleased]: https://github.com/emperorsixpacks/yamlx/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/emperorsixpacks/yamlx/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/emperorsixpacks/yamlx/compare/v1.0.3...v1.1.0
