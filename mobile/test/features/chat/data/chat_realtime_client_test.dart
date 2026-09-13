import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';

const _circleId = '22222222-2222-2222-2222-222222222222';

String _frame({
  required String type,
  required String eventId,
  Object? payload,
}) =>
    jsonEncode({
      ChatJsonKeys.type: type,
      ChatJsonKeys.eventId: eventId,
      'occurred_at': '2026-09-03T12:00:00Z',
      ChatJsonKeys.payload: payload,
    });

Map<String, dynamic> _messagePayload({String circleId = _circleId}) => {
      'id': '11111111-1111-1111-1111-111111111111',
      'circle_id': circleId,
      'sender_id': '33333333-3333-3333-3333-333333333333',
      'sender_name': 'مريم',
      'message_type': 'text',
      'content': 'السلام عليكم',
      'sent_at': '2026-09-03T12:00:00Z',
      'delivery_status': 'delivered',
    };

void main() {
  test('decodes chat.message into a ChatMessageEvent for this circle', () {
    final decoder = ChatRealtimeEventDecoder(_circleId);

    final event = decoder.decode(_frame(
      type: ChatRealtimeTypes.message,
      eventId: 'event-1',
      payload: _messagePayload(),
    ));

    expect(event, isA<ChatMessageEvent>());
    final messageEvent = event as ChatMessageEvent;
    expect(messageEvent.eventId, 'event-1');
    expect(messageEvent.message.id, '11111111-1111-1111-1111-111111111111');
    expect(messageEvent.message.content, 'السلام عليكم');
    expect(messageEvent.message.deliveryStatus, ChatDeliveryStatus.delivered);
  });

  test('drops duplicate event ids, malformed frames, and foreign projections',
      () {
    final decoder = ChatRealtimeEventDecoder(_circleId);

    final first = decoder.decode(_frame(
      type: ChatRealtimeTypes.message,
      eventId: 'event-1',
      payload: _messagePayload(),
    ));
    final duplicate = decoder.decode(_frame(
      type: ChatRealtimeTypes.message,
      eventId: 'event-1',
      payload: _messagePayload(),
    ));
    final malformed = decoder.decode('not-json');
    final noType = decoder.decode(jsonEncode(
        {ChatJsonKeys.eventId: 'event-2', ChatJsonKeys.payload: {}}));
    final foreign = decoder.decode(_frame(
      type: ChatRealtimeTypes.message,
      eventId: 'event-3',
      payload: {..._messagePayload(), 'circle_id': 'other-circle'},
    ));

    expect(first, isNotNull);
    expect(duplicate, isNull);
    expect(malformed, isNull);
    expect(noType, isNull);
    expect(foreign, isNull);
  });

  test('reports other chat event types as unknown for reconciliation', () {
    final decoder = ChatRealtimeEventDecoder(_circleId);

    final typing = decoder.decode(_frame(
      type: 'chat.typing',
      eventId: 'event-4',
      payload: {
        'user_id': 'u',
        'circle_id': _circleId,
        'is_typing': true,
      },
    ));
    final unreadableMessage = decoder.decode(_frame(
      type: ChatRealtimeTypes.message,
      eventId: 'event-5',
      payload: 'not-an-object',
    ));

    expect(typing, isA<ChatUnknownEvent>());
    expect((typing as ChatUnknownEvent).type, 'chat.typing');
    expect(unreadableMessage, isA<ChatUnknownEvent>());
  });

  test('a revoked session stops reconnecting and ends the stream', () async {
    final server = await _LoopbackChatServer.start()
      ..ticketStatus = 401;
    addTearDown(server.close);
    final client = WebSocketChatRealtimeClient(server.dio);
    addTearDown(client.dispose);

    final done = Completer<void>();
    client
        .circleChatEvents(_circleId, token: 't', backendSessionId: 's')
        .listen((_) {}, onDone: done.complete);

    // Terminal auth failure: the subscription ends instead of retrying the
    // ticket forever; REST reconciliation surfaces the access loss.
    await done.future.timeout(const Duration(seconds: 3));
    expect(server.ticketAttempts, 1);
  });

  test('reconnects after a socket closes and keeps the decoder state',
      () async {
    final server = await _LoopbackChatServer.start();
    server.closeAfterSubscribe = true;
    addTearDown(server.close);
    final client = WebSocketChatRealtimeClient(server.dio,
        heartbeatInterval: const Duration(milliseconds: 20));
    addTearDown(client.dispose);
    final reconnected = Completer<ChatReconnectedEvent>();
    final subscription = client
        .circleChatEvents(_circleId, token: 't', backendSessionId: 's')
        .listen((event) {
      if (event is ChatReconnectedEvent && !reconnected.isCompleted) {
        reconnected.complete(event);
      }
    });

    await reconnected.future.timeout(const Duration(seconds: 3));
    expect(server.connectionCount, greaterThanOrEqualTo(2));
    await subscription.cancel();
  });

  test('overlapping circle connections tear down independently', () async {
    final server = await _LoopbackChatServer.start();
    addTearDown(server.close);
    final client = WebSocketChatRealtimeClient(server.dio);
    addTearDown(client.dispose);

    final receivedByA = Completer<ChatRealtimeEvent>();
    final subscriptionA = client
        .circleChatEvents(_circleId, token: 't', backendSessionId: 's')
        .listen(receivedByA.complete);
    final receivedByB = Completer<ChatRealtimeEvent>();
    final subscriptionB = client
        .circleChatEvents('circle-b', token: 't', backendSessionId: 's')
        .listen(receivedByB.complete);

    await _until(() => server.socketsByTopic.length == 2);
    final socketA = server.socketsByTopic['circle.$_circleId']!;
    final socketB = server.socketsByTopic['circle.circle-b']!;
    // Two subscriptions mean two distinct live sockets.
    expect(socketA, isNot(same(socketB)));

    socketA.add(_frame(
      type: ChatRealtimeTypes.message,
      eventId: 'event-a',
      payload: _messagePayload(),
    ));
    expect(await receivedByA.future.timeout(const Duration(seconds: 2)),
        isA<ChatMessageEvent>());

    // Cancelling A's subscription closes only A's socket; B stays live.
    await subscriptionA.cancel();
    await _until(() => server.closedSockets.contains(socketA));
    expect(server.closedSockets, isNot(contains(socketB)));

    socketB.add(_frame(
      type: ChatRealtimeTypes.message,
      eventId: 'event-b',
      payload: _messagePayload(circleId: 'circle-b'),
    ));
    expect(await receivedByB.future.timeout(const Duration(seconds: 2)),
        isA<ChatMessageEvent>());

    await subscriptionB.cancel();
    await _until(() => server.closedSockets.contains(socketB));
  });

  test('dispose closes every live connection', () async {
    final server = await _LoopbackChatServer.start();
    addTearDown(server.close);
    final client = WebSocketChatRealtimeClient(server.dio);

    client
        .circleChatEvents(_circleId, token: 't', backendSessionId: 's')
        .listen((_) {});
    client
        .circleChatEvents('circle-b', token: 't', backendSessionId: 's')
        .listen((_) {});
    await _until(() => server.socketsByTopic.length == 2);

    await client.dispose();

    await _until(() => server.closedSockets.length == 2);
  });

  test('cancelling while the ticket is in flight closes the socket it opens',
      () async {
    final server = await _LoopbackChatServer.start();
    final ticketGate = Completer<void>();
    server.holdTickets = ticketGate;
    addTearDown(server.close);
    final client = WebSocketChatRealtimeClient(server.dio);
    addTearDown(client.dispose);

    final subscription = client
        .circleChatEvents(_circleId, token: 't', backendSessionId: 's')
        .listen((_) {});
    await subscription.cancel();
    ticketGate.complete();

    await _until(() => server.closedSockets.length == 1);
  });
}

