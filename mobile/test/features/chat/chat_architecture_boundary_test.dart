import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

void main() {
  test('chat source stays independent from providers and extra sockets', () {
    // T089 is an approved, supplemental architecture-policy guard. Runtime
    // tests cannot prove an absent import, so it scans only chat production
    // sources and does not claim whole-repository coverage.
    final chatRoot = Directory('lib/features/chat');
    final forbiddenImports = <String>[
      'package:livekit_client/',
      'package:firebase_messaging/',
    ];

    var socketOwners = 0;
    for (final file in chatRoot.listSync(recursive: true).whereType<File>()) {
      if (!file.path.endsWith('.dart')) continue;
      final source = file.readAsStringSync();
      for (final forbidden in forbiddenImports) {
        expect(
          source.contains(forbidden),
          isFalse,
          reason: '${file.path} imports forbidden chat dependency $forbidden',
        );
      }
      if (source.contains('WebSocket.connect(')) {
        socketOwners++;
        expect(
          file.path.replaceAll('\\', '/'),
          endsWith('data/chat_realtime_client.dart'),
          reason: 'chat must reuse its one shared realtime client',
        );
      }
    }
    expect(socketOwners, 1, reason: 'chat must own exactly one socket client');
  });
}
