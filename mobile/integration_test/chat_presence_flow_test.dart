import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/direct_chat_screen.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_status_widgets.dart';
import 'package:integration_test/integration_test.dart';

import '../test/helpers/stub_auth_notifier.dart';

/// T071 acceptance: local client projections remain truthful while the
/// backend owns durable delivery and presence authorization.
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('group and direct delivery, reads, typing stop, and expiry',
      (tester) async {
    final api = _PresenceApi();
    final presence = ChatPresenceController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'me'),
    );
    addTearDown(presence.dispose);

    await tester.pumpWidget(MaterialApp(
      home: Column(
        children: ChatDeliveryStatus.values
            .map((status) => ChatDeliveryStatusView(status: status))
            .toList(),
      ),
    ));
    for (final status in ChatDeliveryStatus.values) {
      expect(find.text(_label(status)), findsOneWidget);
    }

    final groupMessage = _message(circleId: 'circle-1');
    final directMessage = _message(dmPeerId: 'peer-1');
    await presence.markGroupMessageRead(groupMessage, 'circle-1');
    await presence.markDirectMessageRead(directMessage, 'peer-1');
    expect(api.groupReads, ['circle-1:message-1']);
    expect(api.directReads, ['peer-1:message-1']);

    final expiry = DateTime.now().add(const Duration(milliseconds: 40));
    presence.handleRealtimeEvent(ChatTypingEvent(
      eventId: 'group-start',
      userId: 'group-peer',
      circleId: 'circle-1',
      isTyping: true,
      expiresAt: expiry,
    ));
    presence.handleRealtimeEvent(ChatTypingEvent(
      eventId: 'direct-start',
      userId: 'peer-1',
      dmPeerId: 'peer-1',
      isTyping: true,
      expiresAt: expiry,
    ));
    expect(presence.state.typing, hasLength(2));

    presence.handleRealtimeEvent(ChatTypingEvent(
      eventId: 'group-stop',
      userId: 'group-peer',
      circleId: 'circle-1',
      isTyping: false,
      expiresAt: expiry,
    ));
    expect(presence.state.typing.keys, isNot(contains('circle-1:group-peer')));
    await tester.pump(const Duration(milliseconds: 80));
    expect(presence.state.typing, isEmpty);
  });

  testWidgets('group and direct transports carry reads, typing, and expiry',
      (tester) async {
    final server = await _PresenceLoopbackServer.start();
    addTearDown(server.close);
    final client = WebSocketChatRealtimeClient(server.dio,
        heartbeatInterval: const Duration(minutes: 1));
    addTearDown(client.dispose);
    final presence = ChatPresenceController(
      _PresenceApi(),
      () async => (token: 'token', sessionId: 'session', userId: 'me'),
    );
    addTearDown(presence.dispose);

    final groupEvents = <ChatRealtimeEvent>[];
    final directEvents = <ChatRealtimeEvent>[];
    final groupSubscription = client
        .circleChatEvents('circle-1',
            token: 'token', backendSessionId: 'session')
        .listen((event) {
      groupEvents.add(event);
      presence.handleRealtimeEvent(event);
    });
    final directSubscription = client
        .directChatEvents('peer-1', token: 'token', backendSessionId: 'session')
        .listen((event) {
      directEvents.add(event);
      presence.handleRealtimeEvent(event);
    });
    addTearDown(groupSubscription.cancel);
    addTearDown(directSubscription.cancel);

    await _until(
        () => server.groupSocket != null && server.directSocket != null);
    final expiry = DateTime.now().add(const Duration(milliseconds: 40));
    server.sendGroup(_frame(
      type: ChatRealtimeTypes.messageRead,
      eventId: 'group-read',
      payload: {
        'message_id': 'group-message',
        'reader_id': 'group-peer',
        'read_at': DateTime.now().toUtc().toIso8601String(),
      },
    ));
    server.sendDirect(_frame(
      type: ChatRealtimeTypes.messageRead,
      eventId: 'direct-read',
      payload: {
        'message_id': 'direct-message',
        'reader_id': 'peer-1',
        'read_at': DateTime.now().toUtc().toIso8601String(),
      },
    ));
    server.sendGroup(_typingFrame('group-start', 'group-peer', expiry,
        circleId: 'circle-1'));
    server.sendDirect(
        _typingFrame('direct-start', 'peer-1', expiry, dmPeerId: 'peer-1'));

    await _until(() => groupEvents.length == 2 && directEvents.length == 2);
    expect(groupEvents.first, isA<ChatMessageReadEvent>());
    expect(directEvents.first, isA<ChatMessageReadEvent>());
    expect(presence.state.typing, hasLength(2));

    await client.sendTyping(circleId: 'circle-1', isTyping: true);
    await client.sendTyping(circleId: 'circle-1', isTyping: false);
    await client.sendTyping(dmPeerId: 'peer-1', isTyping: true);
    await client.sendTyping(dmPeerId: 'peer-1', isTyping: false);
    await _until(() => server.typingFrames.length == 4);
    expect(
        server.typingFrames.map((frame) => frame['payload']),
        containsAll([
          {'circle_id': 'circle-1', 'dm_peer_id': null, 'is_typing': true},
          {'circle_id': 'circle-1', 'dm_peer_id': null, 'is_typing': false},
          {'circle_id': null, 'dm_peer_id': 'peer-1', 'is_typing': true},
          {'circle_id': null, 'dm_peer_id': 'peer-1', 'is_typing': false},
        ]));

    server.sendGroup(_typingFrame('group-stop', 'group-peer', expiry,
        circleId: 'circle-1', isTyping: false));
    await _until(
        () => !presence.state.typing.containsKey('circle-1:group-peer'));
    await tester.pump(const Duration(milliseconds: 80));
    expect(presence.state.typing, isEmpty);
  });

  testWidgets('group and direct screens submit reads and render presence',
      (tester) async {
    final api = _ScreenApi();
    final presence = ChatPresenceController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'me'),
    );
    final group = GroupChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'me'),
      realtime: _Realtime(),
      presence: presence,
    );
    await tester.pumpWidget(ProviderScope(
      overrides: [
        authControllerProvider.overrideWith((_) => StubAuthNotifier(
              initialState: AuthState(
                status: AuthStatus.authenticated,
                sessionId: 'session',
                user: BackendUser(
                  id: 'me',
                  firebaseUid: 'firebase-me',
                  preferredLanguage: 'en',
                  createdAt: DateTime.utc(2026),
                ),
              ),
            )),
        groupChatControllerProvider('circle-1').overrideWith((_) => group),
      ],
      child: const MaterialApp(home: GroupChatScreen(circleId: 'circle-1')),
    ));
    await tester.pumpAndSettle();
    expect(api.groupReads, ['circle-1:incoming']);
    expect(find.text('Read'), findsOneWidget);

    group.handleRealtimeEvent(ChatTypingEvent(
      eventId: 'typing',
      userId: 'peer',
      circleId: 'circle-1',
      isTyping: true,
      expiresAt: DateTime.now().add(const Duration(seconds: 5)),
    ));
    await tester.pump();
    expect(find.text('peer typing…'), findsOneWidget);

    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pumpAndSettle();

    final direct = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'me'),
      presence: presence,
    );
    await tester.pumpWidget(ProviderScope(
      overrides: [
        authControllerProvider.overrideWith((_) => StubAuthNotifier(
              initialState: AuthState(
                status: AuthStatus.authenticated,
                sessionId: 'session',
                user: BackendUser(
                  id: 'me',
                  firebaseUid: 'firebase-me',
                  preferredLanguage: 'en',
                  createdAt: DateTime.utc(2026),
                ),
              ),
            )),
        directChatControllerProvider('peer-1').overrideWith((_) => direct),
      ],
      child: const MaterialApp(home: DirectChatScreen(peerId: 'peer-1')),
    ));
    await tester.pumpAndSettle();
    expect(api.directReads, ['peer-1:direct-incoming']);
    expect(find.text('Read'), findsOneWidget);

    final expiry = DateTime.now().add(const Duration(seconds: 5));
    direct.handleRealtimeEvent(ChatTypingEvent(
      eventId: 'direct-typing',
      userId: 'peer-1',
      dmPeerId: 'peer-1',
      isTyping: true,
      expiresAt: expiry,
    ));
    expect(presence.typingUserIds, contains('peer-1'));
    await tester.pump();
    expect(find.text('peer-1 typing…'), findsOneWidget);
    direct.handleRealtimeEvent(ChatTypingEvent(
      eventId: 'direct-stop',
      userId: 'peer-1',
      dmPeerId: 'peer-1',
      isTyping: false,
      expiresAt: expiry,
    ));
    await tester.pump();
    expect(find.text('peer-1 typing…'), findsNothing);
  });
}

