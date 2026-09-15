import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

const _peerId = '22222222-2222-2222-2222-222222222222';

ChatMessage _directMessage(String id,
        {ChatMessageType type = ChatMessageType.text,
        String? mediaUrl,
        String dmPeerId = _peerId,
        ChatDeliveryStatus status = ChatDeliveryStatus.delivered}) =>
    ChatMessage(
      id: id,
      senderId: '33333333-3333-3333-3333-333333333333',
      circleId: null,
      dmPeerId: dmPeerId,
      content: 'سلام',
      type: type,
      sentAt: DateTime.utc(2026, 9, 3),
      deliveryStatus: status,
      mediaUrl: mediaUrl,
    );

void main() {
  test('projects incoming direct messages addressed to the current user',
      () async {
    final api = _FakeDirectApi()
      ..pages.add(const ChatMessagePage(messages: [], hasMore: false));
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    addTearDown(controller.dispose);
    await controller.open(_peerId);
    controller.handleRealtimeEvent(ChatMessageEvent(
      eventId: 'incoming-event',
      message: ChatMessage(
        id: 'incoming',
        senderId: _peerId,
        circleId: null,
        dmPeerId: 'user',
        content: 'سلام',
        type: ChatMessageType.text,
        sentAt: DateTime.utc(2026, 9, 3),
        deliveryStatus: ChatDeliveryStatus.delivered,
      ),
    ));
    expect(
        controller.state.messages.map((message) => message.id), ['incoming']);
  });

  test('active read event refreshes the peer history authoritatively',
      () async {
    final api = _FakeDirectApi()
      ..pages.add(ChatMessagePage(
          messages: [_directMessage('readable')], hasMore: false))
      ..pages.add(ChatMessagePage(messages: [
        _directMessage('readable', status: ChatDeliveryStatus.read)
      ], hasMore: false));
    final realtime = _FakeDirectRealtimeClient();
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
      realtime: realtime,
    );
    addTearDown(controller.dispose);

    await controller.open(_peerId);
    realtime.emit(ChatMessageReadEvent(
      eventId: 'read-active',
      messageId: 'readable',
      readerId: _peerId,
      readAt: DateTime.utc(2026, 9, 3, 12),
    ));
    await Future<void>.delayed(Duration.zero);

    expect(controller.state.messages.single.deliveryStatus,
        ChatDeliveryStatus.read);
    expect(api.listCalls, 2);
  });

  test('cross-peer direct events do not alter the active thread', () async {
    const otherPeerId = '44444444-4444-4444-4444-444444444444';
    final api = _FakeDirectApi()
      ..pages.add(ChatMessagePage(
          messages: [_directMessage('active')], hasMore: false));
    final realtime = _FakeDirectRealtimeClient();
    final presence = ChatPresenceController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
      realtime: realtime,
      presence: presence,
    );
    addTearDown(() {
      controller.dispose();
      presence.dispose();
    });

    await controller.open(_peerId);
    realtime
      ..emit(ChatMessageEvent(
          eventId: 'other-message',
          message: _directMessage('other-message', dmPeerId: otherPeerId)))
      ..emit(ChatMessageReadEvent(
          eventId: 'other-read',
          messageId: 'other-message',
          readerId: otherPeerId,
          readAt: DateTime.utc(2026, 9, 3, 12)))
      ..emit(ChatTypingEvent(
          eventId: 'other-typing',
          userId: otherPeerId,
          dmPeerId: otherPeerId,
          isTyping: true,
          expiresAt: DateTime.utc(2026, 9, 3, 12, 0, 5)));
    await Future<void>.delayed(Duration.zero);

    expect(controller.state.messages.map((message) => message.id), ['active']);
    expect(api.listCalls, 1);
    expect(presence.state.readReceipts, isEmpty);
    expect(presence.state.typing, isEmpty);
  });

  test('projects deletion events as an idempotent redaction', () async {
    final message =
        _directMessage('deleted', mediaUrl: 'https://media').copyWith(
      pinnedAt: DateTime.utc(2026, 9, 3, 12),
      replyPreview: const ChatReplyPreviewProjection(
        id: 'reply',
        senderName: 'A',
        preview: 'quoted',
        deleted: false,
      ),
    );
    final api = _FakeDirectApi()
      ..pages.add(ChatMessagePage(messages: [message], hasMore: false));
    final realtime = _FakeDirectRealtimeClient();
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
      realtime: realtime,
    );
    addTearDown(controller.dispose);

    await controller.open(_peerId);
    final event = ChatMessageDeletedEvent(
      eventId: 'deleted-event',
      messageId: 'deleted',
      deletedAt: DateTime.utc(2026, 9, 3, 12, 1),
    );
    realtime.emit(event);
    realtime.emit(event);
    await Future<void>.delayed(Duration.zero);

    final deleted = controller.state.messages.single;
    expect(deleted.content, isEmpty);
    expect(deleted.mediaUrl, isNull);
    expect(deleted.pinnedAt, isNull);
    expect(deleted.replyPreview?.preview, isEmpty);
    expect(deleted.replyPreview?.deleted, isTrue);
    expect(deleted.deletedAt, event.deletedAt);
  });

  test('dispose cancels the active direct realtime subscription', () async {
    final api = _FakeDirectApi()
      ..pages.add(const ChatMessagePage(messages: [], hasMore: false));
    final realtime = _FakeDirectRealtimeClient();
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
      realtime: realtime,
    );

    await controller.open(_peerId);
    controller.dispose();
    await Future<void>.delayed(Duration.zero);

    expect(realtime.cancelled, isTrue);
  });

  test(
      'opens a direct authenticated realtime stream for read and typing updates',
      () async {
    final api = _FakeDirectApi()
      ..pages.add(const ChatMessagePage(messages: [], hasMore: false));
    final realtime = _FakeDirectRealtimeClient();
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
      realtime: realtime,
    );
    addTearDown(controller.dispose);

    await controller.open(_peerId);
    await controller.setTyping(true);

    expect(realtime.directPeerIds, [_peerId]);
    expect(realtime.typingPeers, [_peerId]);
  });

  test('open restores eligible pair history and send uses the peer route',
      () async {
    final api = _FakeDirectApi()
      ..pages.add(const ChatMessagePage(messages: [], hasMore: false));
    final controller = DirectChatController(
      api,
      () async => (
        token: 'token',
        sessionId: 'session',
        userId: '33333333-3333-3333-3333-333333333333'
      ),
    );
    addTearDown(controller.dispose);

    await controller.open(_peerId);
    expect(controller.state.status, DirectChatStatus.ready);
    expect(await controller.sendText('رسالة خاصة'), isTrue);
    expect(api.sentPeerIds, [_peerId]);
  });

  test('a denied pair becomes an access-lost state with safe copy', () async {
    final api = _FakeDirectApi()
      ..openError = const ChatApiException(
          statusCode: 403, code: 'ERR_FORBIDDEN', message: 'private detail');
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    addTearDown(controller.dispose);

    await controller.open(_peerId);
    expect(controller.state.status, DirectChatStatus.accessLost);
    expect(controller.state.errorMessage, isNot(contains('private detail')));
  });

  test('losing eligibility during send clears access and history', () async {
    final api = _FakeDirectApi()
      ..pages.add(ChatMessagePage(
          messages: [_directMessage('existing')], hasMore: false))
      ..sendError = const ChatApiException(
        statusCode: 403,
        code: 'ERR_FORBIDDEN',
        message: 'relationship no longer qualifies',
      );
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    addTearDown(controller.dispose);

    await controller.open(_peerId);
    expect(await controller.sendText('blocked after removal'), isFalse);
    expect(controller.state.status, DirectChatStatus.accessLost);
    expect(controller.state.messages, isEmpty);
    expect(controller.state.errorMessage, 'This conversation is unavailable');
  });

  test('restored history keeps media and older-page continuity', () async {
    final api = _FakeDirectApi()
      ..pages.add(ChatMessagePage(
        messages: [
          _directMessage('media',
              type: ChatMessageType.image, mediaUrl: 'https://media.example/1'),
        ],
        hasMore: true,
        nextBefore: 'older',
      ))
      ..pages.add(ChatMessagePage(
        messages: [_directMessage('older')],
        hasMore: false,
      ));
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    addTearDown(controller.dispose);

    await controller.open(_peerId);
    await controller.loadOlder();

    expect(controller.state.status, DirectChatStatus.ready);
    expect(controller.state.messages.map((message) => message.id),
        containsAll(<String>['media', 'older']));
    expect(
      controller.state.messages
          .singleWhere((message) => message.id == 'media')
          .mediaUrl,
      'https://media.example/1',
    );
  });
}

