import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/app/chats_screen.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_discovery_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_moderation_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_picker.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/media_attachment_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/voice_note_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_media_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/direct_chat_screen.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

/// Wave 3 (US4) visual evidence harness.
///
/// The emulator screenshot channel stalled repeatedly in this environment, so
/// like Wave 4 this harness runs as a plain widget test and rasterizes each
/// state off a [RepaintBoundary], with the bundled Poppins/Cairo fonts loaded
/// so Arabic renders truthfully.
///
/// Run from `mobile/`:
///   flutter test tool/wave3_visual_capture_test.dart
///
/// PNGs land in
/// `../specs/019-mobile-app-shell-brand/evidence/screenshots/wave3/`
/// at 390dp logical width (780px at pixelRatio 2).
const _outDir =
    '../specs/019-mobile-app-shell-brand/evidence/screenshots/wave3';

const _circleId = 'wave3-circle';
const _peerId = 'wave3-peer';
const _currentUserId = 'wave3-me';

void main() {
  setUpAll(() async {
    // Load the bundled fonts so captures render real Poppins/Cairo glyphs
    // instead of the Ahem test font.
    Future<void> load(String family, String path) async {
      final bytes = await File(path).readAsBytes();
      final loader = FontLoader(family)
        ..addFont(Future.value(ByteData.sublistView(bytes)));
      await loader.load();
    }

    await load('Poppins', 'assets/fonts/Poppins-Regular.ttf');
    await load('Poppins', 'assets/fonts/Poppins-SemiBold.ttf');
    await load('Poppins', 'assets/fonts/Poppins-Bold.ttf');
    await load('Cairo', 'assets/fonts/Cairo-Regular.ttf');
    await load('Cairo', 'assets/fonts/Cairo-Bold.ttf');
    // Material icons ship with the framework, not the test environment.
    final flutterRoot = Platform.environment['FLUTTER_ROOT'];
    if (flutterRoot != null) {
      await load('MaterialIcons',
          '$flutterRoot/bin/cache/artifacts/material_fonts/MaterialIcons-Regular.otf');
    }
  });

  for (final locale in const [Locale('ar'), Locale('en')]) {
    testWidgets('wave3_chat_matrix_${locale.languageCode}', (tester) async {
      addTearDown(tester.view.reset);
      final boundaryKey = GlobalKey();
      final rtl = locale.languageCode == 'ar';

      /// Pumps the state, waits for [sentinel] to stay visible for 3
      /// consecutive frames, then writes the PNG.
      Future<void> capture({
        required String state,
        required Brightness brightness,
        required Widget child,
        List<Override> overrides = const [],
        required Finder sentinel,
        Future<void> Function()? prepare,
        Size size = const Size(390, 844),
        double textScale = 1,
      }) async {
        tester.view.devicePixelRatio = 1;
        tester.view.physicalSize = size;
        await tester.pumpWidget(const SizedBox.shrink());
        await tester.pump();
        await tester.pumpWidget(
          ProviderScope(
            overrides: [
              authControllerProvider.overrideWith(
                (_) => _VisualAuthController(),
              ),
              ...overrides,
            ],
            child: RepaintBoundary(
              key: boundaryKey,
              child: MaterialApp(
                debugShowCheckedModeBanner: false,
                theme: halaqatyLightTheme(),
                darkTheme: halaqatyDarkTheme(),
                themeMode: brightness == Brightness.dark
                    ? ThemeMode.dark
                    : ThemeMode.light,
                builder: (context, appChild) => MediaQuery(
                  data: MediaQuery.of(context).copyWith(
                    textScaler: TextScaler.linear(textScale),
                  ),
                  child: Directionality(
                    textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
                    child: appChild!,
                  ),
                ),
                home: child,
              ),
            ),
          ),
        );
        if (prepare != null) await prepare();
        var stable = 0;
        for (var i = 0; i < 60 && stable < 3; i++) {
          await tester.pump(const Duration(milliseconds: 100));
          stable = sentinel.evaluate().isNotEmpty ? stable + 1 : 0;
        }
        if (stable < 3) {
          throw StateError('Wave 3 visual sentinel never stabilized: '
              '$state ($sentinel)');
        }
        // Rasterization is real engine/async work — it must run outside the
        // fake test zone.
        await tester.runAsync(() async {
          final boundary = boundaryKey.currentContext!.findRenderObject()!
              as RenderRepaintBoundary;
          final image = await boundary.toImage(pixelRatio: 2.0);
          final byteData =
              await image.toByteData(format: ui.ImageByteFormat.png);
          final name = 'wave3_${state}_${locale.languageCode}_'
              '${brightness == Brightness.dark ? 'dark' : 'light'}.png';
          final file = await File('$_outDir/$name').create(recursive: true);
          await file.writeAsBytes(byteData!.buffer.asUint8List());
        });
      }

      Future<void> captureGroupConversation(
        Brightness brightness, {
        required String state,
        required GroupChatControllerState controllerState,
        required Finder sentinel,
        bool readOnly = false,
        Size? size,
        double textScale = 1,
      }) async {
        final api = _VisualChatApi();
        await capture(
          state: state,
          brightness: brightness,
          child: GroupChatScreen(
            circleId: _circleId,
            circleName: 'Wave 3 Circle',
            readOnly: readOnly,
          ),
          size: size ?? const Size(390, 844),
          textScale: textScale,
          overrides: _chatOverrides(
            api,
            group: _VisualGroupChatController(api, controllerState),
          ),
          sentinel: sentinel,
        );
      }

      Future<void> captureDirectConversation(
        Brightness brightness, {
        required String state,
        required List<ChatMessage> messages,
        required Finder sentinel,
      }) async {
        final api = _VisualChatApi();
        await capture(
          state: state,
          brightness: brightness,
          child: const DirectChatScreen(peerId: _peerId),
          overrides: _chatOverrides(
            api,
            direct: _VisualDirectChatController(api, messages),
          ),
          sentinel: sentinel,
        );
      }

      Future<void> captureComposerSurface(
        Brightness brightness, {
        required String state,
        required _VisualVoiceController voice,
        required Finder sentinel,
        Future<void> Function()? prepare,
      }) async {
        await capture(
          state: state,
          brightness: brightness,
          child: const Scaffold(
            body: Align(
              alignment: AlignmentDirectional.bottomCenter,
              child: ChatMediaComposerBar(circleId: _circleId),
            ),
          ),
          overrides: [
            voiceNoteControllerProvider(_circleId).overrideWith((_) => voice),
            mediaAttachmentControllerProvider(_circleId)
                .overrideWith((_) => _VisualAttachmentController()),
          ],
          sentinel: sentinel,
          prepare: prepare,
        );
      }

      Future<void> captureResponsiveConversation(
        Brightness brightness, {
        required String state,
        required Size size,
        double textScale = 1,
      }) =>
          captureGroupConversation(
            brightness,
            state: state,
            controllerState: GroupChatControllerState(
              status: GroupChatStatus.ready,
              messages: _historyMessages(),
            ),
            sentinel: find.text('Ready for review'),
            size: size,
            textScale: textScale,
          );

      for (final brightness in Brightness.values) {
        for (final state in _chatListStates) {
          await capture(
            state: 'chats_${state.name}',
            brightness: brightness,
            child: const ChatsScreen(),
            overrides: [
              circleDiscoveryControllerProvider.overrideWith(
                (_) => _VisualCircleDiscoveryController(state.value),
              ),
            ],
            sentinel: state.sentinel,
          );
        }

        await captureGroupConversation(
          brightness,
          state: 'group_empty',
          controllerState: const GroupChatControllerState(
            status: GroupChatStatus.ready,
          ),
          sentinel: find.text(rtl
              ? 'لا توجد رسائل بعد؛ ابدأ المحادثة'
              : 'No messages yet; start the conversation'),
        );
        await captureGroupConversation(
          brightness,
          state: 'group_history',
          controllerState: GroupChatControllerState(
            status: GroupChatStatus.ready,
            messages: _historyMessages(),
          ),
          sentinel: find.text('Ready for review'),
        );
        await captureDirectConversation(
          brightness,
          state: 'direct_empty',
          messages: const [],
          sentinel: find.byType(TextField),
        );
        await captureDirectConversation(
          brightness,
          state: 'direct_history',
          messages: _directMessages(),
          sentinel: find.text('Direct reply'),
        );

        await captureComposerSurface(
          brightness,
          state: 'attachment_sheet',
          voice: _VisualVoiceController(),
          sentinel: find.text(
              rtl ? ChatUiLabels.attachImage : ChatUiLabels.attachImageEn),
          prepare: () async {
            await tester.pump();
            await tester.pump(const Duration(milliseconds: 100));
            await tester.tap(
                find.text(rtl ? ChatUiLabels.attach : ChatUiLabels.attachEn));
            await tester.pump();
            await tester.pump(const Duration(milliseconds: 500));
          },
        );
        await captureComposerSurface(
          brightness,
          state: 'voice_preview',
          voice: _VisualVoiceController(
            const VoiceNoteState(
              phase: VoiceNotePhase.recorded,
              duration: Duration(seconds: 18),
              amplitudes: [-48, -30, -12, -24, -8, -36],
            ),
          ),
          sentinel: find.text('0:18'),
        );

        final failed = _message(
          'failed-draft',
          content: rtl ? 'رسالة لم تُرسل' : 'Message not sent',
          senderId: _currentUserId,
          status: ChatDeliveryStatus.pending,
        );
        await captureGroupConversation(
          brightness,
          state: 'failed_send',
          controllerState: GroupChatControllerState(
            status: GroupChatStatus.ready,
            messages: [failed],
            terminalFailures: const {'failed-draft': 'offline'},
          ),
          sentinel: find.text(
            rtl ? ChatUiLabels.sendFailed : ChatUiLabels.sendFailedEn,
          ),
        );

        await capture(
          state: 'offline_draft',
          brightness: brightness,
          child: Scaffold(
            appBar: AppBar(
              title: Text(rtl ? 'مسودة دون اتصال' : 'Offline draft'),
            ),
            body: ChatComposer(onSend: (_) async => false),
          ),
          sentinel: find.text(rtl ? 'مسودة محفوظة' : 'Saved draft'),
          prepare: () async {
            await tester.enterText(
              find.byType(TextField),
              rtl ? 'مسودة محفوظة' : 'Saved draft',
            );
            await tester.pump();
          },
        );

        await captureGroupConversation(
          brightness,
          state: 'archived_read_only',
          readOnly: true,
          controllerState: GroupChatControllerState(
            status: GroupChatStatus.ready,
            messages: _historyMessages(),
            readOnly: true,
          ),
          sentinel: find.text(rtl
              ? ChatUiLabels.readOnlyArchived
              : ChatUiLabels.readOnlyArchivedEn),
        );
        await captureGroupConversation(
          brightness,
          state: 'access_lost',
          controllerState: const GroupChatControllerState(
            status: GroupChatStatus.accessLost,
          ),
          sentinel: find
              .text(rtl ? ChatUiLabels.accessLost : ChatUiLabels.accessLostEn),
        );

        await captureResponsiveConversation(
          brightness,
          state: 'conversation_320dp',
          size: const Size(320, 720),
        );
        await captureResponsiveConversation(
          brightness,
          state: 'conversation_600dp',
          size: const Size(600, 900),
        );
        await captureResponsiveConversation(
          brightness,
          state: 'conversation_text_200pct',
          size: const Size(390, 844),
          textScale: 2,
        );
      }
    });
  }
}

