# Copilot Instructions for this Repository

Welcome! This repository contains a Go implementation of an ESC/POS thermal printer driver with USB and Bluetooth LE connectivity. These conventions help AI tools produce high-quality changes.

## Quick context
- Language: Go 1.21
- Modules: `pkg/` (connections, escpos, printer), CLIs in `cmd/`.
- BLE: based on `tinygo.org/x/bluetooth` with robust scan + characteristic discovery.
- USB: based on `github.com/google/gousb`.
- Logging: `github.com/sirupsen/logrus` with levels (Info default, Debug with `-verbose`).

## Coding conventions
- Use logrus for all logging. Levels:
  - Info: user-facing progress and success messages
  - Warn: recoverable issues
  - Error/Fatal: failures
  - Debug: verbose scan/discovery and low-level I/O details (gated by `-verbose`)
- Avoid printing directly with fmt for operational logs; only use fmt for CLI help text and user prompts.
- Prefer small, focused changes; keep public APIs backward compatible unless explicitly requested.
- Keep Windows compatibility in mind. USB printing uses bulk OUT endpoint; BLE writes in 20-byte chunks with short delays.

## BLE discovery rules
- When no address is provided, scan and select devices that expose the ESC/POS characteristic `49535343-8841-43f4-a8d4-ecbe34729bb3`. More characteristics may be added if discovered.
- Cache device capabilities in-session by MAC to avoid re-checking devices already known to be non-printers.
- Do not fall back to arbitrary characteristics (like Device Name). Only use known printer write characteristics.

## Documentation
- Public/exported identifiers must have GoDoc comments describing behavior and parameters.
- Keep README feature lists, usage examples, and prerequisites accurate when behavior changes. README will be the documentation a new user will need to use the commands or library.

## Testing & quality
- Build should succeed on Windows, macOS, and Linux.
- Prefer fast, minimal tests. When changing BLE/USB behavior, include clear notes in PRs describing manual validation steps.
- Run `go vet` and linters where relevant. Avoid introducing dead code or unused imports.

## Commit & PR style
- Clear titles in imperative mood (e.g., "Add BLE characteristic fallback").
- Brief body explaining user-visible changes, edge cases, and risk.

Thanks for helping improve the project.
