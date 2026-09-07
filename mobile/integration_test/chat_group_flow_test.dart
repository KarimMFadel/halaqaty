import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:integration_test/integration_test.dart';

const _circleId = 'circle-1';
const _meId = 'me-1';

/// T036 acceptance flow: with no live session anywhere, an active member
/// loads paginated group history, sends one valid text message, receives the
/// realtime projection exactly once (a duplicate delivery deduplicates), and
/// the whole flow runs on the chat REST + circle-topic boundary alone.
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
      'group chat flow: history, pagination, send, realtime, dedup — no live session',
      (tester) async {
    final backend = _ChatFlowBackend();
    final realtime = _ChatFlowRealtime();
    final chat = GroupChatController(
      backend,
      () async => (token: 'token', sessionId: 'backend-session', userId: _meId),
      realtime: realtime,
    );

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith(
            (_) => _IntegrationAuthNotifier(userId: _meId),
          ),
          groupChatControllerProvider(_circleId).overrideWith((_) => chat),
        ],
        child: const MaterialApp(
          home: GroupChatScreen(
            circleId: _circleId,
            circleName: 'حلقة الفجر',
          ),
        ),
      ),
    );

    // 1. Authoritative history renders with deterministic order: the
    //    newest-first page displays oldest-at-top, newest-at-bottom.
    await _pumpUntil(tester, () => chat.state.status == GroupChatStatus.ready);
    expect(find.text('الرسالة الأولى'), findsOneWidget);
    expect(find.text('الرسالة الثانية'), findsOneWidget);
    expect(
      tester.getCenter(find.text('الرسالة الأولى')).dy,
      lessThan(tester.getCenter(find.text('الرسالة الثانية')).dy),
    );
    expect(chat.state.hasMore, isTrue);

    // 2. The pagination trigger loads the older page through the cursor.
    await tester.tap(find.bySemanticsLabel('Load older messages'));
    await _pumpUntil(tester, () => !chat.state.hasMore);
    expect(
      backend.listCalls.singleWhere((call) => call.before != null).before,
      'cursor-1',
    );
    expect(find.text('الرسالة الأقدم'), findsOneWidget);
    expect(find.bySemanticsLabel('Load older messages'), findsNothing);

    // 3. One valid text send: optimistic pending, then the server projection
    //    replaces it — one durable message, one visible item.
    await tester.enterText(
      _editableWithin('Write a message'),
      'مرحبا',
    );
    // Live frames are async: pump once so the composer rebuild enables the
    // send button before the tap (same convention as queue_flow_test).
    await tester.pump();
    await tester.tap(find.bySemanticsLabel('Send'));
    await _pumpUntil(
        tester, () => chat.state.messages.any((m) => m.id == 'server-1'));
    await tester.pumpAndSettle();

    expect(backend.sentContents, ['مرحبا']);
    expect(backend.sentIdempotencyKeys, hasLength(1));
    // Exactly one "مرحبا": the bubble. The composer was cleared.
    expect(find.text('مرحبا'), findsOneWidget);
    expect(find.bySemanticsLabel('Delivered'), findsOneWidget);

    // 4. The realtime event is received once: an at-least-once duplicate
    //    delivery (two distinct event ids, one message id) projects exactly
    //    one visible message.
    final live = _message(
      'm3',
      content: 'الرسالة الحية',
      sentAt: DateTime.utc(2026, 9, 3, 12, 10),
    );
    realtime.emit(ChatMessageEvent(eventId: 'event-1', message: live));
    realtime.emit(ChatMessageEvent(eventId: 'event-2', message: live));
    await _pumpUntil(
        tester, () => chat.state.messages.any((m) => m.id == 'm3'));
    await tester.pumpAndSettle();

    expect(find.text('الرسالة الحية'), findsOneWidget);
    expect(chat.state.messages.where((m) => m.id == 'm3'), hasLength(1));

    // 5. No live-session dependency: the entire flow ran over the chat
    //    boundary alone — one circle-topic subscription, no session-room or
    //    media transport was ever consulted.
    expect(realtime.circleChatEventsCalls, 1);
  });
}

Future<void> _pumpUntil(
  WidgetTester tester,
  bool Function() condition, {
  Duration timeout = const Duration(seconds: 5),
}) async {
  final deadline = DateTime.now().add(timeout);
  while (!condition() && DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await tester.pump();
  expect(condition(), isTrue, reason: 'Timed out waiting for the chat flow');
}

/// Locates the editable text of a field labeled via its Semantics wrapper.
Finder _editableWithin(String label) => find.descendant(
      of: find.bySemanticsLabel(label),
      matching: find.byType(EditableText),
    );

ChatMessage _message(
  String id, {
  String content = 'نص',
  String senderId = 'other-1',
  String? senderName = 'مريم',
  DateTime? sentAt,
  ChatDeliveryStatus status = ChatDeliveryStatus.delivered,
}) =>
    ChatMessage(
      id: id,
      senderId: senderId,
      circleId: _circleId,
      content: content,
      type: ChatMessageType.text,
      sentAt: sentAt ?? DateTime.utc(2026, 9, 3, 12),
      deliveryStatus: status,
      senderName: senderName,
    );

ChatMessagePage _page(
  List<ChatMessage> messages, {
  bool hasMore = false,
  String? nextBefore,
}) =>
    ChatMessagePage(
        messages: messages, hasMore: hasMore, nextBefore: nextBefore);

class _IntegrationAuthNotifier extends StateNotifier<AuthState>
    implements AuthController {
  _IntegrationAuthNotifier({required String userId})
      : super(AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'backend-session',
          user: BackendUser(
            id: userId,
            firebaseUid: 'fb-$userId',
            preferredLanguage: 'ar',
            createdAt: DateTime.utc(2026, 1, 1),
          ),
        ));

  @override
  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async {}

  @override
  Future<void> signIn({
    required String email,
    required String password,
  }) async {}

  @override
  Future<void> logout() async {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }
}

class _ListCall {
  const _ListCall({this.before});
  final String? before;
}

/// Fake REST boundary with canned pages and captured sends.
class _ChatFlowBackend extends ChatApiClient {
  _ChatFlowBackend() : super(Dio()) {
    pages.addAll([
      _page(
        [
          _message('m2',
              content: 'الرسالة الثانية',
              sentAt: DateTime.utc(2026, 9, 3, 12, 5)),
          _message('m1', content: 'الرسالة الأولى'),
        ],
        hasMore: true,
        nextBefore: 'cursor-1',
      ),
      _page([
        _message('m0',
            content: 'الرسالة الأقدم', sentAt: DateTime.utc(2026, 9, 3, 11)),
      ]),
    ]);
  }

  final pages = <ChatMessagePage>[];
  final listCalls = <_ListCall>[];
  final sentIdempotencyKeys = <String>[];
  final sentContents = <String>[];

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async {
    listCalls.add(_ListCall(before: before));
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
  }) async {
    sentIdempotencyKeys.add(idempotencyKey);
    sentContents.add(content);
    return _message(
      'server-1',
      content: content,
      senderId: _meId,
      senderName: null,
      sentAt: DateTime.utc(2026, 9, 3, 12, 15),
    );
  }
}

/// Fake realtime boundary: one circle-topic subscription over a broadcast
/// stream; no session-room transport exists on this seam at all.
class _ChatFlowRealtime implements ChatRealtimeClient {
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
