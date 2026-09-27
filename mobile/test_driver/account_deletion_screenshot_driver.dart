import 'dart:io';

import 'package:integration_test/integration_test_driver_extended.dart';

Future<void> main() async {
  await integrationDriver(
    onScreenshot: (String name, List<int> bytes,
        [Map<String, Object?>? args]) async {
      final fileName = switch (name) {
        'f001_account_deletion_rtl' => 'account-deletion-rtl.png',
        'f001_account_deletion_confirm_rtl' =>
          'account-deletion-confirm-rtl.png',
        _ => null,
      };
      if (fileName == null) return true;
      final file = await File(
        '../specs/001-auth-roles-profile/evidence/screenshots/$fileName',
      ).create(recursive: true);
      await file.writeAsBytes(bytes);
      return true;
    },
  );
}
