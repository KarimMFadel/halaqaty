import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
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

  test('decodes a queue state event with its REST entry identifiers', () {
    final event = QueueRealtimeEventDecoder('live-1')
        .decode(_queueStateFrame(eventId: 'queue-state-1', version: 1));

    expect(event, isA<QueueStateEvent>());
    final state = event! as QueueStateEvent;
    expect(state.eventId, 'queue-state-1');
    expect(state.queue, isA<QueueState>());
    expect(state.queue.entries.single.id, 'entry-1');
    expect(state.queue.version, 1);
  });

  test('drops duplicate queue event ids and signals a round version gap', () {
    final decoder = QueueRealtimeEventDecoder('live-1');

    expect(
      decoder.decode(_queueStateFrame(eventId: 'queue-state-1', version: 1)),
      isA<QueueStateEvent>(),
    );
    expect(
      decoder.decode(_queueStateFrame(eventId: 'queue-state-1', version: 1)),
      isNull,
    );

    final gap = decoder.decode(jsonEncode({
      'type': 'queue.advanced',
      'event_id': 'queue-advance-3',
      'occurred_at': '2026-01-01T00:00:03Z',
      'payload': {
        'session_id': 'live-1',
        'round_id': 'round-1',
        'selected_entry_id': 'entry-1',
        'version': 3,
      },
    }));

    expect(gap, isA<QueueVersionGapEvent>());
    final versionGap = gap! as QueueVersionGapEvent;
    expect(versionGap.previousVersion, 1);
    expect(versionGap.receivedVersion, 3);
  });

  test('accepts distinct queue events at the current round version', () {
    final decoder = QueueRealtimeEventDecoder('live-1');
    expect(
      decoder.decode(_queueStateFrame(eventId: 'queue-state-2', version: 2)),
      isA<QueueStateEvent>(),
    );

    final event = decoder.decode(jsonEncode({
      'type': 'queue.opt_out_requested',
      'event_id': 'opt-out-request-1',
      'occurred_at': '2026-01-01T00:00:01Z',
      'payload': {
        'session_id': 'live-1',
        'round_id': 'round-1',
        'request_id': 'request-1',
        'queue_entry_id': 'entry-1',
        'student_id': 'student-1',
        'version': 2,
      },
    }));

    expect(event, isA<QueueChangeEvent>());
  });

  test('accepts a lower version when an event starts a different round', () {
    final decoder = QueueRealtimeEventDecoder('live-1');
    expect(
      decoder.decode(_queueStateFrame(eventId: 'old-round-state', version: 5)),
      isA<QueueStateEvent>(),
    );

    final event = decoder.decode(jsonEncode({
      'type': 'queue.entry_updated',
      'event_id': 'new-round-entry-update',
      'occurred_at': '2026-01-01T00:00:01Z',
      'payload': {
        'session_id': 'live-1',
        'round_id': 'round-2',
        'queue_entry_id': 'entry-2',
        'student_id': 'student-1',
        'old_status': 'waiting',
        'new_status': 'opted_out',
        'position': 1,
        'entry_version': 2,
        'version': 2,
      },
    }));

    expect(event, isA<QueueChangeEvent>());
  });

  test('websocket client emits queue recovery signal only once for a gap',
      () async {
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
        if (frame['action'] != 'subscribe') return;
        socket.add(_queueStateFrame(eventId: 'queue-state-1', version: 1));
        socket.add(_queueStateFrame(eventId: 'queue-state-1', version: 1));
        socket.add(jsonEncode({
          'type': 'queue.advanced',
          'event_id': 'queue-advance-3',
          'occurred_at': '2026-01-01T00:00:03Z',
          'payload': {
            'session_id': 'live-1',
            'round_id': 'round-1',
            'selected_entry_id': 'entry-1',
            'version': 3,
          },
        }));
      });
    });

    final client = WebSocketRealtimeSessionClient(
      Dio(BaseOptions(baseUrl: 'http://127.0.0.1:${server.port}/api/v1')),
    );
    addTearDown(() async {
      await client.dispose();
      await server.close(force: true);
    });

    final events = await client
        .sessionEvents('live-1', token: 'firebase', backendSessionId: 'session')
        .take(2)
        .toList()
        .timeout(const Duration(seconds: 2));

    expect(events.first, isA<QueueStateEvent>());
    expect(events.last, isA<QueueVersionGapEvent>());
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

String _queueStateFrame({required String eventId, required int version}) =>
    jsonEncode({
      'type': 'queue.state',
      'event_id': eventId,
      'occurred_at': '2026-01-01T00:00:00Z',
      'payload': {
        'session_id': 'live-1',
        'round_id': 'round-1',
        'round_number': 1,
        'round_type': 'revision',
        'lifecycle': 'active',
        'surah_id': 2,
        'from_ayah': 1,
        'to_ayah': 5,
        'grading_required': false,
        'selected_entry_id': null,
        'version': version,
        'policy': {
          'population': 'present_at_activation',
          'unfinished_finalization': 'mark_unfinished_skipped',
          'opt_out': 'approval_required',
          'grade_visibility': 'managers_and_student',
          'grade_correction': 'audited_any_time',
          'version': 1,
        },
        'preorder': [],
        'entries': [
          {
            'queue_entry_id': 'entry-1',
            'student_id': 'student-1',
            'student_name': 'مريم',
            'position': 1,
            'status': 'waiting',
            'version': 1,
          },
        ],
      },
    });
