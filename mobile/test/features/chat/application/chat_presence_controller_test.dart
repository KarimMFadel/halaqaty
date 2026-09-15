import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

void main() {
  ChatPresenceController buildController({ChatApiClient? api}) =>
      ChatPresenceController(
        api ?? ChatApiClient(Dio()),
        () async => (token: 'token', sessionId: 'session', userId: 'me'),
      );

  test('does not submit a read for the sender own message', () async {
    final api = _PresenceApi();
    final controller = buildController(api: api);
    addTearDown(controller.dispose);

    await controller.markGroupMessageRead(_message(senderId: 'me'), 'circle-1');

    expect(api.circleReads, isEmpty);
  });

  test('submits recipient group and direct reads to their contracted routes',
      () async {
    final api = _PresenceApi();
    final controller = buildController(api: api);
    addTearDown(controller.dispose);

    await controller.markGroupMessageRead(_message(), 'circle-1');
    await controller.markDirectMessageRead(_message(), 'peer-1');

    expect(api.circleReads, contains('circle-1:message-1'));
    expect(api.directReads, contains('peer-1:message-1'));
  });

  test('keeps group read details and deduplicates repeated reader facts', () {
    final controller = buildController();
    addTearDown(controller.dispose);
    final at = DateTime.utc(2026, 1, 1);
    final firstRead = ChatMessageReadEvent(
        eventId: 'read-1',
        messageId: 'message-1',
        readerId: 'reader-1',
        readAt: at);
    controller.handleRealtimeEvent(firstRead);
    controller.handleRealtimeEvent(ChatMessageReadEvent(
        eventId: 'read-2',
        messageId: 'message-1',
        readerId: 'reader-2',
        readAt: at.add(const Duration(minutes: 1))));
    controller.handleRealtimeEvent(firstRead);

    expect(
      controller.state.readReceipts['message-1']
          ?.map((receipt) => (receipt.readerId, receipt.readAt)),
      [
        ('reader-1', at),
        ('reader-2', at.add(const Duration(minutes: 1))),
      ],
    );
  });

  test('typing stop removes the indicator before its expiry', () {
    final controller = buildController();
    addTearDown(controller.dispose);
    final expires = DateTime.now().add(const Duration(seconds: 5));
    controller.handleRealtimeEvent(ChatTypingEvent(
        eventId: 'typing-1',
        userId: 'user-1',
        circleId: 'circle-1',
        isTyping: true,
        expiresAt: expires));
    expect(controller.state.typing, hasLength(1));
    controller.handleRealtimeEvent(ChatTypingEvent(
        eventId: 'typing-stop-1',
        userId: 'user-1',
        circleId: 'circle-1',
        isTyping: false,
        expiresAt: expires));
    expect(controller.state.typing, isEmpty);
  });

  testWidgets('typing expires after the server five-second lifetime',
      (tester) async {
    final controller = buildController();
    addTearDown(controller.dispose);
    controller.handleRealtimeEvent(ChatTypingEvent(
        eventId: 'typing-1',
        userId: 'user-1',
        circleId: 'circle-1',
        isTyping: true,
        expiresAt: DateTime.now().add(const Duration(seconds: 5))));

    await tester.pump(const Duration(seconds: 4));
    expect(controller.state.typing, hasLength(1));
    await tester.pump(const Duration(seconds: 1));
    expect(controller.state.typing, isEmpty);
  });

  test('keeps group and direct typing projections separate', () {
    final controller = buildController();
    addTearDown(controller.dispose);
    final expiry = DateTime.now().add(const Duration(seconds: 5));

    controller.handleRealtimeEvent(ChatTypingEvent(
        eventId: 'group-typing',
        userId: 'user-1',
        circleId: 'circle-1',
        isTyping: true,
        expiresAt: expiry));
    controller.handleRealtimeEvent(ChatTypingEvent(
        eventId: 'direct-typing',
        userId: 'user-1',
        dmPeerId: 'peer-1',
        isTyping: true,
        expiresAt: expiry));

    expect(controller.state.typing, hasLength(2));
  });
}

ChatMessage _message({String senderId = 'other'}) => ChatMessage(
      id: 'message-1',
      senderId: senderId,
      circleId: 'circle-1',
      content: 'text',
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );

class _PresenceApi extends ChatApiClient {
  _PresenceApi() : super(Dio());

  final circleReads = <String>[];
  final directReads = <String>[];

  @override
  Future<void> markMessageRead({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
    required String idempotencyKey,
  }) async =>
      circleReads.add('$circleId:$messageId');

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
