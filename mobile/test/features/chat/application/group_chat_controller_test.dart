import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

const _circleId = '22222222-2222-2222-2222-222222222222';
const _senderId = '33333333-3333-3333-3333-333333333333';

ChatMessage _message(
  String id, {
  DateTime? sentAt,
  ChatDeliveryStatus status = ChatDeliveryStatus.delivered,
  String content = 'نص',
  String senderId = _senderId,
}) =>
    ChatMessage(
      id: id,
      senderId: senderId,
      circleId: _circleId,
      content: content,
      type: ChatMessageType.text,
      sentAt: sentAt ?? DateTime.utc(2026, 9, 3, 12),
      deliveryStatus: status,
    );

ChatMessagePage _page(
  List<ChatMessage> messages, {
  bool hasMore = false,
  String? nextBefore,
}) =>
    ChatMessagePage(
        messages: messages, hasMore: hasMore, nextBefore: nextBefore);

void main() {
  test('initial history marks only eligible incoming messages as read',
      () async {
    final api = _FakeChatApi()
      ..pages.add(_page([
        _message('incoming', senderId: 'other'),
        _message('own', senderId: 'user'),
      ]));
    final presence = ChatPresenceController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    final controller = GroupChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
      realtime: _FakeChatRealtimeClient(),
      presence: presence,
    );
    addTearDown(() {
      controller.dispose();
      presence.dispose();
    });

    await controller.open('circle');

    expect(api.groupReadIds, ['incoming']);
  });
  test('open loads the authoritative newest-first page and subscribes',
      () async {
    final api = _FakeChatApi()
      ..pages.add(_page(
        [
          _message('m2', sentAt: DateTime.utc(2026, 9, 3, 12, 5)),
          _message('m1')
        ],
        hasMore: true,
        nextBefore: 'cursor-1',
      ));
    final realtime = _FakeChatRealtimeClient();
    final controller = _controller(api, realtime);
    addTearDown(controller.dispose);

    await controller.open(_circleId);

    expect(controller.state.status, GroupChatStatus.ready);
    expect(controller.state.messages.map((m) => m.id), ['m2', 'm1']);
    expect(controller.state.hasMore, isTrue);
    expect(controller.state.nextBefore, 'cursor-1');
    expect(realtime.circleChatEventsCalls, 1);
  });

  test('loadOlder pages with the before cursor and prepends older messages',
      () async {
    final api = _FakeChatApi()
      ..pages.add(_page(
        [
          _message('m2', sentAt: DateTime.utc(2026, 9, 3, 12, 5)),
          _message('m1')
        ],
        hasMore: true,
        nextBefore: 'cursor-1',
      ))
      ..pages
          .add(_page([_message('m0', sentAt: DateTime.utc(2026, 9, 3, 11))]));
    final controller = _controller(api, _FakeChatRealtimeClient());
    addTearDown(controller.dispose);
    await controller.open(_circleId);

    await controller.loadOlder();

    expect(
        api.listCalls.singleWhere((c) => c.before != null).before, 'cursor-1');
    expect(controller.state.messages.map((m) => m.id), ['m2', 'm1', 'm0']);
    expect(controller.state.hasMore, isFalse);

    await controller.loadOlder();
    expect(api.listCalls, hasLength(2));
  });

  test('loadOlder marks newly loaded eligible incoming messages as read',
      () async {
    final api = _FakeChatApi()
      ..pages.add(_page([_message('own', senderId: _senderId)],
          hasMore: true, nextBefore: 'cursor-1'))
      ..pages.add(_page([
        _message('incoming', senderId: 'other'),
        _message('older-own', senderId: _senderId),
      ]));
    final presence = ChatPresenceController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: _senderId),
    );
    final controller = GroupChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: _senderId),
      realtime: _FakeChatRealtimeClient(),
      presence: presence,
    );
    addTearDown(() {
      controller.dispose();
      presence.dispose();
    });

    await controller.open(_circleId);
    await controller.loadOlder();

    expect(api.groupReadIds, ['incoming']);
  });

  test('sendText is optimistic pending then replaces with the server message',
      () async {
    final api = _FakeChatApi()..pages.add(_page([_message('m1')]));
    final sendCompleter = Completer<ChatMessage>();
    api.nextSendResult = sendCompleter.future;
    final controller = _controller(api, _FakeChatRealtimeClient());
    addTearDown(controller.dispose);
    await controller.open(_circleId);

    final sending = controller.sendText('مرحبا');
    await Future<void>.delayed(Duration.zero);
    final inFlight =
        controller.state.messages.singleWhere((m) => m.content == 'مرحبا');
    // The request has left the local queue but acceptance is unconfirmed:
    // the local projection shows `sent` (US5-AC2), never a server state.
    expect(inFlight.deliveryStatus, ChatDeliveryStatus.sent);
    expect(inFlight.senderId, _senderId);

    final serverMessage = _message('server-1',
        status: ChatDeliveryStatus.delivered, content: 'مرحبا');
    sendCompleter.complete(serverMessage);
    expect(await sending, isTrue);

    expect(
        controller.state.messages
            .where((m) => m.content == 'مرحبا')
            .map((m) => m.id),
        ['server-1']);
    final replaced =
        controller.state.messages.singleWhere((m) => m.id == 'server-1');
    expect(replaced.deliveryStatus, ChatDeliveryStatus.delivered);
    expect(api.sentIdempotencyKeys, hasLength(1));
    expect(api.sentContents, ['مرحبا']);
  });

  test('duplicate realtime messages are deduplicated by message id', () async {
    final api = _FakeChatApi()..pages.add(_page([_message('m1')]));
    final realtime = _FakeChatRealtimeClient();
    final controller = _controller(api, realtime);
    addTearDown(controller.dispose);
    await controller.open(_circleId);

    final message = _message('m3', sentAt: DateTime.utc(2026, 9, 3, 12, 10));
    // Distinct event ids prove dedup happens at the message-id level.
    realtime.emit(ChatMessageEvent(eventId: 'event-1', message: message));
    realtime.emit(ChatMessageEvent(eventId: 'event-2', message: message));

    expect(controller.state.messages.where((m) => m.id == 'm3'), hasLength(1));
    expect(controller.state.messages.map((m) => m.id), ['m3', 'm1']);
  });

  test('unknown events reconcile against authoritative history', () async {
    final api = _FakeChatApi()
      ..pages.add(_page([_message('m1')]))
      ..pages.add(_page([
        _message('m2', sentAt: DateTime.utc(2026, 9, 3, 12, 5)),
        _message('m1'),
      ]));
    final realtime = _FakeChatRealtimeClient();
    final controller = _controller(api, realtime);
    addTearDown(controller.dispose);
    await controller.open(_circleId);

    realtime
        .emit(const ChatUnknownEvent(eventId: 'event-9', type: 'chat.typing'));
    await Future<void>.delayed(Duration.zero);

    expect(api.listCalls, hasLength(2));
    expect(api.listCalls.last.before, isNull);
    expect(controller.state.messages.map((m) => m.id), ['m2', 'm1']);
  });

  test('a realtime message arriving during the initial page load survives',
      () async {
    // The REST snapshot is in flight while a commit is delivered live: the
    // page predates the commit, so applying it must merge, not replace
    // (FR-011 REST/realtime arrival in either order).
    final listGate = Completer<ChatMessagePage>();
    final api = _FakeChatApi()..nextListResult = listGate.future;
    final realtime = _FakeChatRealtimeClient();
    final controller = _controller(api, realtime);
    addTearDown(controller.dispose);

    final opening = controller.open(_circleId);
    await Future<void>.delayed(Duration.zero);
    realtime.emit(ChatMessageEvent(
      eventId: 'event-live',
      message: _message('m-live', sentAt: DateTime.utc(2026, 9, 3, 12, 30)),
    ));
    listGate.complete(_page([_message('m1')]));
    await opening;

    expect(controller.state.messages.map((m) => m.id), ['m-live', 'm1']);
  });

  test('close stops realtime updates and resets the projection', () async {
    final api = _FakeChatApi()..pages.add(_page([_message('m1')]));
    final realtime = _FakeChatRealtimeClient();
    final controller = _controller(api, realtime);
    addTearDown(controller.dispose);
    await controller.open(_circleId);

    await controller.close();
    realtime.emit(ChatMessageEvent(
      eventId: 'event-late',
      message: _message('m3', sentAt: DateTime.utc(2026, 9, 3, 12, 10)),
    ));

    expect(controller.state.status, GroupChatStatus.idle);
    expect(controller.state.messages, isEmpty);
  });

  test('open failure surfaces an error state', () async {
    final api = _FakeChatApi()..listFailure = StateError('history failed');
    final controller = _controller(api, _FakeChatRealtimeClient());
    addTearDown(controller.dispose);

    await controller.open(_circleId);

    expect(controller.state.status, GroupChatStatus.error);
    expect(controller.state.errorMessage, contains('history failed'));
  });

  test('send failure surfaces an action error and drops the optimistic item',
      () async {
    final api = _FakeChatApi()
      ..pages.add(_page([_message('m1')]))
      ..sendFailure = StateError('send failed');
    final controller = _controller(api, _FakeChatRealtimeClient());
    addTearDown(controller.dispose);
    await controller.open(_circleId);

    // The draft survives rejection so the composer can retry: false means
    // the server did not accept the message.
    expect(await controller.sendText('مرحبا'), isFalse);

    expect(controller.state.status, GroupChatStatus.ready);
    expect(controller.state.actionErrorMessage, contains('send failed'));
    expect(controller.state.messages.map((m) => m.id), ['m1']);
  });

  test('sendText returns false on invalid drafts and failed credentials',
      () async {
    var credentialsFail = false;
    final api = _FakeChatApi()..pages.add(_page([_message('m1')]));
    final controller = GroupChatController(
      api,
      () async {
        if (credentialsFail) throw StateError('token expired');
        return (token: 'token', sessionId: 'session', userId: _senderId);
      },
      realtime: _FakeChatRealtimeClient(),
    );
    addTearDown(controller.dispose);
    await controller.open(_circleId);

    // Invalid (whitespace-only) draft: rejected before any REST call.
    expect(await controller.sendText('   '), isFalse);
    expect(api.sentContents, isEmpty);

    // Credential failure: rejected and surfaced as an action error.
    credentialsFail = true;
    expect(await controller.sendText('مرحبا'), isFalse);
    expect(api.sentContents, isEmpty);
    expect(controller.state.actionErrorMessage, contains('token expired'));
  });
}

