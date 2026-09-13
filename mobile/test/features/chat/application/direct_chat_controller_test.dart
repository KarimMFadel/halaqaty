import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';

const _peerId = '22222222-2222-2222-2222-222222222222';

ChatMessage _directMessage(String id,
        {ChatMessageType type = ChatMessageType.text, String? mediaUrl}) =>
    ChatMessage(
      id: id,
      senderId: '33333333-3333-3333-3333-333333333333',
      circleId: null,
      dmPeerId: _peerId,
      content: 'سلام',
      type: type,
      sentAt: DateTime.utc(2026, 9, 3),
      deliveryStatus: ChatDeliveryStatus.delivered,
      mediaUrl: mediaUrl,
    );

void main() {
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

class _FakeDirectApi extends ChatApiClient {
  _FakeDirectApi() : super(Dio());

  final pages = <ChatMessagePage>[];
  final sentPeerIds = <String>[];
  Object? openError;
  Object? sendError;

  @override
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async {
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
