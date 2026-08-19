import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';

void main() {
  test('builds the required subscribe and heartbeat frames', () {
    expect(realtimeSubscribeMessage('session.live-1'), {
      'action': 'subscribe',
      'topic': 'session.live-1',
    });
    expect(realtimePingMessage, {'type': 'ping'});
  });

  test('real websocket path subscribes, receives snapshot, and heartbeats',
      () async {
    final frames = <Map<String, dynamic>>[];
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      if (request.method == 'POST' &&
          request.uri.path == '/api/v1/realtime/tickets') {
        request.response.headers.contentType = ContentType.json;
        request.response.write(jsonEncode({'token': 'ticket'}));
        await request.response.close();
        return;
      }
      if (!WebSocketTransformer.isUpgradeRequest(request)) return;
      final socket = await WebSocketTransformer.upgrade(request);
      socket.listen((raw) {
        final frame = jsonDecode(raw as String) as Map<String, dynamic>;
        frames.add(frame);
        if (frame['action'] == 'subscribe') {
          socket.add(
              jsonEncode({'type': 'subscribed', 'topic': 'session.live-1'}));
          socket.add(jsonEncode({
            'type': 'session.snapshot',
            'timestamp': '2026-01-01T00:00:00Z',
            'payload': {
              'session': {
                'id': 'live-1',
                'status': 'active',
                'is_locked': false
              },
              'participants': []
            }
          }));
        }
      });
    });

    final client = WebSocketRealtimeSessionClient(
        Dio(BaseOptions(baseUrl: 'http://127.0.0.1:${server.port}/api/v1')),
        heartbeatInterval: const Duration(milliseconds: 1));
    addTearDown(() async {
      await client.dispose();
      await server.close(force: true);
    });

    final event = await client
        .sessionEvents('live-1', token: 'firebase', backendSessionId: 'session')
        .first
        .timeout(const Duration(seconds: 2));
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(event, isA<SessionSnapshotEvent>());
    expect(frames.first, realtimeSubscribeMessage('session.live-1'));
    expect(frames.any((frame) => frame['type'] == 'ping'), isTrue);
  });

  test('parses a session snapshot with participants', () {
    final raw = jsonEncode({
      'type': 'session.snapshot',
      'timestamp': '2026-01-01T00:00:00Z',
      'payload': {
        'session': {'id': 'live-1', 'status': 'active', 'is_locked': true},
        'participants': [
          {
            'user_id': 'u1',
            'display_name': 'عمر عبدالله',
            'role': 'student',
            'is_currently_present': true,
            'hand_raised_at': '2026-01-01T00:00:01Z'
          }
        ]
      }
    });

    final event = parseRealtimeSessionEvent(raw, 'live-1');

    expect(event, isA<SessionSnapshotEvent>());
    final snapshot = event! as SessionSnapshotEvent;
    expect(snapshot.sessionId, 'live-1');
    expect(snapshot.isLocked, isTrue);
    expect(snapshot.participants.single.userId, 'u1');
    expect(snapshot.participants.single.isHandRaised, isTrue);
  });

  test('drops events for other sessions and unknown types', () {
    final otherSession = jsonEncode({
      'type': 'session.participant_joined',
      'payload': {
        'session_id': 'live-2',
        'user_id': 'u2',
        'display_name': 'X',
        'role': 'teacher'
      }
    });
    expect(parseRealtimeSessionEvent(otherSession, 'live-1'), isNull);

    final unknownType = jsonEncode({'type': 'chat.typing', 'payload': {}});
    expect(parseRealtimeSessionEvent(unknownType, 'live-1'), isNull);
  });

  test('parses hand raised with the envelope timestamp', () {
    final raw = jsonEncode({
      'type': 'session.hand_raised',
      'timestamp': '2026-01-01T10:30:00Z',
      'payload': {
        'session_id': 'live-1',
        'participant_id': 'u1',
        'participant_name': 'عمر عبدالله'
      }
    });

    final event = parseRealtimeSessionEvent(raw, 'live-1');

    expect(event, isA<HandRaisedEvent>());
    final raised = event! as HandRaisedEvent;
    expect(raised.participantId, 'u1');
    expect(raised.participantName, 'عمر عبدالله');
    expect(raised.at, DateTime.utc(2026, 1, 1, 10, 30));
  });

  test('parses lock changed and participant removed events', () {
    final lockRaw = jsonEncode({
      'type': 'session.lock_changed',
      'payload': {'session_id': 'live-1', 'locked': true, 'changed_by': 'u9'}
    });
    expect(
        parseRealtimeSessionEvent(lockRaw, 'live-1'), isA<LockChangedEvent>());

    final removedRaw = jsonEncode({
      'type': 'session.participant_removed',
      'payload': {'session_id': 'live-1', 'user_id': 'u1', 'changed_by': 'u9'}
    });
    expect(parseRealtimeSessionEvent(removedRaw, 'live-1'),
        isA<ParticipantRemovedEvent>());
  });

  test('ignores malformed frames', () {
    expect(parseRealtimeSessionEvent('not json', 'live-1'), isNull);
    expect(parseRealtimeSessionEvent('{"type": 42}', 'live-1'), isNull);
  });

  test('builds the websocket url from the api base url', () {
    expect(realtimeWebSocketUrl('http://localhost:8080/api/v1').toString(),
        'ws://localhost:8080/api/v1/ws');
    expect(realtimeWebSocketUrl('https://api.halaqaty.app/api/v1').toString(),
        'wss://api.halaqaty.app/api/v1/ws');
    expect(realtimeWebSocketUrl('http://localhost:8080/api/v1/').toString(),
        'ws://localhost:8080/api/v1/ws');
  });
}
