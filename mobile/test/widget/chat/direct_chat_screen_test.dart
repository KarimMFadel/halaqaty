import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:dio/dio.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
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

    expect(find.text('This conversation is unavailable'), findsOneWidget);
    expect(find.textContaining('private'), findsNothing);
  });
}

class _FakeReadyApi extends ChatApiClient {
  _FakeReadyApi() : super(Dio());

  @override
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async =>
      const ChatMessagePage(messages: [], hasMore: false);
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
