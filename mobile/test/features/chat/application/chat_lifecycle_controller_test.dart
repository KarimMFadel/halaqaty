import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

const _circleId = '22222222-2222-2222-2222-222222222222';
const _memberId = '33333333-3333-3333-3333-333333333333';

void main() {
  group('GroupChatController lifecycle', () {
    test('reconnect: current access loss clears history immediately', () async {
      final api = _LifecycleChatApi()
        ..pages.add(_page([_message('old-period')]))
        ..nextListFailure = const ChatApiException(
          statusCode: 403,
          code: 'forbidden',
          message: 'forbidden',
        );
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);
      await controller.open(_circleId);

      realtime.emit(const ChatReconnectedEvent(eventId: 'reconnected'));
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.status, GroupChatStatus.accessLost);
      expect(controller.state.messages, isEmpty);
      expect(await controller.sendText('must not leave the device'), isFalse);
      expect(api.sentContents, isEmpty);

      realtime.emit(ChatMessageEvent(
        eventId: 'late-delivery',
        message: _message('must-stay-hidden'),
      ));
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.messages, isEmpty);
    });

    test('rejoin: reopening replaces history with the new membership period',
        () async {
      final api = _LifecycleChatApi()
        ..pages.add(_page([_message('first-period')]))
        ..pages.add(_page([_message('second-period')]));
      final controller = _controller(api, _LifecycleRealtimeClient());
      addTearDown(controller.dispose);

      await controller.open(_circleId);
      await controller.close();
      await controller.open(_circleId);

      expect(
        controller.state.messages.map((message) => message.id),
        ['second-period'],
      );
      expect(
        controller.state.messages
            .any((message) => message.id == 'first-period'),
        isFalse,
      );
    });

  });
}

GroupChatController _controller(
  _LifecycleChatApi api,
  _LifecycleRealtimeClient realtime,
) =>
    GroupChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: _memberId),
      realtime: realtime,
    );

ChatMessage _message(String id) => ChatMessage(
      id: id,
      senderId: _memberId,
      circleId: _circleId,
      content: id,
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 8, 12),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );

ChatMessagePage _page(List<ChatMessage> messages) => ChatMessagePage(
      messages: messages,
      hasMore: false,
    );

class _LifecycleChatApi extends ChatApiClient {
  _LifecycleChatApi() : super(Dio());

  final pages = <ChatMessagePage>[];
  final sentContents = <String>[];
  Object? nextListFailure;

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async {
    final failure = nextListFailure;
    if (failure != null) {
      nextListFailure = null;
      throw failure;
    }
    return pages.removeAt(0);
  }

  @override
  Future<ChatMessage> sendTextMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String idempotencyKey,
  }) async {
    sentContents.add(content);
    return _message('unexpected-send');
  }
}

class _LifecycleRealtimeClient implements ChatRealtimeClient {
  final _events = StreamController<ChatRealtimeEvent>.broadcast(sync: true);

  void emit(ChatRealtimeEvent event) => _events.add(event);

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(
    String circleId, {
    required String token,
    required String backendSessionId,
  }) =>
      _events.stream;

  @override
  Future<void> dispose() => _events.close();
}
