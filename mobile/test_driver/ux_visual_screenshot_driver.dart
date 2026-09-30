import 'dart:io';

import 'package:integration_test/integration_test_driver_extended.dart';

Future<void> main() async {
  await integrationDriver(
    onScreenshot: (String name, List<int> bytes,
        [Map<String, Object?>? args]) async {
      final file = await File(
        '../specs/019-mobile-app-shell-brand/evidence/screenshots/ux/$name.png',
      ).create(recursive: true);
      await file.writeAsBytes(bytes);
      return true;
    },
  );
}
