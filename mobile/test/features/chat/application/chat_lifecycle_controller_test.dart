import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/pending_message_store.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

const _circleId = '22222222-2222-2222-2222-222222222222';
const _memberId = '33333333-3333-3333-3333-333333333333';

void main() {
  group('GroupChatController lifecycle', () {
    test('initial access denial clears history and cancels realtime', () async {
      final api = _LifecycleChatApi()
        ..nextListFailure = const ChatApiException(
          statusCode: 401,
          code: 'unauthorized',
          message: 'unauthorized',
        );
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);

      await controller.open(_circleId);

      expect(controller.state.status, GroupChatStatus.accessLost);
      expect(controller.state.messages, isEmpty);
      expect(realtime.hasListener, isFalse);
      realtime.emit(ChatMessageEvent(
        eventId: 'late-delivery',
        message: _message('must-stay-hidden'),
      ));
      await Future<void>.delayed(Duration.zero);
      expect(controller.state.messages, isEmpty);
    });

    test('initial access denial does not restore a durable pending draft',
        () async {
      final store = PendingMessageStore(_MemoryStorage());
      await store.save(const PendingMessageEnvelope(
        idempotencyKey: 'retained-draft',
        circleId: _circleId,
        content: 'must stay hidden',
      ));
      final api = _LifecycleChatApi()
        ..nextListFailure = const ChatApiException(
          statusCode: 401,
          code: 'unauthorized',
          message: 'unauthorized',
        );
      final controller = GroupChatController(
        api,
        () async => (token: 'token', sessionId: 'session', userId: _memberId),
        realtime: _LifecycleRealtimeClient(),
        pendingStore: store,
        retryDelay: (_) async {},
      );
      addTearDown(controller.dispose);

      await controller.open(_circleId);

      expect(controller.state.status, GroupChatStatus.accessLost);
      expect(controller.state.messages, isEmpty);
      expect(api.sentContents, isEmpty);
    });

    test('pagination access denial clears history and cancels realtime',
        () async {
      final api = _LifecycleChatApi()
        ..pages
            .add(_page([_message('visible')], hasMore: true, nextBefore: 'p2'))
        ..nextListFailure = const ChatApiException(
          statusCode: 404,
          code: 'not_found',
          message: 'not found',
        );
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);

      await controller.open(_circleId);
      await controller.loadOlder();

      expect(controller.state.status, GroupChatStatus.accessLost);
      expect(controller.state.messages, isEmpty);
      expect(realtime.hasListener, isFalse);
    });

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

    test('active write denial clears history and stops realtime delivery',
        () async {
      final api = _LifecycleChatApi()
        ..pages.add(_page([_message('visible-before-removal')]))
        ..nextSendFailure = const ChatApiException(
          statusCode: 403,
          code: 'forbidden',
          message: 'forbidden',
        );
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);

      await controller.open(_circleId);

      expect(await controller.sendText('must be rejected'), isFalse);
      expect(controller.state.status, GroupChatStatus.accessLost);
      expect(controller.state.messages, isEmpty);

      realtime.emit(ChatMessageEvent(
        eventId: 'late-delivery',
        message: _message('must-stay-hidden'),
      ));
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.messages, isEmpty);
    });

    test('stale write denial cannot revoke a rejoined chat', () async {
      final api = _LifecycleChatApi()
        ..pages.add(_page([_message('first-period')]))
        ..pages.add(_page([_message('second-period')]))
        ..pendingSendFailure = Completer<Object>();
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);

      await controller.open(_circleId);
      final send = controller.sendText('old-period-send');
      await api.sendStarted.future;
      await controller.close();
      await controller.open(_circleId);
      api.pendingSendFailure!.complete(const ChatApiException(
        statusCode: 403,
        code: 'forbidden',
        message: 'forbidden',
      ));

      expect(await send, isFalse);
      expect(controller.state.status, GroupChatStatus.ready);
      expect(
        controller.state.messages.map((message) => message.id),
        ['second-period'],
      );
      expect(realtime.hasListener, isTrue);
    });

    test('write denial still revokes access after concurrent reconciliation',
        () async {
      final api = _LifecycleChatApi()
        ..pages.add(_page([_message('visible-before-removal')]))
        ..pages.add(_page([_message('reconciled')]))
        ..pendingSendFailure = Completer<Object>();
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);
      await controller.open(_circleId);

      final send = controller.sendText('removed-during-send');
      await api.sendStarted.future;
      realtime.emit(const ChatUnknownEvent(
        eventId: 'unknown-during-send',
        type: 'chat.message_read',
      ));
      await Future<void>.delayed(Duration.zero);
      api.pendingSendFailure!.complete(const ChatApiException(
        statusCode: 403,
        code: 'forbidden',
        message: 'forbidden',
      ));

      expect(await send, isFalse);
      expect(controller.state.status, GroupChatStatus.accessLost);
      expect(controller.state.messages, isEmpty);
    });

    test(
        'pagination denial still revokes access after concurrent reconciliation',
        () async {
      final api = _LifecycleChatApi()
        ..pages
            .add(_page([_message('visible')], hasMore: true, nextBefore: 'p2'))
        ..pages.add(_page([_message('reconciled')]))
        ..pendingListFailure = Completer<Object>();
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);
      await controller.open(_circleId);

      final older = controller.loadOlder();
      await api.listStarted.future;
      realtime.emit(const ChatUnknownEvent(
        eventId: 'unknown-during-page',
        type: 'chat.message_read',
      ));
      await Future<void>.delayed(Duration.zero);
      api.pendingListFailure!.complete(const ChatApiException(
        statusCode: 404,
        code: 'not_found',
        message: 'not found',
      ));

      await older;
      expect(controller.state.status, GroupChatStatus.accessLost);
      expect(controller.state.messages, isEmpty);
    });

    test('rejoin after access loss loads only the new membership period',
        () async {
      final api = _LifecycleChatApi()
        ..pages.add(_page([_message('first-period')]))
        ..pages.add(_page([_message('second-period')]));
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);

      await controller.open(_circleId);
      api.nextListFailure = const ChatApiException(
        statusCode: 403,
        code: 'forbidden',
        message: 'forbidden',
      );
      realtime.emit(const ChatReconnectedEvent(eventId: 'removed'));
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.status, GroupChatStatus.accessLost);

      await controller.open(_circleId);

      expect(controller.state.status, GroupChatStatus.ready);
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

    test('archived chat retains reads while denying mutations', () async {
      final api = _LifecycleChatApi()
        ..pages.add(_page(
          [
            _message(
              'archived-newer',
              sentAt: DateTime.utc(2026, 9, 8, 12, 1),
            ),
          ],
          hasMore: true,
          nextBefore: 'older-page',
        ))
        ..pages.add(_page([_message('archived-older')]));
      final controller = _controller(api, _LifecycleRealtimeClient());
      addTearDown(controller.dispose);

      await controller.open(_circleId);
      controller.setReadOnly(true);
      await controller.loadOlder();

      expect(
        controller.state.messages.map((message) => message.id),
        ['archived-newer', 'archived-older'],
      );
      expect(await controller.sendText('blocked'), isFalse);
      expect(api.sentContents, isEmpty);
    });

    test('archived reconnect keeps the chat read-only', () async {
      final api = _LifecycleChatApi()..pages.add(_page([_message('archived')]));
      final realtime = _LifecycleRealtimeClient();
      final controller = _controller(api, realtime);
      addTearDown(controller.dispose);

      await controller.open(_circleId);
      controller.setReadOnly(true);
      realtime.emit(const ChatReconnectedEvent(eventId: 'reconnected'));
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.readOnly, isTrue);
      expect(await controller.sendText('blocked after reconnect'), isFalse);
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

ChatMessage _message(String id, {DateTime? sentAt}) => ChatMessage(
      id: id,
      senderId: _memberId,
      circleId: _circleId,
      content: id,
      type: ChatMessageType.text,
      sentAt: sentAt ?? DateTime.utc(2026, 9, 8, 12),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );

ChatMessagePage _page(
  List<ChatMessage> messages, {
  bool hasMore = false,
  String? nextBefore,
}) =>
    ChatMessagePage(
      messages: messages,
      hasMore: hasMore,
      nextBefore: nextBefore,
    );

class _LifecycleChatApi extends ChatApiClient {
  _LifecycleChatApi() : super(Dio());

  final pages = <ChatMessagePage>[];
  final sentContents = <String>[];
  Object? nextListFailure;
  Object? nextSendFailure;
  Completer<Object>? pendingSendFailure;
  Completer<Object>? pendingListFailure;
  final sendStarted = Completer<void>();
  final listStarted = Completer<void>();

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async {
    final pendingFailure = before == null ? null : pendingListFailure;
    if (pendingFailure != null) {
      listStarted.complete();
      throw await pendingFailure.future;
    }
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
    String? replyToId,
  }) async {
    sentContents.add(content);
    sendStarted.complete();
    final pendingFailure = pendingSendFailure;
    if (pendingFailure != null) {
      throw await pendingFailure.future;
    }
    final failure = nextSendFailure;
    if (failure != null) {
      nextSendFailure = null;
      throw failure;
    }
    return _message('unexpected-send');
  }
}

class _LifecycleRealtimeClient implements ChatRealtimeClient {
  final _events = StreamController<ChatRealtimeEvent>.broadcast(sync: true);

  void emit(ChatRealtimeEvent event) => _events.add(event);

  bool get hasListener => _events.hasListener;

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
