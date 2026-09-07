import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';

const _token = 'firebase-token';
const _backendSessionId = 'backend-session';
const _circleId = '22222222-2222-2222-2222-222222222222';
const _messageId = '11111111-1111-1111-1111-111111111111';

Map<String, dynamic> _messageJson({
  String id = _messageId,
  String deliveryStatus = 'delivered',
}) =>
    {
      'id': id,
      'circle_id': _circleId,
      'sender_id': '33333333-3333-3333-3333-333333333333',
      'sender_name': 'مريم',
      'message_type': 'text',
      'content': 'السلام عليكم',
      'sent_at': '2026-09-03T12:00:00Z',
      'delivery_status': deliveryStatus,
    };

void main() {
  group('ChatApiClient', () {
    test('lists group messages with auth headers and cursor query', () async {
      final requests = <RequestOptions>[];
      final client = ChatApiClient(
        Dio()
          ..httpClientAdapter = _ChatAdapter(
            requests,
            [
              _ChatResponse.ok(200, {
                'data': [
                  _messageJson(),
                  _messageJson(id: '44444444-4444-4444-4444-444444444444'),
                ],
                'has_more': true,
                'next_before': '55555555-5555-5555-5555-555555555555',
              }),
            ],
          ),
      );

      final page = await client.listMessages(
        token: _token,
        sessionId: _backendSessionId,
        circleId: _circleId,
        limit: 25,
        before: '55555555-5555-5555-5555-555555555555',
      );

      final request = requests.single;
      expect(request.path, '/circles/$_circleId/messages');
      expect(request.method, 'GET');
      expect(request.headers['Authorization'], 'Bearer $_token');
      expect(request.headers['X-Halaqaty-Session-ID'], _backendSessionId);
      expect(request.queryParameters['limit'], 25);
      expect(request.queryParameters['before'],
          '55555555-5555-5555-5555-555555555555');
      expect(page.messages, hasLength(2));
      expect(page.messages.first.type, ChatMessageType.text);
      expect(page.hasMore, isTrue);
      expect(page.nextBefore, '55555555-5555-5555-5555-555555555555');
    });

    test('omits pagination query when no cursor is supplied', () async {
      final requests = <RequestOptions>[];
      final client = ChatApiClient(
        Dio()
          ..httpClientAdapter = _ChatAdapter(
            requests,
            [
              _ChatResponse.ok(200, {
                'data': [_messageJson()],
                'has_more': false,
                'next_before': null,
              }),
            ],
          ),
      );

      await client.listMessages(
        token: _token,
        sessionId: _backendSessionId,
        circleId: _circleId,
      );

      expect(requests.single.queryParameters, isEmpty);
    });

    test('sends a text message with the idempotency key header', () async {
      final requests = <RequestOptions>[];
      final client = ChatApiClient(
        Dio()
          ..httpClientAdapter = _ChatAdapter(
            requests,
            [
              _ChatResponse.ok(201, _messageJson(deliveryStatus: 'delivered')),
            ],
          ),
      );

      final message = await client.sendTextMessage(
        token: _token,
        sessionId: _backendSessionId,
        circleId: _circleId,
        content: 'مرحبا',
        idempotencyKey: 'send-retry-key',
      );

      final request = requests.single;
      expect(request.path, '/circles/$_circleId/messages');
      expect(request.method, 'POST');
      expect(request.headers['Authorization'], 'Bearer $_token');
      expect(request.headers['X-Halaqaty-Session-ID'], _backendSessionId);
      expect(request.headers['Idempotency-Key'], 'send-retry-key');
      expect(request.data, {
        'message_type': 'text',
        'content': 'مرحبا',
      });
      expect(message.id, _messageId);
      expect(message.deliveryStatus, ChatDeliveryStatus.delivered);
    });

    test('maps contract error envelopes to ChatApiException', () async {
      final cases = <({
        int statusCode,
        String code,
        String message,
      })>[
        (
          statusCode: 403,
          code: 'ERR_FORBIDDEN',
          message: 'You are not a member of this circle.',
        ),
        (
          statusCode: 429,
          code: 'ERR_RATE_LIMITED',
          message: 'Too many messages.',
        ),
      ];

      for (final testCase in cases) {
        final client = ChatApiClient(
          Dio()
            ..httpClientAdapter = _ChatAdapter(
              <RequestOptions>[],
              [
                _ChatResponse.ok(testCase.statusCode, {
                  'error': {'code': testCase.code, 'message': testCase.message},
                }),
              ],
            ),
        );

        try {
          await client.listMessages(
            token: _token,
            sessionId: _backendSessionId,
            circleId: _circleId,
          );
          fail('Expected ChatApiException for ${testCase.statusCode}.');
        } on ChatApiException catch (error) {
          expect(error.statusCode, testCase.statusCode);
          expect(error.code, testCase.code);
          expect(error.message, testCase.message);
        }
      }
    });
  });
}

class _ChatResponse {
  const _ChatResponse(this.statusCode, this.body);

  factory _ChatResponse.ok(int statusCode, Map<String, dynamic> body) =>
      _ChatResponse(statusCode, body);

  final int statusCode;
  final Map<String, dynamic> body;
}

class _ChatAdapter implements HttpClientAdapter {
  _ChatAdapter(this.requests, this.responses);

  final List<RequestOptions> requests;
  final List<_ChatResponse> responses;

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
