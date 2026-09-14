import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/data/pending_message_store.dart';
import 'package:integration_test/integration_test.dart';

const _circleId = 'circle-1';
const _userId = 'user-1';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
      'offline send survives restart and converges after REST and WebSocket redelivery',
      (tester) async {
    final storage = _MemoryStorage();
    final store = PendingMessageStore(storage);
    final sentKeys = <String>[];

    final offlineRealtime = _RecoveryRealtime();
    final offline = _controller(
      _RecoveryApi(sentKeys: sentKeys, offline: true),
      offlineRealtime,
      store,
    );
    await offline.open(_circleId);
    expect(await offline.sendText('رسالة مؤجلة'), isFalse);
    final pending = (await store.loadAll()).single;
    expect(
        offline.state.messages.single.deliveryStatus, ChatDeliveryStatus.sent);
    await offline.close();
    offline.dispose();
    await offlineRealtime.dispose();

    final recoveredApi = _RecoveryApi(sentKeys: sentKeys);
    final recoveredRealtime = _RecoveryRealtime();
    final recovered = _controller(recoveredApi, recoveredRealtime, store);
    addTearDown(recovered.dispose);
    addTearDown(recoveredRealtime.dispose);
    await recovered.open(_circleId);

    expect(recovered.state.messages.single.id, pending.idempotencyKey);
    await _until(() async => (await store.loadAll()).isEmpty);
    expect(sentKeys, [pending.idempotencyKey, pending.idempotencyKey]);
    expect(await store.loadAll(), isEmpty);
    expect(recovered.state.messages.single.id, 'server-1');

    final authoritative = _message('server-1');
    recoveredRealtime
        .emit(ChatMessageEvent(eventId: 'delivery-1', message: authoritative));
    recoveredRealtime
        .emit(ChatMessageEvent(eventId: 'delivery-2', message: authoritative));
    recoveredRealtime.emit(const ChatReconnectedEvent(eventId: 'reconnect-1'));
    await tester.pump();
    await tester.pump();

    expect(recovered.state.messages.where((m) => m.id == 'server-1'),
        hasLength(1));
    expect(recoveredApi.listCalls, greaterThanOrEqualTo(2));
  });
}

Future<void> _until(Future<bool> Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 10));
  while (!await condition()) {
    if (DateTime.now().isAfter(deadline)) {
      fail('Timed out waiting for automatic offline recovery');
    }
    await Future<void>.delayed(const Duration(milliseconds: 20));
  }
}

GroupChatController _controller(ChatApiClient api, ChatRealtimeClient realtime,
        PendingMessageStore store) =>
    GroupChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: _userId),
      realtime: realtime,
      pendingStore: store,
    );

ChatMessage _message(String id) => ChatMessage(
      id: id,
      senderId: _userId,
      circleId: _circleId,
      content: 'رسالة مؤجلة',
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 8),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );

class _RecoveryApi extends ChatApiClient {
  _RecoveryApi({required this.sentKeys, this.offline = false}) : super(Dio());

  final List<String> sentKeys;
  final bool offline;
  int listCalls = 0;

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async {
    listCalls++;
    return const ChatMessagePage(messages: [], hasMore: false);
  }

  @override
  Future<ChatMessage> sendTextMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String idempotencyKey,
  }) async {
    sentKeys.add(idempotencyKey);
    if (offline) throw const _OfflineException();
    return _message('server-1');
  }
}

class _OfflineException implements Exception {
  const _OfflineException();
}

class _RecoveryRealtime implements ChatRealtimeClient {
  final _events = StreamController<ChatRealtimeEvent>.broadcast(sync: true);

  void emit(ChatRealtimeEvent event) => _events.add(event);

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
          {required String token, required String backendSessionId}) =>
      _events.stream;

  @override
  Future<void> dispose() => _events.close();
}

class _MemoryStorage extends FlutterSecureStorage {
  final values = <String, String>{};

  @override
  Future<String?> read({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    LinuxOptions? lOptions,
    WindowsOptions? wOptions,
  }) async =>
      values[key];

  @override
  Future<void> write({
    required String key,
    required String? value,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    LinuxOptions? lOptions,
    WindowsOptions? wOptions,
  }) async {
    if (value == null) {
      values.remove(key);
    } else {
      values[key] = value;
    }
  }

  @override
  Future<void> delete({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    LinuxOptions? lOptions,
    WindowsOptions? wOptions,
  }) async {
    values.remove(key);
  }
}