/// Polls [condition] until it holds, failing after a short deadline instead
/// of hanging the suite on a never-torn-down connection.
Future<void> _until(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 2));
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) {
      fail('Timed out waiting for the condition to hold');
    }
    await Future<void>.delayed(const Duration(milliseconds: 5));
  }
}

/// Minimal loopback chat backend: issues realtime tickets and records each
/// subscribed WebSocket so tests can assert per-connection teardown.
class _LoopbackChatServer {
  _LoopbackChatServer(this._http);

  final HttpServer _http;
  final socketsByTopic = <String, WebSocket>{};
  final closedSockets = <WebSocket>{};
  int connectionCount = 0;
  bool closeAfterSubscribe = false;

  /// HTTP status returned for realtime ticket requests (200 by default).
  int ticketStatus = 200;
  int ticketAttempts = 0;

  /// When set, ticket responses wait on this completer first.
  Completer<void>? holdTickets;

  static Future<_LoopbackChatServer> start() async {
    final http = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final server = _LoopbackChatServer(http);
    http.listen((request) async {
      if (request.method == 'POST' &&
          request.uri.path == '/api/v1/realtime/tickets') {
        final gate = server.holdTickets;
        if (gate != null) await gate.future;
        server.ticketAttempts++;
        if (server.ticketStatus != 200) {
          request.response.statusCode = server.ticketStatus;
          await request.response.close();
          return;
        }
        request.response.headers.contentType = ContentType.json;
        request.response.write(jsonEncode({'token': 'ticket'}));
        await request.response.close();
        return;
      }
      if (!WebSocketTransformer.isUpgradeRequest(request)) return;
      final socket = await WebSocketTransformer.upgrade(request);
      server.connectionCount++;
      socket.listen((raw) {
        final frame = jsonDecode(raw as String) as Map<String, dynamic>;
        if (frame['action'] == 'subscribe' && frame['topic'] is String) {
          final topic = frame['topic'] as String;
          server.socketsByTopic[topic] = socket;
          socket.add(jsonEncode({'type': 'subscribed', 'topic': topic}));
          if (server.closeAfterSubscribe && server.connectionCount == 1) {
            unawaited(socket.close());
          }
        }
      }, onDone: () => server.closedSockets.add(socket));
    });
    return server;
  }

  Dio get dio =>
      Dio(BaseOptions(baseUrl: 'http://127.0.0.1:${_http.port}/api/v1'));

  Future<void> close() => _http.close(force: true);
}
