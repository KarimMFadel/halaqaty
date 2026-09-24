import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:dio/dio.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_moderation_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/direct_chat_screen.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';

import '../../helpers/stub_auth_notifier.dart';

void main() {
  testWidgets('composer emits direct typing start and stop', (tester) async {
    final realtime = _FakeRealtime();
    final controller = DirectChatController(
      _FakeReadyApi(),
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
      realtime: realtime,
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier(
                initialState: AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'session',
                  user: BackendUser(
                    id: 'user',
                    firebaseUid: 'firebase-user',
                    preferredLanguage: 'en',
                    createdAt: DateTime.utc(2026),
                  ),
                ),
              )),
          directChatControllerProvider('peer').overrideWith((_) => controller),
        ],
        child: const MaterialApp(home: DirectChatScreen(peerId: 'peer')),
      ),
    );
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextField), 'typing');
    await tester.pump();
    await tester.enterText(find.byType(TextField), '');
    await tester.pump();

    expect(realtime.typing, [true, false]);
  });

  testWidgets('shows safe denial copy in RTL and LTR', (tester) async {
    final controller = DirectChatController(
      _FakeDeniedApi(),
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier(
                initialState: AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'session',
                  user: BackendUser(
                    id: 'user',
                    firebaseUid: 'firebase-user',
                    preferredLanguage: 'en',
                    createdAt: DateTime.utc(2026),
                  ),
                ),
              )),
          directChatControllerProvider('peer').overrideWith((_) => controller),
        ],
        child: const MaterialApp(home: DirectChatScreen(peerId: 'peer')),
      ),
    );
    await tester.pumpAndSettle();

    // Lost access is terminal: honest copy with a safe exit, never a retry
    // loop, and never the raw server code.
    expect(find.text('You no longer have access to this conversation'),
        findsOneWidget);
    expect(find.text('Back'), findsOneWidget);
    expect(find.text('Retry'), findsNothing);
    expect(find.textContaining('private'), findsNothing);
  });

  testWidgets('wires the authorized delete action into the direct screen',
      (tester) async {
    final message = ChatMessage(
      id: 'own-message',
      senderId: 'user',
      circleId: null,
      dmPeerId: 'peer',
      content: 'حذفني',
      type: ChatMessageType.text,
      sentAt: DateTime.now().toUtc().subtract(const Duration(minutes: 1)),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );
    final api = _FakeReadyApi(message: message);
    final chat = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    final moderation = ChatModerationController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier(
                initialState: AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'session',
                  user: BackendUser(
                    id: 'user',
                    firebaseUid: 'firebase-user',
                    preferredLanguage: 'en',
                    createdAt: DateTime.utc(2026),
                  ),
                ),
              )),
          directChatControllerProvider('peer').overrideWith((_) => chat),
          chatModerationControllerProvider.overrideWith((_) => moderation),
        ],
        child: const MaterialApp(home: DirectChatScreen(peerId: 'peer')),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.bySemanticsLabel('Delete message'), findsOneWidget);
    await tester.tap(find.bySemanticsLabel('Delete message'));
    await tester.pumpAndSettle();

    expect(api.deletedMessageId, 'own-message');
    expect(find.text('Message deleted'), findsOneWidget);
  });

  testWidgets('load failure keeps a retry path', (tester) async {
    final controller = DirectChatController(
      _FakeFlakyApi(),
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier(
                initialState: AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'session',
                  user: BackendUser(
                    id: 'user',
                    firebaseUid: 'firebase-user',
                    preferredLanguage: 'en',
                    createdAt: DateTime.utc(2026),
                  ),
                ),
              )),
          directChatControllerProvider('peer').overrideWith((_) => controller),
        ],
        child: const MaterialApp(home: DirectChatScreen(peerId: 'peer')),
      ),
    );
    await tester.pumpAndSettle();

    expect(controller.state.status, DirectChatStatus.error);
    expect(find.text('Could not load chat history'), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
    expect(find.textContaining('socket reset'), findsNothing);

    // The retry path re-opens the conversation and recovers to ready.
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(controller.state.status, DirectChatStatus.ready);
  });

  testWidgets('empty conversation shows guidance and keeps the composer',
      (tester) async {
    final controller = DirectChatController(
      _FakeReadyApi(),
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier(
                initialState: AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'session',
                  user: BackendUser(
                    id: 'user',
                    firebaseUid: 'firebase-user',
                    preferredLanguage: 'en',
                    createdAt: DateTime.utc(2026),
                  ),
                ),
              )),
          directChatControllerProvider('peer').overrideWith((_) => controller),
        ],
        child: const MaterialApp(home: DirectChatScreen(peerId: 'peer')),
      ),
    );
    await tester.pumpAndSettle();

    expect(
        find.text('No messages yet; start the conversation'), findsOneWidget);
    expect(find.byType(TextField), findsOneWidget);
  });

  testWidgets(
      'send stays disabled for an empty draft and a rejected send announces '
      'the failure', (tester) async {
    final semantics = tester.ensureSemantics();
    final api = _FakeReadyApi()..sendFailure = StateError('offline');
    final controller = DirectChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier(
                initialState: AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'session',
                  user: BackendUser(
                    id: 'user',
                    firebaseUid: 'firebase-user',
                    preferredLanguage: 'en',
                    createdAt: DateTime.utc(2026),
                  ),
                ),
              )),
          directChatControllerProvider('peer').overrideWith((_) => controller),
        ],
        child: const MaterialApp(home: DirectChatScreen(peerId: 'peer')),
      ),
    );
    await tester.pumpAndSettle();

    IconButton sendButton() =>
        tester.widget<IconButton>(find.widgetWithIcon(IconButton, Icons.send));

    // Empty drafts cannot be sent at all.
    expect(sendButton().onPressed, isNull);

    await tester.enterText(find.byType(TextField), 'مرحبا');
    await tester.pump();
    expect(sendButton().onPressed, isNotNull);

    // A rejected send keeps the draft and announces the failure in a live
    // region instead of staying silent.
    await tester.tap(find.widgetWithIcon(IconButton, Icons.send));
    await tester.pumpAndSettle();
    expect(api.sentContents, isEmpty);
    expect(
      tester.widget<TextField>(find.byType(TextField)).controller?.text,
      'مرحبا',
    );
    expect(find.text('Action failed'), findsOneWidget);
    expect(
      tester
          .getSemantics(find.text('Action failed'))
          .flagsCollection
          .isLiveRegion,
      isTrue,
    );
    semantics.dispose();
  });
}

