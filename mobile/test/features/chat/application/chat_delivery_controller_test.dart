import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
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
    // Editing a terminal draft rotates the idempotency key: the spec rejects
    // reusing one key for materially different content.
    await controller.editPending(key, 'edited');
    expect(controller.state.terminalFailures, isEmpty);
    final edited = (await store.loadAll()).single;
    expect(edited.content, 'edited');
    expect(edited.idempotencyKey, isNot(key));
    await controller.discardPending(edited.idempotencyKey);
    expect(await store.loadAll(), isEmpty);
  });

  test('a 429 with Retry-After replaces the next delay, capped at 30s',
      () async {
    final delays = <Duration>[];
    final api = _DeliveryApi(failuresBeforeSuccess: 2)
      ..rateLimited = true;
    final controller = GroupChatController(
      api,
      _credentials,
      realtime: _Realtime(),
      pendingStore: PendingMessageStore(_MemoryStorage()),
      retryDelay: (delay) async => delays.add(delay),
    );
    await controller.open('circle');
    expect(await controller.sendText('draft'), isFalse);
    await _until(() => api.keys.length == 3);

    // Schedule [1, 2, 4]: every failed attempt was a 429 carrying
    // Retry-After 60s, so each following wait honors the server but is
    // capped at 30 seconds (FR-008).
    expect(delays, const [
      Duration(seconds: 30),
      Duration(seconds: 30),
    ]);
    expect(api.keys.toSet(), hasLength(1));
  });

  test('restored pending messages keep the sender id after a restart',
      () async {
    final storage = _MemoryStorage();
    final first = GroupChatController(
      _DeliveryApi(failuresBeforeSuccess: 99),
      _credentials,
      realtime: _Realtime(),
      pendingStore: PendingMessageStore(storage),
      retryDelay: (delay) async {},
    );
    await first.open('circle');
    await first.sendText('offline draft');
    await _until(() => first.state.terminalFailures.isNotEmpty);

    // New controller, same durable storage: the restarted app restores the
    // queued draft as the user's OWN pending message (FR-008/US2-AC1).
    final restored = GroupChatController(
      _DeliveryApi(failuresBeforeSuccess: 99),
      _credentials,
      realtime: _Realtime(),
      pendingStore: PendingMessageStore(storage),
      retryDelay: (delay) async {},
    );
    await restored.open('circle');
    await _until(() => restored.state.messages.isNotEmpty);
    final pending =
        restored.state.messages.singleWhere((m) => m.content == 'offline draft');
    expect(pending.senderId, 'user');
    expect(pending.deliveryStatus, ChatDeliveryStatus.pending);
  });

  test('editing a terminal draft issues a fresh idempotency key', () async {
    final store = PendingMessageStore(_MemoryStorage());
    final api = _DeliveryApi(failuresBeforeSuccess: 99);
    final controller = GroupChatController(
      api,
      _credentials,
      realtime: _Realtime(),
      pendingStore: store,
      retryDelay: (delay) async {},
    );
    await controller.open('circle');
    await controller.sendText('original');
    await _until(() => controller.state.terminalFailures.isNotEmpty);
    final oldKey = controller.state.terminalFailures.keys.single;

    await controller.editPending(oldKey, 'materially different');

    final envelope = (await store.loadAll()).single;
    expect(envelope.idempotencyKey, isNot(oldKey));
    expect(envelope.content, 'materially different');
    expect(
        controller.state.messages.map((m) => m.id), [envelope.idempotencyKey]);
    expect(controller.state.terminalFailures, isEmpty);
  });

  test('an in-flight optimistic message shows the sent state', () async {
    final api = _DeliveryApi(failuresBeforeSuccess: 0);
    final sendGate = Completer<ChatMessage>();
    api.nextResult = sendGate.future;
    final controller = GroupChatController(
      api,
      _credentials,
      realtime: _Realtime(),
      pendingStore: PendingMessageStore(_MemoryStorage()),
    );
    await controller.open('circle');

    final sending = controller.sendText('in flight');
    await Future<void>.delayed(Duration.zero);
    final optimistic =
        controller.state.messages.singleWhere((m) => m.content == 'in flight');
    expect(optimistic.deliveryStatus, ChatDeliveryStatus.sent);

    sendGate.complete(ChatMessage(
      id: 'server',
      senderId: 'user',
      circleId: 'circle',
      content: 'in flight',
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026),
      deliveryStatus: ChatDeliveryStatus.delivered,
    ));
    expect(await sending, isTrue);
    expect(controller.state.messages.single.id, 'server');
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
  bool rateLimited = false;
  Future<ChatMessage>? nextResult;

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
    if (nextResult != null) return nextResult!;
    if (attempts++ < failuresBeforeSuccess) {
      if (rateLimited) {
        throw const ChatApiException(
            statusCode: 429,
            code: 'ERR_RATE_LIMITED',
            message: 'slow down',
            retryAfterSeconds: 60);
      }
      throw const ChatApiException(
          statusCode: null, code: 'ERR_REQUEST_FAILED', message: 'offline');
    }
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
