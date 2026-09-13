import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:dio/dio.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/direct_chat_screen.dart';

void main() {
  testWidgets('shows safe denial copy in RTL and LTR', (tester) async {
    final controller = DirectChatController(
      _FakeDeniedApi(),
      () async => (token: 'token', sessionId: 'session', userId: 'user'),
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
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
