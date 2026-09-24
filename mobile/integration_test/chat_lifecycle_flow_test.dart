import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('chat lifecycle preserves access boundaries without live session',
      (tester) async {
    final api = _LifecycleApi();
    final realtime = _Realtime();
    final chat = GroupChatController(api,
        () async => (token: 'token', sessionId: 'session', userId: 'member'),
        realtime: realtime);
    await tester.pumpWidget(ProviderScope(
      overrides: [
        authControllerProvider.overrideWith((_) => _Auth()),
        groupChatControllerProvider('circle').overrideWith((_) => chat),
      ],
      child: const MaterialApp(
        home: Directionality(
          textDirection: TextDirection.rtl,
          child: GroupChatScreen(circleId: 'circle', readOnly: false),
        ),
      ),
    ));
    await _ready(tester, chat);
    expect(find.text('قبل الإزالة'), findsOneWidget);

    // Removal: access loss clears the current period and blocks writes.
    api.failReads = true;
    realtime.emit(const ChatUnknownEvent(
      eventId: 'removed',
      type: 'chat.membership_removed',
    ));
    await _until(tester, () => chat.state.status == GroupChatStatus.accessLost);
    expect(chat.state.messages, isEmpty);
    expect(await chat.sendText('ممنوع'), isFalse);

    // Rejoin: a fresh open creates a new period without old messages.
    api.failReads = false;
    api.next = _message('after-rejoin', 'بعد العودة');
    await chat.open('circle');
    await _ready(tester, chat);
    await tester.pump();
    expect(find.text('بعد العودة'), findsOneWidget);
    expect(find.text('قبل الإزالة'), findsNothing);

    // Archive: retained reads remain visible while all mutations disappear.
    chat.setReadOnly(true);
    await tester.pump();
    expect(find.text(ChatUiLabels.readOnlyArchived), findsOneWidget);
    expect(find.byType(TextField), findsNothing);
    expect(await chat.sendText('ممنوع بعد الأرشفة'), isFalse);
    expect(api.sent, isFalse);

    // Revoked backend session: the next authoritative read loses access.
    chat.setReadOnly(false);
    api.failReads = true;
    realtime.emit(const ChatUnknownEvent(
      eventId: 'revoked-session',
      type: 'chat.session_revoked',
    ));
    await _until(tester, () => chat.state.status == GroupChatStatus.accessLost);
    expect(chat.state.messages, isEmpty);
  });
}

Future<void> _ready(WidgetTester tester, GroupChatController chat) async {
  await _until(tester, () => chat.state.status == GroupChatStatus.ready);
}

Future<void> _until(WidgetTester tester, bool Function() condition) async {
  for (var i = 0; i < 100 && !condition(); i++) {
    await tester.pump(const Duration(milliseconds: 20));
  }
  expect(condition(), isTrue);
}

ChatMessage _message(String id, String content) => ChatMessage(
      id: id,
      senderId: 'member',
      circleId: 'circle',
      content: content,
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );

class _Auth extends StateNotifier<AuthState> implements AuthController {
  _Auth()
      : super(AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session',
            user: BackendUser(
                id: 'member',
                firebaseUid: 'fb',
                preferredLanguage: 'ar',
                createdAt: DateTime.utc(2026))));
  @override
  Future<void> register(
      {required String email,
      required String password,
      required String displayName,
      required String preferredLanguage}) async {}
  @override
  Future<void> signIn(
      {required String email, required String password}) async {}
  @override
  Future<void> logout() async {}
}

class _LifecycleApi extends ChatApiClient {
  _LifecycleApi() : super(Dio());
  bool failReads = false;
  bool sent = false;
  ChatMessage next = _message('before-removal', 'قبل الإزالة');
  @override
  Future<ChatMessagePage> listMessages(
      {required String token,
      required String sessionId,
      required String circleId,
      int? limit,
      String? before}) async {
    if (failReads) {
      throw const ChatApiException(
          statusCode: 403, code: 'forbidden', message: 'forbidden');
    }
    return ChatMessagePage(messages: [next], hasMore: false);
  }

  @override
  Future<ChatMessage> sendTextMessage(
      {required String token,
      required String sessionId,
      required String circleId,
      required String content,
      required String idempotencyKey}) async {
    sent = true;
    return next;
  }
}

class _Realtime implements ChatRealtimeClient {
  final _events = StreamController<ChatRealtimeEvent>.broadcast(sync: true);
  void emit(ChatRealtimeEvent event) => _events.add(event);
  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
          {required String token, required String backendSessionId}) =>
      _events.stream;
  @override
  Future<void> dispose() => _events.close();
}
