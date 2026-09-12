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

import '../../helpers/stub_auth_notifier.dart';

void main() {
  testWidgets('archived chat retains history and exposes no mutations in RTL',
      (tester) async {
    final api = _Api();
    final controller = GroupChatController(
      api,
      () async => (token: 'token', sessionId: 'session', userId: 'me'),
      realtime: _Realtime(),
    );

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier(
                initialState: AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'session',
                  user: BackendUser(
                    id: 'me',
                    firebaseUid: 'firebase',
                    preferredLanguage: 'ar',
                    createdAt: DateTime.utc(2026),
                  ),
                ),
              )),
          groupChatControllerProvider('circle').overrideWith((_) => controller),
        ],
        child: const MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: GroupChatScreen(
              circleId: 'circle',
              readOnly: true,
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('رسالة محفوظة'), findsOneWidget);
    expect(find.text('هذه المحادثة للقراءة فقط'), findsOneWidget);
    expect(find.byType(TextField), findsNothing);
    expect(await controller.sendText('ممنوع'), isFalse);
    expect(api.sent, isFalse);
  });
}

class _Api extends ChatApiClient {
  _Api() : super(Dio());
  bool sent = false;
  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async => ChatMessagePage(
        messages: [
          ChatMessage(
            id: 'm1',
            senderId: 'other',
            circleId: circleId,
            content: 'رسالة محفوظة',
            type: ChatMessageType.text,
            sentAt: DateTime.utc(2026),
            deliveryStatus: ChatDeliveryStatus.delivered,
          ),
        ],
        hasMore: false,
      );
  @override
  Future<ChatMessage> sendTextMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String idempotencyKey,
  }) async {
    sent = true;
    throw StateError('must not be called');
  }
}

class _Realtime implements ChatRealtimeClient {
  final _events = StreamController<ChatRealtimeEvent>.broadcast();
  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
          {required String token, required String backendSessionId}) =>
      _events.stream;
  @override
  Future<void> dispose() => _events.close();
}
