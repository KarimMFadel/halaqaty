# ADR-022: Parse Chat Attachments Before Staging

**Status:** Accepted  
**Date:** 2026-09-11  
**Decider:** Karim

## Context

F-004 FR-022 requires parsed media validation and server-enforced voice duration. Magic bytes and a client-supplied duration do not establish either. ADR-021 originally approved only MinIO as a new backend dependency; the Go standard library has image decoders but no audio-container or PDF parser.

## Decision

- Add FFprobe (distributed in the FFmpeg package) and QPDF to the API runtime and verification environment. Karim explicitly approved both dependencies on 2026-09-11.
- Parse allowlisted audio with FFprobe, reject non-audio streams and malformed containers, and derive duration from parsed packet timing and container metadata. Enforce the 300-second ceiling before MinIO storage. Client duration remains a validated compatibility field, never the authority.
- Validate JPEG/PNG with Go's standard image decoders and validate PDFs with QPDF. Reject malformed data before staging. Bound image dimensions before allocating decoded pixels.
- Invoke fixed executable names with argument arrays, private temporary files, a bounded context and bounded output. Disable FFprobe network protocols. Do not log uploaded bytes, filenames, parser diagnostics, or private temporary paths. Missing tools fail closed.
- Add no transcoding, live-session recording, background audio, or new Go media framework.
- Karim also approved resolving the Flutter `file_picker` constraint to `^10.3.10`, subject to successful dependency resolution and tests. The planned `^12.2.0` conflicts with `flutter_secure_storage ^9.2.2` through incompatible `win32` major versions. Preserve the existing secure-storage dependency and its auth behavior.

## Consequences

API runtime images and developer/CI test environments must install these two tools. Successful media tests use valid generated media rather than header-only placeholders. These tools validate file structure; they do not provide malware scanning or PDF content sanitization.

## Alternatives

Trusting declared duration or signatures violates FR-022. Handwritten audio/PDF parsers would add substantial security-sensitive code. A cloud media service would add an unnecessary data boundary and infrastructure.

## References

- [ADR-021](ADR-021-chat-persistence-media-and-delivery.md)
- [F-004 specification](../../../../specs/004-real-time-chat/spec.md)
- [Constitution](../../../../.specify/memory/constitution.md)