class _FakeDirectRealtimeClient
    implements ChatRealtimeClient, ChatRealtimePresenceClient {
  _FakeDirectRealtimeClient()
      : _events = StreamController<ChatRealtimeEvent>.broadcast(sync: true) {
    _events.onCancel = () => cancelled = true;
  }

  final directPeerIds = <String>[];
  final typingPeers = <String>[];
  final StreamController<ChatRealtimeEvent> _events;
  bool cancelled = false;

  void emit(ChatRealtimeEvent event) => _events.add(event);

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
          {required String token, required String backendSessionId}) =>
      const Stream.empty();

  @override
  Stream<ChatRealtimeEvent> directChatEvents(String peerId,
      {required String token, required String backendSessionId}) {
    directPeerIds.add(peerId);
    return _events.stream;
  }

  @override
  Future<void> sendTyping(
      {String? circleId, String? dmPeerId, required bool isTyping}) async {
    if (isTyping && dmPeerId != null) typingPeers.add(dmPeerId);
  }

  @override
  Future<void> dispose() => _events.close();
}

class _FakeDirectApi extends ChatApiClient {
  _FakeDirectApi() : super(Dio());

  final pages = <ChatMessagePage>[];
  int listCalls = 0;
  final sentPeerIds = <String>[];
  Object? openError;
  Object? sendError;

  @override
  Future<void> markDirectMessageRead({
    required String token,
    required String sessionId,
    required String userId,
    required String messageId,
    required String idempotencyKey,
  }) async {}

  @override
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async {
    listCalls++;
    if (openError != null) throw openError!;
    return pages.removeAt(0);
  }

  @override
  Future<ChatMessage> sendDirectTextMessage({
    required String token,
    required String sessionId,
    required String userId,
    required String content,
    required String idempotencyKey,
  }) async {
    if (sendError != null) throw sendError!;
    sentPeerIds.add(userId);
    return _directMessage('server-message');
  }
}