final _chatListStates = <({
  String name,
  CircleDiscoveryState value,
  Finder sentinel,
})>[
  (
    name: 'empty',
    value: const CircleDiscoveryState(),
    sentinel: find.byKey(const Key('chatsEmpty')),
  ),
  (
    name: 'error',
    value: const CircleDiscoveryState(failure: CircleJoinFailure.network),
    sentinel: find.byKey(const Key('circleLoadError')),
  ),
  (
    name: 'loaded',
    value: CircleDiscoveryState(myCircles: [_circle]),
    sentinel: find.byKey(const Key('chatCircle-$_circleId')),
  ),
];

final _circle = CircleSummary(
  id: _circleId,
  name: 'حلقة الإتقان',
  description: 'Wave 3 visual fixture',
  maxCapacity: 20,
  genderRestriction: 'mixed',
  language: 'ar',
  createdAt: DateTime.utc(2026, 9, 1),
);

List<Override> _chatOverrides(
  _VisualChatApi api, {
  _VisualGroupChatController? group,
  _VisualDirectChatController? direct,
}) =>
    [
      if (group != null)
        groupChatControllerProvider(_circleId).overrideWith((_) => group),
      if (direct != null)
        directChatControllerProvider(_peerId).overrideWith((_) => direct),
      chatModerationControllerProvider.overrideWith(
        (_) => ChatModerationController(api, _credentials),
      ),
      chatDiscoveryControllerProvider(_circleId)
          .overrideWith((_) => _VisualDiscoveryController()),
      circleMembersProvider(_circleId).overrideWith((_) => Future.value([])),
      voiceNoteControllerProvider(_circleId)
          .overrideWith((_) => _VisualVoiceController()),
      mediaAttachmentControllerProvider(_circleId)
          .overrideWith((_) => _VisualAttachmentController()),
    ];

