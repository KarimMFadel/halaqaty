import 'dart:io';

import 'package:integration_test/integration_test_driver_extended.dart';

/// Saves the Android visual suites' screenshots for CI artifact upload.
Future<void> main() async {
  await integrationDriver(
    onScreenshot: (String name, List<int> bytes,
        [Map<String, Object?>? args]) async {
      final file = await File(
        'build/visual-artifacts/screenshots/$name.png',
      ).create(recursive: true);
      await file.writeAsBytes(bytes);
      return true;
    },
    writeResponseOnFailure: true,
    responseDataCallback: (data) => writeResponseData(
      data,
      destinationDirectory: 'build/visual-artifacts',
    ),
  );
}
