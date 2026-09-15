import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_moderation_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
      'T088: sender and teacher deletion converge to a redacted projection',
      (tester) async {
    final api = _AcceptanceApi();
    final controller = ChatModerationController(api,
        () async => (token: 'token', sessionId: 'session', userId: 'sender'));
    final senderMessage =
        _message('sender-message', 'sender', media: 'https://private/media');
    final teacherMessage = _message('teacher-target', 'student');
    controller.setMessages([senderMessage, teacherMessage]);

    expect(
        await controller.deleteCircleMessage('circle', senderMessage,
            now: DateTime.utc(2026, 9, 15, 10), isTeacher: false),
        isTrue);
    expect(
        await controller.deleteCircleMessage('circle', teacherMessage,
            now: DateTime.utc(2026, 9, 15, 10), isTeacher: true),
        isTrue);
    controller.handleDeletionEvent(ChatMessageDeletedEvent(
        eventId: 'race-1',
        messageId: 'sender-message',
        deletedAt: DateTime.utc(2026, 9, 15, 10)));

    expect(api.deletedIds, ['sender-message', 'teacher-target']);
    expect(
        controller.state.messages.every((message) => message.deletedAt != null),
        isTrue);
    expect(
        controller.state.messages.every((message) => message.content.isEmpty),
        isTrue);
    expect(
        controller.state.messages.every((message) => message.mediaUrl == null),
        isTrue);
  });

  testWidgets('T088: duplicate sender and teacher requests converge safely',
      (tester) async {
    final api = _AcceptanceApi();
    final controller = ChatModerationController(api,
        () async => (token: 'token', sessionId: 'session', userId: 'sender'));
    final message = _message('race-message', 'sender');
    controller.setMessages([message]);

    final results = await Future.wait([
      controller.deleteCircleMessage('circle', message,
          now: DateTime.utc(2026, 9, 15, 10), isTeacher: false),
      controller.deleteCircleMessage('circle', message,
          now: DateTime.utc(2026, 9, 15, 10), isTeacher: false),
    ]);

    expect(results, [true, true]);
    expect(api.deletedIds, ['race-message', 'race-message']);
    expect(controller.state.deletedMessageIds, {'race-message'});
    expect(controller.state.messages.single.content, isEmpty);
  });

  testWidgets(
      'T088: deletion event redacts audit-visible group and direct projections',
      (tester) async {
    final groupRealtime = _AcceptanceRealtimeClient();
    final group = GroupChatController(
      _AcceptanceApi()
        ..pages.add(ChatMessagePage(
            messages: [_message('group-audit', 'student')], hasMore: false)),
      () async => (token: 'token', sessionId: 'session', userId: 'teacher'),
      realtime: groupRealtime,
    );
    final directRealtime = _AcceptanceRealtimeClient();
    final direct = DirectChatController(
      _AcceptanceApi()
        ..pages.add(ChatMessagePage(
            messages: [_directMessage('direct-audit')], hasMore: false)),
      () async => (token: 'token', sessionId: 'session', userId: 'sender'),
      realtime: directRealtime,
    );
    addTearDown(() {
      group.dispose();
      direct.dispose();
    });

    await group.open('circle');
    await direct.open('peer');
    final groupEvent = _deletedEvent('group-audit');
    final directEvent = _deletedEvent('direct-audit');
    groupRealtime.emit(groupEvent);
    directRealtime.emit(directEvent);
    await Future<void>.delayed(Duration.zero);

    expect(group.state.messages.single.content, isEmpty);
    expect(group.state.messages.single.deletedAt, groupEvent.deletedAt);
    expect(direct.state.messages.single.content, isEmpty);
    expect(direct.state.messages.single.deletedAt, directEvent.deletedAt);
  });

  testWidgets('T088: media projection is revoked after delete acceptance',
      (tester) async {
    final api = _AcceptanceApi();
    final controller = ChatModerationController(api,
        () async => (token: 'token', sessionId: 'session', userId: 'sender'));
    final message = _message('media-delete', 'sender',
        media: 'https://minio.invalid/private/media');
    controller.setMessages([message]);

    expect(
        await controller.deleteCircleMessage('circle', message,
            now: DateTime.utc(2026, 9, 15, 10)),
        isTrue);
    final revoked = controller.state.messages.single;
    expect(revoked.mediaUrl, isNull);
    expect(revoked.content, isEmpty);
    // Real object deletion and retained-byte checks belong to the backend
    // PostgreSQL/MinIO integration gate, not this mobile seam test.
    expect(api.deletedIds, ['media-delete']);
  });
}

ChatMessageDeletedEvent _deletedEvent(String messageId) =>
    ChatMessageDeletedEvent(
      eventId: 'audit-$messageId',
      messageId: messageId,
      deletedAt: DateTime.utc(2026, 9, 15, 10),
    );

ChatMessage _directMessage(String id) => ChatMessage(
      id: id,
      senderId: 'sender',
      circleId: null,
      dmPeerId: 'peer',
      content: 'private direct',
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 15, 9, 55),
      deliveryStatus: ChatDeliveryStatus.delivered,
      mediaUrl: 'https://minio.invalid/private/direct',
    );

ChatMessage _message(String id, String senderId, {String? media}) =>
    ChatMessage(
      id: id,
      senderId: senderId,
      circleId: 'circle',
      content: 'private content',
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 15, 9, 55),
      deliveryStatus: ChatDeliveryStatus.delivered,
      mediaUrl: media,
      pinnedAt: DateTime.utc(2026, 9, 15, 9, 56),
      replyPreview: const ChatReplyPreviewProjection(
          id: 'reply',
          senderName: 'student',
          preview: 'quoted',
          deleted: false),
    );

class _AcceptanceApi extends ChatApiClient {
  _AcceptanceApi() : super(Dio());
  final pages = <ChatMessagePage>[];
  final deletedIds = <String>[];

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async =>
      pages.removeAt(0);

  @override
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async =>
      pages.removeAt(0);

  @override
  Future<void> deleteCircleMessage(
          {required String token,
          required String sessionId,
          required String circleId,
          required String messageId}) async =>
      deletedIds.add(messageId);
}

class _AcceptanceRealtimeClient
    implements ChatRealtimeClient, ChatRealtimePresenceClient {
  final _events = StreamController<ChatRealtimeEvent>.broadcast(sync: true);

  void emit(ChatRealtimeEvent event) => _events.add(event);

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
          {required String token, required String backendSessionId}) =>
      _events.stream;

  @override
  Stream<ChatRealtimeEvent> directChatEvents(String peerId,
          {required String token, required String backendSessionId}) =>
      _events.stream;

  @override
  Future<void> sendTyping(
      {String? circleId, String? dmPeerId, required bool isTyping}) async {}

  @override
  Future<void> dispose() => _events.close();
}
