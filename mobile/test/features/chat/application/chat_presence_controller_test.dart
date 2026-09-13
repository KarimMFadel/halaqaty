import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

void main() {
  ChatPresenceController buildController() => ChatPresenceController(
        ChatApiClient(Dio()),
        () async => (token: 'token', sessionId: 'session'),
      );

  test('deduplicates targeted read facts', () {
    final controller = buildController();
    addTearDown(controller.dispose);
    final at = DateTime.utc(2026, 1, 1);
    final event = ChatMessageReadEvent(
        eventId: 'read-1',
        messageId: 'message-1',
        readerId: 'reader-1',
        readAt: at);
    controller.handleRealtimeEvent(event);
    controller.handleRealtimeEvent(event);
    expect(controller.state.readReceipts['message-1'], hasLength(1));
  });

  test('typing expires and stop removes the indicator', () async {
    final controller = buildController();
    addTearDown(controller.dispose);
    final expires = DateTime.now().add(const Duration(milliseconds: 40));
    controller.handleRealtimeEvent(ChatTypingEvent(
        eventId: 'typing-1',
        userId: 'user-1',
        circleId: 'circle-1',
        isTyping: true,
        expiresAt: expires));
    expect(controller.state.typing, hasLength(1));
    await Future<void>.delayed(const Duration(milliseconds: 80));
    expect(controller.state.typing, isEmpty);
  });
}