String _label(ChatDeliveryStatus status) =>
    status.name[0].toUpperCase() + status.name.substring(1);

ChatMessage _message({String? circleId, String? dmPeerId}) => ChatMessage(
      id: 'message-1',
      senderId: 'peer-1',
      circleId: circleId,
      dmPeerId: dmPeerId,
      content: 'message',
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );

class _PresenceApi extends ChatApiClient {
  _PresenceApi() : super(Dio());

  final groupReads = <String>[];
  final directReads = <String>[];

  @override
  Future<void> markMessageRead({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
    required String idempotencyKey,
  }) async =>
      groupReads.add('$circleId:$messageId');

  @override
  Future<void> markDirectMessageRead({
    required String token,
    required String sessionId,
    required String userId,
    required String messageId,
    required String idempotencyKey,
  }) async =>
      directReads.add('$userId:$messageId');
}

class _ScreenApi extends _PresenceApi {
  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async =>
      ChatMessagePage(messages: [
        ChatMessage(
          id: 'own',
          senderId: 'me',
          circleId: circleId,
          content: 'own',
          type: ChatMessageType.text,
          sentAt: DateTime.utc(2026),
          deliveryStatus: ChatDeliveryStatus.read,
        ),
        ChatMessage(
          id: 'incoming',
          senderId: 'peer',
          circleId: circleId,
          content: 'hello',
          type: ChatMessageType.text,
          sentAt: DateTime.utc(2026),
          deliveryStatus: ChatDeliveryStatus.delivered,
        ),
      ], hasMore: false);