GroupChatController _controller(
        _FakeChatApi api, _FakeChatRealtimeClient realtime) =>
    GroupChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: _senderId),
      realtime: realtime,
    );

class _ListCall {
  const _ListCall({this.before});
  final String? before;
}

class _FakeChatApi extends ChatApiClient {
  _FakeChatApi() : super(Dio());

  final pages = <ChatMessagePage>[];
  final listCalls = <_ListCall>[];
  Object? listFailure;
  Future<ChatMessagePage>? nextListResult;

  final sentIdempotencyKeys = <String>[];
  final sentContents = <String>[];
  Future<ChatMessage>? nextSendResult;
  Object? sendFailure;
  final groupReadIds = <String>[];

  @override
  Future<void> markMessageRead({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
    required String idempotencyKey,
    String? replyToId,
  }) async =>
      groupReadIds.add(messageId);

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async {
    listCalls.add(_ListCall(before: before));
    if (listFailure != null) throw listFailure!;
    if (nextListResult != null) return nextListResult!;
    if (pages.isEmpty) {
      throw StateError('No canned page for list call');
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
    sentIdempotencyKeys.add(idempotencyKey);
    sentContents.add(content);
    if (sendFailure != null) throw sendFailure!;
    return nextSendResult!;
  }
}

class _FakeChatRealtimeClient implements ChatRealtimeClient {
  final StreamController<ChatRealtimeEvent> _events =
      StreamController<ChatRealtimeEvent>.broadcast(sync: true);

  int circleChatEventsCalls = 0;

  void emit(ChatRealtimeEvent event) => _events.add(event);

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(
    String circleId, {
    required String token,
    required String backendSessionId,
  }) {
    circleChatEventsCalls++;
    return _events.stream;
  }

  @override
  Future<void> dispose() => _events.close();
}