Future<({String token, String sessionId, String userId})>
    _credentials() async => (
          token: 'visual-token',
          sessionId: 'visual-session',
          userId: _currentUserId
        );

List<ChatMessage> _historyMessages() => [
      _message(
        'read-own',
        content: 'Ready for review',
        senderId: _currentUserId,
        senderName: 'Karim',
        status: ChatDeliveryStatus.read,
      ),
      _message(
        'delivered-other',
        content: 'بارك الله فيكم',
        senderId: _peerId,
        senderName: 'مريم',
      ),
    ];

List<ChatMessage> _directMessages() => [
      _message(
        'direct-own',
        content: 'Direct reply',
        senderId: _currentUserId,
        dmPeerId: _peerId,
        circleId: null,
        status: ChatDeliveryStatus.read,
      ),
      _message(
        'direct-other',
        content: 'السلام عليكم',
        senderId: _peerId,
        senderName: 'Ahmed',
        dmPeerId: _peerId,
        circleId: null,
      ),
    ];

ChatMessage _message(
  String id, {
  required String content,
  required String senderId,
  String? senderName,
  String? circleId = _circleId,
  String? dmPeerId,
  ChatDeliveryStatus status = ChatDeliveryStatus.delivered,
}) =>
    ChatMessage(
      id: id,
      senderId: senderId,
      senderName: senderName,
      circleId: circleId,
      dmPeerId: dmPeerId,
      content: content,
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 23, 8),
      deliveryStatus: status,
    );