  @override
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async =>
      ChatMessagePage(messages: [
        ChatMessage(
          id: 'direct-own',
          senderId: 'me',
          circleId: null,
          dmPeerId: userId,
          content: 'own',
          type: ChatMessageType.text,
          sentAt: DateTime.utc(2026),
          deliveryStatus: ChatDeliveryStatus.read,
        ),
        ChatMessage(
          id: 'direct-incoming',
          senderId: userId,
          circleId: null,
          dmPeerId: userId,
          content: 'incoming',
          type: ChatMessageType.text,
          sentAt: DateTime.utc(2026),
          deliveryStatus: ChatDeliveryStatus.delivered,
        ),
      ], hasMore: false);
}

class _Realtime implements ChatRealtimeClient {
  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
          {required String token, required String backendSessionId}) =>
      const Stream.empty();

  @override
  Future<void> dispose() async {}
}

class _PresenceLoopbackServer {
  _PresenceLoopbackServer(this._http);

  final HttpServer _http;
  final sockets = <WebSocket>[];
  WebSocket? groupSocket;
  WebSocket? directSocket;
  final typingFrames = <Map<String, dynamic>>[];

  static Future<_PresenceLoopbackServer> start() async {
    final http = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final server = _PresenceLoopbackServer(http);
    http.listen((request) async {
      if (request.method == 'POST' &&
          request.uri.path == '/api/v1/realtime/tickets') {
        request.response.headers.contentType = ContentType.json;
        request.response.write(jsonEncode({'token': 'ticket'}));
        await request.response.close();
        return;
      }
      if (!WebSocketTransformer.isUpgradeRequest(request)) return;
      final socket = await WebSocketTransformer.upgrade(request);
      server.sockets.add(socket);
      if (server.groupSocket != null && socket != server.groupSocket) {
        server.directSocket = socket;
      }
      socket.listen((raw) {
        final frame = jsonDecode(raw as String) as Map<String, dynamic>;
        if (frame['action'] == 'subscribe') {
          server.groupSocket = socket;
          if (server.sockets.length == 2) {
            server.directSocket =
                server.sockets.firstWhere((candidate) => candidate != socket);
          }
          socket
              .add(jsonEncode({'type': 'subscribed', 'topic': frame['topic']}));
        }
        if (frame['type'] == 'cmd.chat.typing') {
          server.typingFrames.add(frame);
        }
      });
    });
    return server;
  }

  Dio get dio =>
      Dio(BaseOptions(baseUrl: 'http://127.0.0.1:${_http.port}/api/v1'));

  void sendGroup(Map<String, dynamic> frame) =>
      groupSocket!.add(jsonEncode(frame));

  void sendDirect(Map<String, dynamic> frame) =>
      directSocket!.add(jsonEncode(frame));

  Future<void> close() => _http.close(force: true);
}

Future<void> _until(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 2));
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) {
      fail('Timed out waiting for transport');
    }
    await Future<void>.delayed(const Duration(milliseconds: 5));
  }
}

Map<String, dynamic> _frame({
  required String type,
  required String eventId,
  required Map<String, dynamic> payload,
}) =>
    {'type': type, 'event_id': eventId, 'payload': payload};

Map<String, dynamic> _typingFrame(
        String eventId, String userId, DateTime expiry,
        {String? circleId, String? dmPeerId, bool isTyping = true}) =>
    _frame(
      type: ChatRealtimeTypes.typing,
      eventId: eventId,
      payload: {
        'user_id': userId,
        'circle_id': circleId,
        'dm_peer_id': dmPeerId,
        'is_typing': isTyping,
        'expires_at': expiry.toUtc().toIso8601String(),
      },
    );
