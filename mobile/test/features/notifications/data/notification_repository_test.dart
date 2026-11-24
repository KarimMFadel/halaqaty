import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/notifications/data/notification_repository.dart';
import 'package:halaqaty_mobile/features/notifications/domain/notification_models.dart';

const _token = 'firebase-token';
const _sessionId = 'backend-session';

Map<String, dynamic> _itemJson(String id, {bool isRead = false}) => {
      'id': id,
      'type': 'session_reminder',
      'title': 'تذكير بجلسة قادمة',
      'body': 'ستبدأ الجلسة قريباً حسب جدول الحلقة.',
      'source_ref': {
        'type': 'session',
        'id': '22222222-2222-2222-2222-222222222222',
      },
      'created_at': '2026-10-25T18:30:00Z',
      'is_read': isRead,
    };

class _QueuedResponse {
  const _QueuedResponse(this.statusCode, this.body);

  final int statusCode;
  final Map<String, dynamic> body;
}

/// Captures requests and serves queued responses (queue_api_client_test
/// adapter pattern).
class _Adapter implements HttpClientAdapter {
  _Adapter(this.requests, this.responses);

  final List<RequestOptions> requests;
  final List<_QueuedResponse> responses;

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<List<int>>? requestStream,
    Future<dynamic>? cancelFuture,
  ) async {
    requests.add(options);
    final response = responses.removeAt(0);
    return ResponseBody.fromString(
      jsonEncode(response.body),
      response.statusCode,
      headers: <String, List<String>>{
        Headers.contentTypeHeader: <String>['application/json'],
      },
    );
  }
}

void main() {
  test('list() sends the contract query and decodes the exact page shape',
      () async {
    final requests = <RequestOptions>[];
    final client = DioNotificationRepository(
      Dio()
        ..httpClientAdapter = _Adapter(requests, [
          _QueuedResponse(200, {
            'items': [
              _itemJson('n-1'),
              _itemJson('n-2', isRead: true),
            ],
            'next_cursor': 'opaque-cursor',
            'unread_count': 12,
          }),
        ]),
    );

    final page = await client.list(
      firebaseIdToken: _token,
      sessionId: _sessionId,
    );

    expect(requests.single.path, '/notifications');
    expect(requests.single.method, 'GET');
    expect(requests.single.queryParameters['limit'], 20,
        reason: 'contract default limit');
    expect(requests.single.queryParameters.containsKey('cursor'), isFalse);
    expect(requests.single.headers['Authorization'], 'Bearer $_token');
    expect(requests.single.headers['X-Halaqaty-Session-ID'], _sessionId);

    expect(page.unreadCount, 12);
    expect(page.nextCursor, 'opaque-cursor');
    expect(page.items, hasLength(2));
    expect(page.items.first.id, 'n-1');
    expect(page.items.first.type, 'session_reminder');
    expect(page.items.first.title, 'تذكير بجلسة قادمة');
    expect(page.items.first.source.type, 'session');
    expect(page.items.first.source.id, '22222222-2222-2222-2222-222222222222');
    expect(page.items.first.createdAt, DateTime.utc(2026, 10, 25, 18, 30));
    expect(page.items.first.isRead, isFalse);
    expect(page.items.last.isRead, isTrue);
  });

  test('list() follows a cursor and clamps limit into the 1..50 range',
      () async {
    final requests = <RequestOptions>[];
    final client = DioNotificationRepository(
      Dio()
        ..httpClientAdapter = _Adapter(requests, [
          _QueuedResponse(200, {
            'items': <Map<String, dynamic>>[],
            'next_cursor': null,
            'unread_count': 0,
          }),
        ]),
    );

    final page = await client.list(
      firebaseIdToken: _token,
      sessionId: _sessionId,
      cursor: 'opaque',
      limit: 999,
    );

    expect(requests.single.queryParameters['cursor'], 'opaque');
    expect(requests.single.queryParameters['limit'], 50,
        reason: 'openapi maximum: 50; higher limits would 400');
    expect(page.nextCursor, isNull);
    expect(page.unreadCount, 0);
  });

  test('mark-read endpoints return the authoritative unread count', () async {
    final requests = <RequestOptions>[];
    final id = 'aaaaaaaa-2222-2222-2222-222222222222';
    final client = DioNotificationRepository(
      Dio()
        ..httpClientAdapter = _Adapter(requests, [
          _QueuedResponse(200, {'unread_count': 3}),
          _QueuedResponse(200, {'unread_count': 0}),
        ]),
    );

    final afterOne = await client.markRead(
      firebaseIdToken: _token,
      sessionId: _sessionId,
      notificationId: id,
    );
    final afterAll = await client.markAllRead(
      firebaseIdToken: _token,
      sessionId: _sessionId,
    );

    expect(requests[0].method, 'PUT');
    expect(requests[0].path, '/notifications/$id/read');
    expect(requests[1].method, 'PUT');
    expect(requests[1].path, '/notifications/read-all');
    expect(afterOne, 3);
    expect(afterAll, 0);
  });

  test('preferences read and update use the contract shapes', () async {
    final requests = <RequestOptions>[];
    final client = DioNotificationRepository(
      Dio()
        ..httpClientAdapter = _Adapter(requests, [
          _QueuedResponse(200, {
            'types': [
              {
                'type': 'session_reminder',
                'enabled': true,
                'configurable': false,
                'default_enabled': true,
              },
              {
                'type': 'group_message',
                'enabled': false,
                'configurable': true,
                'default_enabled': true,
              },
            ],
            'quiet_hours': {
              'enabled': true,
              'start_local': '22:00',
              'end_local': '07:00',
              'timezone': 'Africa/Cairo',
            },
          }),
          _QueuedResponse(200, {
            'types': [
              {
                'type': 'group_message',
                'enabled': true,
                'configurable': true,
                'default_enabled': true,
              },
            ],
            'quiet_hours': {
              'enabled': false,
              'start_local': '22:00',
              'end_local': '07:00',
              'timezone': 'Africa/Cairo',
            },
          }),
        ]),
    );

    final preferences = await client.getPreferences(
      firebaseIdToken: _token,
      sessionId: _sessionId,
    );
    expect(preferences.types, hasLength(2));
    expect(preferences.types.first.type, 'session_reminder');
    expect(preferences.types.first.configurable, isFalse);
    expect(preferences.quietHours.timezone, 'Africa/Cairo');
    expect(preferences.quietHours.startLocal, '22:00');

    final updated = await client.updatePreferences(
      firebaseIdToken: _token,
      sessionId: _sessionId,
      request: UpdateNotificationPreferencesRequest(
        types: [
          (type: 'group_message', enabled: true),
        ],
        quietHours: const NotificationQuietHours(
          enabled: false,
          startLocal: '22:00',
          endLocal: '07:00',
          timezone: 'Africa/Cairo',
        ),
      ),
    );
    expect(requests[1].method, 'PUT');
    expect(requests[1].path, '/notifications/preferences');
    expect(requests[1].data, {
      'types': [
        {'type': 'group_message', 'enabled': true},
      ],
      'quiet_hours': {
        'enabled': false,
        'start_local': '22:00',
        'end_local': '07:00',
      },
    });
    expect(updated.quietHours.enabled, isFalse);
  });
}