class _VisualAuthController extends StateNotifier<AuthState>
    implements AuthController {
  _VisualAuthController()
      : super(
          AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'visual-session',
            user: BackendUser(
              id: _currentUserId,
              firebaseUid: 'visual-firebase',
              preferredLanguage: 'ar',
              createdAt: DateTime.utc(2026, 1, 1),
            ),
          ),
        );

  @override
  Future<void> logout() async {}

  @override
  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async {}

  @override
  Future<void> signIn(
      {required String email, required String password}) async {}
}

class _VisualCircleDiscoveryController extends CircleDiscoveryController {
  _VisualCircleDiscoveryController(CircleDiscoveryState initialState)
      : super(
          apiClient: _VisualCircleApi(),
          loadFirebaseIdToken: () async => 'visual-token',
          readAuthState: () => const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'visual-session',
          ),
          logout: () async {},
        ) {
    state = initialState;
  }

  @override
  Future<void> loadMyCircles() async {}
}

class _VisualGroupChatController extends GroupChatController {
  _VisualGroupChatController(
    _VisualChatApi api,
    GroupChatControllerState initialState,
  ) : super(api, _credentials, realtime: _VisualRealtime()) {
    state = initialState;
  }

  @override
  Future<void> open(String circleId) async {}
}

class _VisualDirectChatController extends DirectChatController {
  _VisualDirectChatController(_VisualChatApi api, List<ChatMessage> messages)
      : super(api, _credentials) {
    state = DirectChatControllerState(
      status: DirectChatStatus.ready,
      messages: messages,
    );
  }

