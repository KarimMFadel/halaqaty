import 'package:flutter_test/flutter_test.dart';
import 'package:dio/dio.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_moderation_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

void main() {
  test('allows only own messages inside the ten minute deletion window', () {
    final now = DateTime.utc(2026, 9, 15, 10);
    final controller = ChatModerationController(_Api(), () async => _creds());
    final own = _message('own',
        senderId: 'me', sentAt: now.subtract(const Duration(minutes: 9)));
    final late = _message('late',
        senderId: 'me', sentAt: now.subtract(const Duration(minutes: 11)));
    final other = _message('other',
        senderId: 'other', sentAt: now.subtract(const Duration(minutes: 1)));

    expect(controller.canDelete(own, userId: 'me', now: now), isTrue);
    expect(controller.canDelete(late, userId: 'me', now: now), isFalse);
    expect(controller.canDelete(other, userId: 'me', now: now), isFalse);
    expect(controller.canDelete(other, userId: 'me', isTeacher: true, now: now),
        isTrue);
  });

  test('deletion conflict keeps the message and exposes a safe retry state',
      () async {
    final api = _Api()
      ..deleteError = const ChatApiException(
          statusCode: 409, code: 'ERR_CONFLICT', message: 'late');
    final controller = ChatModerationController(api, () async => _creds());
    final message = _message('m1', senderId: 'me');
    controller.setMessages([message]);

    expect(
        await controller.deleteCircleMessage('circle', message,
            now: message.sentAt.add(const Duration(minutes: 10))),
        isFalse);
    expect(controller.state.messages.single.content, 'hello');
    expect(controller.state.conflict, isTrue);
  });

  test(
      'duplicate deletion events redact content, media, reply preview, and pin once',
      () {
    final controller = ChatModerationController(_Api(), () async => _creds());
    controller
        .setMessages([_message('m1', mediaUrl: 'https://media', pinned: true)]);
    final event = ChatMessageDeletedEvent(
        eventId: 'e1',
        messageId: 'm1',
        deletedAt: DateTime.utc(2026, 9, 15, 10));

    controller.handleDeletionEvent(event);
    controller.handleDeletionEvent(event);

    final deleted = controller.state.messages.single;
    expect(deleted.content, isEmpty);
    expect(deleted.mediaUrl, isNull);
    expect(deleted.replyPreview?.preview, isEmpty);
    expect(deleted.pinnedAt, isNull);
    expect(controller.state.deletedMessageIds, {'m1'});
  });
}

({String token, String sessionId, String userId}) _creds() =>
    (token: 'token', sessionId: 'session', userId: 'me');

ChatMessage _message(String id,
        {String senderId = 'me',
        DateTime? sentAt,
        String? mediaUrl,
        bool pinned = false}) =>
    ChatMessage(
      id: id,
      senderId: senderId,
      circleId: 'circle',
      content: 'hello',
      type: ChatMessageType.text,
      sentAt: sentAt ?? DateTime.utc(2026, 9, 15, 9, 55),
      deliveryStatus: ChatDeliveryStatus.delivered,
      mediaUrl: mediaUrl,
      pinnedAt: pinned ? DateTime.utc(2026, 9, 15, 9, 56) : null,
      replyPreview: const ChatReplyPreviewProjection(
          id: 'reply', senderName: 'A', preview: 'quoted', deleted: false),
    );

class _Api extends ChatApiClient implements ChatModerationApi {
  _Api() : super(Dio());
  Object? deleteError;

  @override
  Future<void> deleteCircleMessage(
      {required String token,
      required String sessionId,
      required String circleId,
      required String messageId}) async {
    if (deleteError != null) throw deleteError!;
  }

  @override
  Future<void> deleteDirectMessage(
      {required String token,
      required String sessionId,
      required String userId,
      required String messageId}) async {}
}
