# halaqaty_mobile

A new Flutter project.

## Integration tests in CI

Functional integration tests run on Linux; UX and Wave 0–2 screenshot suites run
on Android emulators because their native screenshot API is unavailable on Linux.
The Android jobs upload PNGs and test logs using `test_driver/ci_screenshot_driver.dart`.
See the [GitHub Actions guide](../docs/engineering/deployment/GITHUB_ACTIONS.md)
for commands, fixture requirements, artifacts, and verification limits.

## Getting Started

This project is a starting point for a Flutter application.

A few resources to get you started if this is your first Flutter project:

- [Learn Flutter](https://docs.flutter.dev/get-started/learn-flutter)
- [Write your first Flutter app](https://docs.flutter.dev/get-started/codelab)
- [Flutter learning resources](https://docs.flutter.dev/reference/learning-resources)

For help getting started with Flutter development, view the
[online documentation](https://docs.flutter.dev/), which offers tutorials,
samples, guidance on mobile development, and a full API reference.
