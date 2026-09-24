import 'dart:io';

import 'package:integration_test/integration_test_driver_extended.dart';

/// Screenshot-writing driver for the Wave 2 visual evidence harness.
///
/// Run from `mobile/`:
///   flutter drive \
///     --driver=test_driver/wave2_screenshot_driver.dart \
///     --target=integration_test/wave2_sessions_visual_test.dart \
///     -d emulator-5554
///
/// PNGs land in `../specs/019-mobile-app-shell-brand/evidence/screenshots/wave2/`.
Future<void> main() async {
  await integrationDriver(
    onScreenshot: (String name, List<int> bytes,
        [Map<String, Object?>? args]) async {
      final file = await File(
        '../specs/019-mobile-app-shell-brand/evidence/screenshots/wave2/$name.png',
      ).create(recursive: true);
      await file.writeAsBytes(bytes);
      return true;
    },
  );
}
