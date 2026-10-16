# Changelog

## [Unreleased]

## [v0.3.0] - 2026-10-16

### Added

- Add typed APIs for main-process attachment, interactive exec, rootfs upload, workload templates, and workspace listing.
- Add a versioned external Go consumer check for the published SDK surface.

### Changed

- Update the Go client to the published control protocol v0.2.0 and current Gateway resource signatures.

## [v0.2.0] - 2026-10-13

### Changed

- Split the Go control client into focused lifecycle, sandbox, log, and overview files.
- Use generated first-party API types from the published `cautem-proto` module.

## [v0.1.6] - 2026-10-11

### Changed

- Rename the module, runtime identifiers and project references to the `cautem` namespace.

## [v0.1.0-beta.1] - 2026-10-10

### Added

- Provide typed Go clients for Control RPC gateway information, inference, sandbox and log management, services, settings and secrets.
- Carry request IDs, resource versions and operation recovery through the management client.

## [v0.1.0-alpha.2] - 2026-10-07

### Added

- Scoped provider profile catalog operations and refresh metadata in the public client API.
- Match OpenShell gateway request/response payloads for workspace services and SSH relay operations.
- Encode URL path segments and decode response bodies with operation context instead of discarding transport details.

### Changed

- Distribute the module under Apache-2.0 and link the public gateway/API documentation from the README.

## [v0.0.2-alpha.1] - 2026-09-28

### Added

- `Proposal.SecurityFlagged` for gateway approval flows that gate bulk approve.