  @override
  Future<void> open(String peerId) async {}
}

class _VisualVoiceController extends VoiceNoteController {
  _VisualVoiceController([VoiceNoteState initialState = const VoiceNoteState()])
      : super(
          upload: (_, __) async => const ChatUploadResult(
            url: 'https://media.example.test/voice',
            uploadId: 'visual',
          ),
          recorder: _VisualRecorder(),
          player: _VisualPlayer(),
        ) {
    state = initialState;
  }
}

class _VisualAttachmentController extends MediaAttachmentController {
  _VisualAttachmentController()
      : super(
          picker: _VisualPicker(),
          uploadImage: (_, __) async => const ChatUploadResult(
            url: 'https://media.example.test/image',
            uploadId: 'visual-image',
          ),
          uploadFile: (_, __) async => const ChatUploadResult(
            url: 'https://media.example.test/file',
            uploadId: 'visual-file',
          ),
          attach: (_, __, ___) async => _message(
            'visual-attachment',
            content: '',
            senderId: _currentUserId,
          ),
        );
}

class _VisualCircleApi extends CircleApiClient {
  _VisualCircleApi() : super(Dio());
}

class _VisualDiscoveryController extends ChatDiscoveryController {
  _VisualDiscoveryController() : super(_VisualDiscoveryApi(), _credentials) {
    state = const ChatDiscoveryState(status: ChatDiscoveryStatus.ready);
  }

  @override
  Future<void> loadPinned(String circleId) async {}
}

class _VisualDiscoveryApi implements ChatDiscoveryApi {
  @override
  Future<ChatMessage> sendReply({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String? replyToId,
  }) =>
      throw UnimplementedError();

  @override
  Future<List<ChatMessage>> listPinnedMessages({
    required String token,
    required String sessionId,
    required String circleId,
  }) async =>
      const [];

  @override
  Future<ChatMessagePage> searchMessages({
    required String token,
    required String sessionId,
    required String circleId,
    required String query,
  }) async =>
      const ChatMessagePage(messages: [], hasMore: false);

  @override
  Future<ChatMessage> pinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) =>
      throw UnimplementedError();

  @override
  Future<void> unpinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) =>
      throw UnimplementedError();
}

class _VisualChatApi extends ChatApiClient {
  _VisualChatApi() : super(Dio());
}

class _VisualRealtime implements ChatRealtimeClient {
  @override
  Stream<ChatRealtimeEvent> circleChatEvents(
    String circleId, {
    required String token,
    required String backendSessionId,
  }) =>
      const Stream.empty();

  @override
  Future<void> dispose() async {}
}

class _VisualPicker implements ChatAttachmentPicker {
  @override
  Future<String?> pickImage() async => null;

  @override
  Future<String?> pickPdf() async => null;
}

class _VisualRecorder implements VoiceRecorder {
  @override
  Stream<double> amplitudeStream() => const Stream.empty();

  @override
  Future<void> dispose() async {}

  @override
  Future<bool> hasPermission() async => true;

  @override
  Future<void> start(String filePath) async {}

  @override
  Future<String?> stop() async => null;
}

class _VisualPlayer implements PreviewPlayer {
  @override
  Future<void> dispose() async {}

  @override
  Future<void> playToCompletion(String filePath) async {}

  @override
  Future<void> playUrlToCompletion(String url) async {}

  @override
  Stream<Duration> get positionStream => const Stream.empty();
}
