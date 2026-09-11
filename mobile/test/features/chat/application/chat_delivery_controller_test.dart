import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_delivery_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/data/pending_message_store.dart';

void main() {
  test('retry schedule is exactly 1, 2, then 4 seconds', () {
    expect(chatRetryDelays, const [
      Duration(seconds: 1),
      Duration(seconds: 2),
      Duration(seconds: 4),
    ]);
  });

  test('failed send retries 1/2/4 then exposes editable terminal state',
      () async {
    final delays = <Duration>[];
    final store = PendingMessageStore(_MemoryStorage());
    final api = _DeliveryApi(failuresBeforeSuccess: 99);
    final controller = GroupChatController(
      api,
      _credentials,
      realtime: _Realtime(),
      pendingStore: store,
      retryDelay: (delay) async => delays.add(delay),
    );
    await controller.open('circle');
    expect(await controller.sendText('draft'), isFalse);
    await _until(() => controller.state.terminalFailures.isNotEmpty);

    final key = controller.state.terminalFailures.keys.single;
    expect(delays, chatRetryDelays);
    expect(api.keys, everyElement(key));
    await controller.editPending(key, 'edited');
    expect(controller.state.terminalFailures, isEmpty);
    expect((await store.loadAll()).single.content, 'edited');
    await controller.discardPending(key);
    expect(await store.loadAll(), isEmpty);
  });
}

Future<({String token, String sessionId, String userId})>
    _credentials() async =>
        (token: 'token', sessionId: 'session', userId: 'user');

Future<void> _until(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 2));
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) fail('Timed out');
    await Future<void>.delayed(Duration.zero);
  }
}

class _DeliveryApi extends ChatApiClient {
  _DeliveryApi({required this.failuresBeforeSuccess}) : super(Dio());
  final int failuresBeforeSuccess;
  final keys = <String>[];
  int attempts = 0;

  @override
  Future<ChatMessagePage> listMessages(
          {required String token,
          required String sessionId,
          required String circleId,
          int? limit,
          String? before}) async =>
      const ChatMessagePage(messages: [], hasMore: false);

  @override
  Future<ChatMessage> sendTextMessage(
      {required String token,
      required String sessionId,
      required String circleId,
      required String content,
      required String idempotencyKey}) async {
    keys.add(idempotencyKey);
    if (attempts++ < failuresBeforeSuccess) throw StateError('offline');
    return ChatMessage(
      id: 'server',
      senderId: 'user',
      circleId: circleId,
      content: content,
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );
  }
}

class _Realtime implements ChatRealtimeClient {
  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
          {required String token, required String backendSessionId}) =>
      const Stream.empty();
  @override
  Future<void> dispose() async {}
}

class _MemoryStorage extends FlutterSecureStorage {
  final values = <String, String>{};
  @override
  Future<String?> read(
          {required String key,
          IOSOptions? iOptions,
          AndroidOptions? aOptions,
          WebOptions? webOptions,
          MacOsOptions? mOptions,
          LinuxOptions? lOptions,
          WindowsOptions? wOptions}) async =>
      values[key];
  @override
  Future<void> write(
      {required String key,
      required String? value,
      IOSOptions? iOptions,
      AndroidOptions? aOptions,
      WebOptions? webOptions,
      MacOsOptions? mOptions,
      LinuxOptions? lOptions,
      WindowsOptions? wOptions}) async {
    if (value == null) {
      values.remove(key);
    } else {
      values[key] = value;
    }
  }

  @override
  Future<void> delete(
      {required String key,
      IOSOptions? iOptions,
      AndroidOptions? aOptions,
      WebOptions? webOptions,
      MacOsOptions? mOptions,
      LinuxOptions? lOptions,
      WindowsOptions? wOptions}) async {
    values.remove(key);
  }
}
