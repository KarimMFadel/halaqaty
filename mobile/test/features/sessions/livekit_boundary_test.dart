import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

void main() {
  test('media interface dependencies exclude adapters and API clients', () {
    final violations = <String>[];
    final visited = <String>{};
    void visit(String path) {
      if (!visited.add(path)) return;
      final source = File(path).readAsStringSync();
      final imports = RegExp(r'''(?:import|export)\s+['"]([^'"]+)['"]''');
      for (final match in imports.allMatches(source)) {
        final uri = match.group(1)!;
        if (uri.contains('livekit') ||
            uri.contains('flutter_riverpod') ||
            uri.endsWith('_api_client.dart')) {
          violations.add('$path depends on $uri');
        }
        const prefix = 'package:halaqaty_mobile/';
        if (uri.startsWith(prefix)) {
          visit('lib/${uri.substring(prefix.length)}');
        } else if (!uri.contains(':')) {
          visit('${File(path).parent.path}/$uri');
        }
      }
    }

    visit('lib/features/sessions/application/media_session.dart');
    expect(violations, isEmpty,
        reason: 'ADR-023 keeps media contracts independent of composition');
  });

  test('LiveKit SDK import is confined to the media adapter', () {
    final violations = <String>[];
    for (final file in _dartFiles(Directory('lib'))) {
      final source = file.readAsStringSync();
      if (!source.contains('package:livekit_client/')) {
        continue;
      }
      final relative = file.path.replaceAll('\\', '/');
      if (relative != 'lib/features/sessions/data/livekit_media_session.dart') {
        violations.add('$relative imports package:livekit_client');
      }
    }
    expect(violations, isEmpty,
        reason: 'ADR-015 provider imports must stay behind MediaSession');
  });

  test('provider-specific livekit fields do not cross the mobile boundary', () {
    final violations = <String>[];
    final fieldPattern = RegExp(r'\blivekit_[A-Za-z0-9_]+\s*[:=]');
    for (final file in _dartFiles(Directory('lib'))) {
      final source = file.readAsStringSync();
      if (fieldPattern.hasMatch(source)) {
        final relative = file.path.replaceAll('\\', '/');
        violations.add('$relative contains a livekit_ field');
      }
    }
    expect(violations, isEmpty,
        reason: 'public/mobile models must use provider-neutral fields');
  });
}

Iterable<File> _dartFiles(Directory root) sync* {
  if (!root.existsSync()) return;
  for (final entity in root.listSync(recursive: true, followLinks: false)) {
    if (entity is File && entity.path.endsWith('.dart')) {
      yield entity;
    }
  }
}
