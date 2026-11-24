import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class _InvitationAdapter implements HttpClientAdapter {
  final requests = <RequestOptions>[];

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<List<int>>? requestStream,
    Future<void>? cancelFuture,
  ) async {
    requests.add(options);
    final data = switch (options.path) {
      '/circles/circle-1/invitations' => {
          'id': 'invitation-1',
          'circle_id': 'circle-1',
          'inviter_user_id': 'teacher-1',
          'recipient_user_id': 'recipient-1',
          'role': 'teacher',
          'state': 'pending',
          'created_at': '2026-11-22T12:00:00Z',
        },
      '/circle-invitations/invitation-1/accept' => {
          'id': 'invitation-1',
          'circle_id': 'circle-1',
          'inviter_user_id': 'teacher-1',
          'recipient_user_id': 'recipient-1',
          'role': 'teacher',
          'state': 'accepted',
          'created_at': '2026-11-22T12:00:00Z',
        },
      _ => <String, dynamic>{'data': <dynamic>[]},
    };
    return ResponseBody.fromString(
      jsonEncode(data),
      200,
      headers: {
        Headers.contentTypeHeader: [Headers.jsonContentType]
      },
    );
  }

  @override
  void close({bool force = false}) {}
}

void main() {
  test('invitation API uses recipient-bound routes and auth headers', () async {
    final adapter = _InvitationAdapter();
    final client = CircleApiClient(
      Dio(BaseOptions(baseUrl: 'https://api.example.test'))
        ..httpClientAdapter = adapter,
    );

    final sent = await client.createCircleInvitation(
      firebaseIdToken: 'firebase-token',
      sessionId: 'session-id',
      circleId: 'circle-1',
      recipientUserId: 'recipient-1',
      role: CircleRole.teacher,
    );
    final accepted = await client.acceptCircleInvitation(
      firebaseIdToken: 'firebase-token',
      sessionId: 'session-id',
      invitationId: sent.id,
    );

    expect(sent.role, CircleRole.teacher);
    expect(accepted.state, 'accepted');
    expect(adapter.requests.map((request) => request.path), [
      '/circles/circle-1/invitations',
      '/circle-invitations/invitation-1/accept',
    ]);
    expect(adapter.requests.first.data, {
      'recipient_user_id': 'recipient-1',
      'role': 'teacher',
    });
    for (final request in adapter.requests) {
      expect(request.headers['Authorization'], 'Bearer firebase-token');
      expect(request.headers['X-Halaqaty-Session-ID'], 'session-id');
    }
  });
}
