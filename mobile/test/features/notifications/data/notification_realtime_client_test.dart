import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart' show ProviderContainer;
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/notifications/data/notification_realtime_client.dart';
import 'package:halaqaty_mobile/features/notifications/domain/notification_models.dart';

NotificationCredentials get credentials => const NotificationCredentials(
      firebaseIdToken: 'firebase-token',
      sessionId: 'backend-session',
    );

void main() {
  /// Polls until a condition holds (test-process only; loopback sockets
  /// deliver asynchronously).
  Future<void> until(bool Function() condition) async {
    const deadline = Duration(seconds: 5);
    final end = DateTime.now().add(deadline);
    while (!condition()) {
      if (DateTime.now().isAfter(end)) {
        fail('condition did not hold within $deadline');
      }
      await Future<void>.delayed(const Duration(milliseconds: 20));
    }
  }

  group('decodeNotificationEvent', () {
    test('decodes the contract envelope with a notification id', () {
      final event = decodeNotificationEvent(jsonEncode({
        'type': 'notification.updated',
        'timestamp': '2026-10-25T18:30:00Z',
        'payload': {
          'notification_id': 'aaaaaaaa-2222-2222-2222-222222222222',
          'unread_count': 3,
        },
      }));

      expect(event, isNotNull);
      expect(event!.unreadCount, 3);
      expect(event.notificationId, 'aaaaaaaa-2222-2222-2222-222222222222');
      expect(event.isReconnectSignal, isFalse);
    });

    test('decodes bulk/count payloads without a notification id', () {
      final event = decodeNotificationEvent(jsonEncode({
        'type': 'notification.updated',
        'timestamp': '2026-10-25T18:30:00Z',
        'payload': {'unread_count': 9},
      }));

      expect(event!.unreadCount, 9);
      expect(event.notificationId, isNull);
    });

    test('other event types, bad payloads, and malformed frames return null',
        () {
      expect(
        decodeNotificationEvent(jsonEncode({
          'type': 'chat.message',
          'payload': {'unread_count': 1},
        })),
        isNull,
      );
      expect(
        decodeNotificationEvent(jsonEncode({
          'type': 'notification.updated',
          'payload': {},
        })),
        isNull,
      );
      expect(
        decodeNotificationEvent(jsonEncode({
          'type': 'notification.updated',
          'payload': {'unread_count': -1},
        })),
        isNull,
      );
      expect(decodeNotificationEvent('not json'), isNull);
      expect(decodeNotificationEvent('[1,2,3]'), isNull);
    });
  });

  group('provider', () {
    test('exposes the websocket client over the shared dio', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      expect(container.read(notificationRealtimeClientProvider),
          isA<WebSocketNotificationRealtimeClient>());
    });
  });

  group('WebSocketNotificationRealtimeClient', () {
    test(
        'connects with the shared ticket, receives notification events, and '
        'sends only heartbeats (no topic subscribe)', () async {
      final ticketRequests = <HttpHeaders>[];
      final upgrades = <String>[]; // query strings of WS upgrades
      final frames = <Map<String, dynamic>>[];
      final sockets = <WebSocket>[];

      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      server.listen((request) async {
        if (request.method == 'POST' &&
            request.uri.path == '/api/v1/realtime/tickets') {
          ticketRequests.add(request.headers);
          request.response.headers.contentType = ContentType.json;
          request.response.write(jsonEncode({'token': 'ticket-1'}));
          await request.response.close();
          return;
        }
        if (!WebSocketTransformer.isUpgradeRequest(request)) {
          request.response.statusCode = 404;
          await request.response.close();
          return;
        }
        upgrades.add(request.uri.query);
        final socket = await WebSocketTransformer.upgrade(request);
        sockets.add(socket);
        socket.listen((data) {
          final frame = jsonDecode(data as String) as Map<String, dynamic>;
          frames.add(frame);
        });
      });
      final dio =
          Dio(BaseOptions(baseUrl: 'http://127.0.0.1:${server.port}/api/v1'));
      final client = WebSocketNotificationRealtimeClient(
        dio,
        heartbeatInterval: const Duration(milliseconds: 50),
      );
      addTearDown(() async {
        await client.dispose();
        for (final socket in sockets) {
          await socket.close();
        }
        await server.close(force: true);
      });

      final received = <NotificationRealtimeEvent>[];
      final subscription = client.events(credentials).listen(received.add);
      addTearDown(subscription.cancel);

      // Ticket carries the shared authenticated headers.
      await Future<void>.delayed(const Duration(milliseconds: 100));
      expect(ticketRequests, hasLength(1));
      expect(ticketRequests.single.value('Authorization'),
          'Bearer firebase-token');
      expect(ticketRequests.single.value('X-Halaqaty-Session-ID'),
          'backend-session');
      expect(upgrades.single, 'token=ticket-1');

      // Server push arrives as the decoded event.
      sockets.single.add(jsonEncode({
        'type': 'notification.updated',
        'timestamp': '2026-10-25T18:30:00Z',
        'payload': {'unread_count': 7},
      }));
      await until(() => received.isNotEmpty);
      expect(received.single.unreadCount, 7);

      // Heartbeats flow; no subscription frames are ever sent.
      await Future<void>.delayed(const Duration(milliseconds: 150));
      expect(frames, isNotEmpty);
      expect(frames.every((frame) => frame['type'] == 'ping'), isTrue);
    });

    test('re-establishes the socket after a drop and signals the reconnect',
        () async {
      final sockets = <WebSocket>[];
      var connections = 0;

      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      server.listen((request) async {
        if (request.method == 'POST' &&
            request.uri.path == '/api/v1/realtime/tickets') {
          request.response.headers.contentType = ContentType.json;
          request.response.write(jsonEncode({'token': 'ticket-1'}));
          await request.response.close();
          return;
        }
        if (!WebSocketTransformer.isUpgradeRequest(request)) {
          request.response.statusCode = 404;
          await request.response.close();
          return;
        }
        final socket = await WebSocketTransformer.upgrade(request);
        connections++;
        sockets.add(socket);
        socket.listen(null, onDone: () {});
        if (connections == 1) {
          // Simulate a network drop right after the first connection.
          await Future<void>.delayed(const Duration(milliseconds: 50));
          await socket.close(1011, 'dropped');
        }
      });
      final dio =
          Dio(BaseOptions(baseUrl: 'http://127.0.0.1:${server.port}/api/v1'));
      final client = WebSocketNotificationRealtimeClient(
        dio,
        heartbeatInterval: const Duration(seconds: 30),
      );
      addTearDown(() async {
        await client.dispose();
        await server.close(force: true);
      });

      final received = <NotificationRealtimeEvent>[];
      final subscription = client.events(credentials).listen(received.add);
      addTearDown(subscription.cancel);

      // The second connection arrives via the internal reconnect loop.
      await until(() => connections >= 2);
      sockets.last.add(jsonEncode({
        'type': 'notification.updated',
        'timestamp': '2026-10-25T18:31:00Z',
        'payload': {'unread_count': 2},
      }));
      await until(() => received.length >= 2);

      // Reconnect signal first (REST reconcile trigger), then the live event.
      expect(received[0].isReconnectSignal, isTrue);
      expect(received[1].unreadCount, 2);
    });

    test('a rejected ticket (401) ends the stream without retrying', () async {
      var ticketRequests = 0;
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      server.listen((request) async {
        if (request.method == 'POST' &&
            request.uri.path == '/api/v1/realtime/tickets') {
          ticketRequests++;
          request.response.statusCode = 401;
          await request.response.close();
          return;
        }
        request.response.statusCode = 404;
        await request.response.close();
      });
      final dio =
          Dio(BaseOptions(baseUrl: 'http://127.0.0.1:${server.port}/api/v1'));
      final client = WebSocketNotificationRealtimeClient(dio);
      addTearDown(() async {
        await client.dispose();
        await server.close(force: true);
      });

      await client
          .events(credentials)
          .drain<void>()
          .timeout(const Duration(seconds: 2));

      // A second ticket attempt would loop forever against a revoked
      // session; a short observation window asserts the terminal path.
      await Future<void>.delayed(const Duration(milliseconds: 1200));
      expect(ticketRequests, 1);
    });
  });
}
