import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_access_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/voice_note_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_media_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';

import '../../helpers/stub_auth_notifier.dart';

void main() {
  testWidgets(
      'archived chat retains history/playback and hides mutation controls '
      '(RTL + LTR)', (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final api = _Api();
      final realtime = _Realtime();
      final controller = GroupChatController(
        api,
        () async => (token: 'token', sessionId: 'session', userId: 'me'),
        realtime: realtime,
      );
      final media = _MediaAccess();

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
                      preferredLanguage:
                          direction == TextDirection.rtl ? 'ar' : 'en',
                      createdAt: DateTime.utc(2026),
                    ),
                  ),
                )),
            groupChatControllerProvider('circle')
                .overrideWith((_) => controller),
            chatMediaAccessControllerProvider('voice-1')
                .overrideWith((_) => media.controller),
          ],
          child: MaterialApp(
            home: Directionality(
              textDirection: direction,
              child: const GroupChatScreen(
                circleId: 'circle',
                readOnly: true,
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();

      final rtl = direction == TextDirection.rtl;
      final playLabel =
          rtl ? ChatUiLabels.playVoiceMessage : ChatUiLabels.playVoiceMessageEn;
      final readOnlyLabel =
          rtl ? 'هذه المحادثة للقراءة فقط' : 'This conversation is read-only';

      expect(find.text('رسالة محفوظة'), findsOneWidget);
      expect(find.text('0:12'), findsOneWidget);
      expect(find.text(readOnlyLabel), findsOneWidget);
      expect(find.byType(TextField), findsNothing);
      expect(find.byType(ChatMediaComposerBar), findsNothing);

      final playSemantics = find.byWidgetPredicate(
        (widget) => widget is Semantics && widget.properties.label == playLabel,
      );
      expect(playSemantics, findsOneWidget);
      final playButton = find.widgetWithText(TextButton, playLabel);
      expect(playButton, findsOneWidget);
      final target = tester.getSize(playButton);
      expect(target.width, greaterThanOrEqualTo(44));
      expect(target.height, greaterThanOrEqualTo(44));

      await tester.tap(playButton);
      await tester.pumpAndSettle();

      expect(media.renewCalls, 1);
      expect(media.player.playedUrl, 'https://media.example/renewed');
      expect(await controller.sendText('ممنوع'), isFalse);
      expect(api.sent, isFalse);

      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pumpAndSettle();
      await realtime.dispose();
    }
    semantics.dispose();
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
  }) async =>
      ChatMessagePage(
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
          ChatMessage(
            id: 'voice-1',
            senderId: 'other',
            circleId: circleId,
            content: '',
            type: ChatMessageType.voice,
            sentAt: DateTime.utc(2026, 1, 1, 0, 1),
            deliveryStatus: ChatDeliveryStatus.delivered,
            voiceDurationSeconds: 12,
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

class _MediaAccess {
  _MediaAccess() {
    controller = ChatMediaAccessController(
      renew: () async {
        renewCalls++;
        return ChatMediaAccess(
          url: 'https://media.example/renewed',
          expiresAt: DateTime.utc(2026, 9, 30),
        );
      },
      player: player,
      download: (_) async => null,
    );
  }

  final player = _Player();
  late final ChatMediaAccessController controller;
  int renewCalls = 0;
}

class _Player implements PreviewPlayer {
  String? playedUrl;

  @override
  Future<void> playToCompletion(String filePath) async {}

  @override
  Future<void> playUrlToCompletion(String url) async => playedUrl = url;

  @override
  Stream<Duration> get positionStream => const Stream.empty();

  @override
  Future<void> dispose() async {}
}
