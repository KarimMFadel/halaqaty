/// Lightweight client-side validation helpers shared across features.
///
/// The backend stays authoritative for every value (constitution §IV.6);
/// these helpers only catch obvious input mistakes before a network call.
library;

/// IANA timezone names are `Area/Location` paths (e.g. `Africa/Cairo`) made
/// of letters, digits, `_`, `+` and `-`; `UTC` is the canonical zoneless
/// value and the deterministic default for pre-F-006 profiles.
final RegExp _ianaZonePattern =
    RegExp(r'^[A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z0-9_+-]+)+$');

/// Returns whether [value] has the shape of an IANA timezone name. This is a
/// shape check only — the server validates against the real IANA database
/// and rejects unknown names with the standard validation envelope.
bool isValidIanaTimezone(String value) {
  final zone = value.trim();
  if (zone == 'UTC') return true;
  return _ianaZonePattern.hasMatch(zone);
}