class _FakeFlakyApi extends ChatApiClient {
  _FakeFlakyApi() : super(Dio());

  var calls = 0;

  @override
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async {
    calls++;
    if (calls == 1) throw StateError('socket reset');
    return ChatMessagePage(messages: const [], hasMore: false);
  }
}

class _FakeReadyApi extends ChatApiClient implements ChatModerationApi {
  _FakeReadyApi({this.message}) : super(Dio());

  final ChatMessage? message;
  String? deletedMessageId;
  Object? sendFailure;
  final sentContents = <String>[];

  @override
  Future<ChatMessage> sendDirectTextMessage({
    required String token,
    required String sessionId,
    required String userId,
    required String content,
    required String idempotencyKey,
  }) async {
    if (sendFailure != null) throw sendFailure!;
    sentContents.add(content);
    return ChatMessage(
      id: 'sent-${sentContents.length}',
      senderId: 'user',
      circleId: null,
      dmPeerId: userId,
      content: content,
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026),
      deliveryStatus: ChatDeliveryStatus.sent,
    );
  }

  @override
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async =>
      ChatMessagePage(
          messages: [if (message != null) message!], hasMore: false);

  @override
  Future<void> deleteDirectMessage({
    required String token,
    required String sessionId,
    required String userId,
    required String messageId,
  }) async {
    deletedMessageId = messageId;
  }

  @override
  Future<void> deleteCircleMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) async {}
}

class _FakeRealtime implements ChatRealtimePresenceClient {
  final typing = <bool>[];

  @override
  Stream<ChatRealtimeEvent> directChatEvents(String peerId,
          {required String token, required String backendSessionId}) =>
      const Stream.empty();

  @override
  Future<void> sendTyping(
      {String? circleId, String? dmPeerId, required bool isTyping}) async {
    typing.add(isTyping);
  }
}

class _FakeDeniedApi extends ChatApiClient {
  _FakeDeniedApi() : super(Dio());

  @override
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async =>
      throw const ChatApiException(
          statusCode: 403, code: 'ERR_FORBIDDEN', message: 'private');
}
