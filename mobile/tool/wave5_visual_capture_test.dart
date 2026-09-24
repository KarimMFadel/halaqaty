import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/app/chats_screen.dart';
import 'package:halaqaty_mobile/app/router.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_picker.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_moderation_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/media_attachment_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/voice_note_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/create_circle_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';
import 'package:halaqaty_mobile/features/sessions/application/circle_sessions_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';

/// Wave 5 (US6) visual evidence harness — T043/T046.
///
/// Same off-device rasterization approach as `wave4_visual_capture_test.dart`
/// (the emulator is shared): each state is pumped in a widget test with the
/// bundled Poppins/Cairo fonts loaded and rasterized off a [RepaintBoundary].
///
/// Wave 5 owns the Screenshot Acceptance Matrix captures that waves 0–4
/// deferred: 320dp and 600dp width variants, 200% text-scale variants, the
/// corrected Waves 0–4 violations (RTL chevrons, circle-detail Schedule
/// notice-only action), and visible keyboard-focus evidence.
///
/// Run from `mobile/`:
///   flutter test tool/wave5_visual_capture_test.dart
///
/// PNGs land in
/// `../specs/019-mobile-app-shell-brand/evidence/screenshots/wave5/`.
const _outDir =
    '../specs/019-mobile-app-shell-brand/evidence/screenshots/wave5';

const _roomId = 'wave5-room';
const _circleId = 'circle-1';
const _meId = 'me-1';

/// One locale/theme combination of the acceptance matrix.
class _Variant {
  const _Variant(this.languageCode, this.brightness);

  final String languageCode;
  final Brightness brightness;

  bool get rtl => languageCode == 'ar';
  Locale get locale => Locale(languageCode);
  String get tag =>
      '${languageCode}_${brightness == Brightness.dark ? 'dark' : 'light'}';
}

/// Width/text-scale matrix captures use Arabic light + English dark so each
/// capture exercises one direction and one theme without doubling the count.
const _matrixVariants = [
  _Variant('ar', Brightness.light),
  _Variant('en', Brightness.dark),
];

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

  testWidgets('wave5 matrix at 320dp width', (tester) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _runMatrix(_Rig(tester), width: 320, suffix: '320dp');
  });

  testWidgets('wave5 matrix at 600dp width', (tester) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _runMatrix(_Rig(tester), width: 600, suffix: '600dp');
  });

  testWidgets('wave5 matrix at 200% text scale', (tester) async {
    tester.platformDispatcher.textScaleFactorTestValue = 2.0;
    addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await _runMatrix(_Rig(tester),
        width: 390, suffix: 'text200', textScale: true);
  });

  testWidgets('wave5 corrected-violation captures', (tester) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final rig = _Rig(tester);

    // T045 fixes under evidence: the Schedule notice-only action and the
    // single direction-aware chevrons (FR-010/FR-032).
    for (final variant in const [
      _Variant('ar', Brightness.light),
      _Variant('en', Brightness.light),
    ]) {
      await rig.capture(
        name: 'circle_detail_schedule_${variant.tag}',
        screen: const CircleDetailScreen(
            circleId: _circleId, currentUserId: 'student-1'),
        variant: variant,
        overrides: _circleDetailOverrides(),
        size: const Size(390, 1600),
        sentinel: find.text(variant.rtl ? 'المواعيد' : 'Schedule'),
      );

      await rig.capture(
        name: 'circle_detail_notice_${variant.tag}',
        screen: const CircleDetailScreen(
            circleId: _circleId, currentUserId: 'student-1'),
        variant: variant,
        overrides: _circleDetailOverrides(),
        size: const Size(390, 1600),
        sentinel: find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        prepare: () async {
          final schedule = find.text(variant.rtl ? 'المواعيد' : 'Schedule');
          await rig.tester.pumpAndSettle();
          await rig.tester.ensureVisible(schedule);
          await rig.tester.pumpAndSettle();
          await rig.tester.tap(schedule);
          await rig.tester.pump(const Duration(milliseconds: 400));
        },
      );
    }

    // Home circle-card chevron after the double-mirror fix, in the real
    // authenticated shell (navigation bar visible) at RTL.
    await rig.captureShell(
      name: 'home_chevron_rtl_ar_light',
      variant: const _Variant('ar', Brightness.light),
      overrides: _shellCircleOverrides(),
      size: const Size(390, 844),
      sentinel: find.byKey(const Key('homeCircle-circle-1')),
      extraSentinels: [find.byKey(const Key('appNavigationBar'))],
    );

    // Chats circle-tile chevron after the double-mirror fix, RTL.
    await rig.capture(
      name: 'chats_chevron_rtl_ar_light',
      screen: const ChatsScreen(),
      variant: const _Variant('ar', Brightness.light),
      overrides: _circleOverrides(),
      size: const Size(390, 844),
      sentinel: find.byKey(const Key('chatCircle-circle-1')),
    );

    // Leave no snackbar timer pending at test end.
    await rig.tester.pumpWidget(const SizedBox.shrink());
    await rig.tester.pump();
  });

  testWidgets('wave5 visible focus evidence', (tester) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final rig = _Rig(tester);

    for (final variant in const [
      _Variant('ar', Brightness.light),
      _Variant('en', Brightness.light),
    ]) {
      const saveKey = Key('profileSaveButton');
      await rig.capture(
        name: 'profile_save_focus_${variant.tag}',
        screen: const ProfileScreen(),
        variant: variant,
        overrides: _profileOverrides(variant),
        size: const Size(390, 844),
        sentinel: find.byKey(saveKey),
        prepare: () async {
          await rig.tester.pumpAndSettle();
          await rig.tester.ensureVisible(find.byKey(saveKey));
          await rig.tester.pumpAndSettle();
          // Keyboard-tab to the primary action so the focus ring is visible
          // in the capture (FR-014 evidence).
          var focused = false;
          for (var i = 0; i < 40 && !focused; i++) {
            await rig.tester.sendKeyEvent(LogicalKeyboardKey.tab);
            await rig.tester.pump();
            final context = FocusManager.instance.primaryFocus?.context;
            context?.visitAncestorElements((ancestor) {
              if (ancestor.widget.key == saveKey) {
                focused = true;
                return false;
              }
              return true;
            });
          }
          if (!focused) {
            throw StateError('profile save button is not keyboard-focusable');
          }
        },
      );
    }
  });
}

/// Owns the widget tester, the repaint boundary, and PNG rasterization.
class _Rig {
  _Rig(this.tester);

  final WidgetTester tester;
  final GlobalKey boundaryKey = GlobalKey();

  /// Pumps a standalone screen inside the harness MaterialApp (explicit
  /// light/dark theme + forced direction), settles, verifies the sentinels,
  /// and writes `wave5_<name>.png` at pixelRatio 2.
  Future<void> capture({
    required String name,
    required Widget screen,
    required _Variant variant,
    required List<Override> overrides,
    required Size size,
    required Finder sentinel,
    List<Finder> extraSentinels = const [],
    Future<void> Function()? prepare,
    bool settle = true,
  }) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          platformLocaleProvider.overrideWithValue(variant.locale),
          ...overrides,
        ],
        child: RepaintBoundary(
          key: boundaryKey,
          child: MaterialApp(
            debugShowCheckedModeBanner: false,
            theme: halaqatyLightTheme(),
            darkTheme: halaqatyDarkTheme(),
            themeMode: variant.brightness == Brightness.dark
                ? ThemeMode.dark
                : ThemeMode.light,
            builder: (context, child) => Directionality(
              textDirection:
                  variant.rtl ? TextDirection.rtl : TextDirection.ltr,
              child: child!,
            ),
            home: screen,
          ),
        ),
      ),
    );
    await _settleAndRasterize(name, sentinel, extraSentinels,
        prepare: prepare, settle: settle);
  }

  /// Pumps the real go_router shell (`appNavigationBar` visible) for
  /// shell-level captures. Uses the same router MyApp builds for the
  /// authenticated state, wrapped in a banner-free harness MaterialApp so
  /// evidence matches the standalone captures; direction and theme are
  /// forced explicitly instead of via the platform dispatcher.
  Future<void> captureShell({
    required String name,
    required _Variant variant,
    required List<Override> overrides,
    required Size size,
    required Finder sentinel,
    List<Finder> extraSentinels = const [],
    Future<void> Function()? prepare,
  }) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          platformLocaleProvider.overrideWithValue(variant.locale),
          ...overrides,
        ],
        child: RepaintBoundary(
          key: boundaryKey,
          child: MaterialApp.router(
            debugShowCheckedModeBanner: false,
            theme: halaqatyLightTheme(),
            darkTheme: halaqatyDarkTheme(),
            themeMode: variant.brightness == Brightness.dark
                ? ThemeMode.dark
                : ThemeMode.light,
            builder: (context, child) => Directionality(
              textDirection:
                  variant.rtl ? TextDirection.rtl : TextDirection.ltr,
              child: child!,
            ),
            routerConfig: buildHalaqatyRouter(AuthStatus.authenticated),
          ),
        ),
      ),
    );
    await _settleAndRasterize(name, sentinel, extraSentinels, prepare: prepare);
  }

  Future<void> _settleAndRasterize(
    String name,
    Finder sentinel,
    List<Finder> extraSentinels, {
    Future<void> Function()? prepare,
    bool settle = true,
  }) async {
    if (prepare != null) await prepare();
    if (settle) {
      await tester.pumpAndSettle();
    } else {
      for (var i = 0; i < 3; i++) {
        await tester.pump(const Duration(milliseconds: 200));
      }
    }
    for (final finder in [sentinel, ...extraSentinels]) {
      if (finder.evaluate().isEmpty) {
        throw StateError('Sentinel missing for $name: $finder');
      }
    }
    // Rasterization is real engine/async work — it must run outside the
    // fake test zone.
    await tester.runAsync(() async {
      final boundary = boundaryKey.currentContext!.findRenderObject()!
          as RenderRepaintBoundary;
      final image = await boundary.toImage(pixelRatio: 2.0);
      final byteData = await image.toByteData(format: ui.ImageByteFormat.png);
      final file =
          await File('$_outDir/wave5_$name.png').create(recursive: true);
      await file.writeAsBytes(byteData!.buffer.asUint8List());
    });
  }
}

// ---------------------------------------------------------------------------
// Width / text-scale matrix (waves 0–4 deferrals: spec.md Screenshot
// Acceptance Matrix, Wave 5 row)
// ---------------------------------------------------------------------------

/// Captures every Wave-5-owned ready state at [width] with Arabic light and
/// English dark variants. [textScale] swaps in the long mixed-script and
/// diacritic fixtures (FR-015 edge cases) and enlarges the viewport heights.
Future<void> _runMatrix(
  _Rig rig, {
  required double width,
  required String suffix,
  bool textScale = false,
}) async {
  double h(double base) => textScale ? base * 1.8 : base;
  for (final variant in _matrixVariants) {
    final tag = '${variant.tag}_$suffix';

    // --- Home (authenticated shell, navigation bar visible) ----------------
    await rig.captureShell(
      name: 'home_loaded_$tag',
      variant: variant,
      overrides: _shellCircleOverrides(longFixture: textScale),
      size: Size(width, h(844)),
      sentinel: find.byKey(const Key('homeCircle-circle-1')),
      extraSentinels: [find.byKey(const Key('appNavigationBar'))],
    );

    // --- Circle discovery --------------------------------------------------
    await rig.capture(
      name: 'discovery_loaded_$tag',
      screen: const CircleDiscoveryScreen(),
      variant: variant,
      overrides: _circleOverrides(),
      size: Size(width, h(1000)),
      sentinel: find.byKey(const Key('openCircle-circle-1')),
      extraSentinels: [find.byKey(const Key('joinCircle-circle-2'))],
    );

    // --- Circle detail (student role) --------------------------------------
    await rig.capture(
      name: 'circle_detail_student_$tag',
      screen: const CircleDetailScreen(
          circleId: _circleId, currentUserId: 'student-1'),
      variant: variant,
      overrides: _circleDetailOverrides(longFixture: textScale),
      size: Size(width, h(1600)),
      sentinel: find.text(variant.rtl ? 'المواعيد' : 'Schedule'),
    );

    // --- Student session room: connected, current turn ---------------------
    final studentRealtime = _CaptureSessionRealtime();
    final studentQueue = QueueController(
      _CaptureQueueApi(),
      _sessionCredentials,
      realtime: studentRealtime,
    );
    final studentRoom = SessionRoomController(
      _CaptureSessionApi(isModerator: false),
      _sessionCredentials,
      _CaptureMediaSession(),
      realtime: studentRealtime,
      queue: studentQueue,
      currentUserId: 'student-1',
    );
    await rig.capture(
      name: 'session_student_turn_$tag',
      screen: const SessionRoomScreen(sessionId: _roomId),
      variant: variant,
      overrides: [
        sessionRoomControllerProvider(_roomId).overrideWith((_) => studentRoom),
      ],
      size: Size(width, h(1400)),
      sentinel: find.text(variant.rtl
          ? 'تم الاتصال. الصوت متاح.'
          : 'Connected. Audio is ready.'),
      // The reciting entry belongs to the captured student: their own turn.
      extraSentinels: [find.text(variant.rtl ? 'يتلو الآن' : 'Reciting')],
      prepare: () async {
        await studentRoom.join(_roomId);
      },
    );

    // --- Manager session room: queue ready + inline grading panel ----------
    final managerRealtime = _CaptureSessionRealtime();
    final managerQueue = QueueController(
      _CaptureQueueApi(),
      _sessionCredentials,
      realtime: managerRealtime,
      isManager: true,
    );
    final managerRoom = SessionRoomController(
      _CaptureSessionApi(isModerator: true),
      _sessionCredentials,
      _CaptureMediaSession(),
      realtime: managerRealtime,
      isModerator: true,
      queue: managerQueue,
    );
    await rig.capture(
      name: 'session_manager_grading_$tag',
      screen: const SessionRoomScreen(sessionId: _roomId),
      variant: variant,
      overrides: [
        sessionRoomControllerProvider(_roomId).overrideWith((_) => managerRoom),
      ],
      size: Size(width, h(1700)),
      sentinel: find.text(variant.rtl ? 'قائمة التلاوة' : 'Recitation queue'),
      // The reciting selected entry surfaces the localized grading panel.
      extraSentinels: [find.text(variant.rtl ? 'ممتاز' : 'Excellent')],
      prepare: () async {
        await managerRoom.join(_roomId);
      },
    );

    // --- Group conversation with history ------------------------------------
    // Only the deferred width/text-scale extras; the 4-variant ready-state
    // conversation captures are owned by the concurrent Wave 3 agent.
    final chatApi = _CaptureChatApi()
      ..page = ChatMessagePage(
          messages: _chatHistory(diacritics: textScale), hasMore: false);
    Future<({String token, String sessionId, String userId})>
        chatCredentials() async =>
            (token: 'token', sessionId: 'capture-session', userId: _meId);
    await rig.capture(
      name: 'chat_conversation_$tag',
      screen:
          const GroupChatScreen(circleId: _circleId, circleName: 'حلقة الفجر'),
      variant: variant,
      overrides: [
        authControllerProvider
            .overrideWith((_) => _CaptureAuth.authenticated(userId: _meId)),
        groupChatControllerProvider(_circleId).overrideWith((_) =>
            GroupChatController(chatApi, chatCredentials,
                realtime: _CaptureChatRealtime())),
        chatModerationControllerProvider.overrideWith(
            (_) => ChatModerationController(chatApi, chatCredentials)),
        // The composer bar's platform seams (record/just_audio/picker) are
        // pure-Dart fakes; the ready-state capture never records or uploads.
        voiceNoteControllerProvider(_circleId).overrideWith(
          (_) => VoiceNoteController(
            recorder: _CaptureRecorder(),
            player: _CapturePlayer(),
            upload: (filePath, durationSeconds) async => _chatUploadResult(),
          ),
        ),
        mediaAttachmentControllerProvider(_circleId).overrideWith(
          (_) => MediaAttachmentController(
            picker: _CapturePicker(),
            uploadImage: (filePath, onProgress) async => _chatUploadResult(),
            uploadFile: (filePath, onProgress) async => _chatUploadResult(),
            attach: (type, uploadId, idempotencyKey) async => ChatMessage(
              id: 'attach-1',
              senderId: _meId,
              circleId: _circleId,
              content: '',
              type: type,
              sentAt: DateTime.utc(2026, 9, 3, 12),
              deliveryStatus: ChatDeliveryStatus.delivered,
            ),
          ),
        ),
      ],
      size: Size(width, h(844)),
      sentinel: find.text('السلام عليكم، هل راجعتم ورد اليوم؟'),
    );

    // --- Auth forms ---------------------------------------------------------
    await rig.capture(
      name: 'login_ready_$tag',
      screen: const LoginScreen(),
      variant: variant,
      overrides: _authOverrides(),
      size: Size(width, h(844)),
      sentinel: find.byKey(const Key('submitButton')),
    );

    await rig.capture(
      name: 'register_ready_$tag',
      screen: const RegisterScreen(),
      variant: variant,
      overrides: _authOverrides(),
      size: Size(width, h(1000)),
      sentinel: find.byKey(const Key('languageDropdown')),
    );

    await rig.capture(
      name: 'register_validation_$tag',
      screen: const RegisterScreen(),
      variant: variant,
      overrides: _authOverrides(),
      size: Size(width, h(1000)),
      sentinel: find.text(
          variant.rtl ? 'الاسم المعروض مطلوب' : 'Display name is required'),
      prepare: () async {
        await rig.tester.pumpAndSettle();
        await rig.tester.ensureVisible(find.byKey(const Key('submitButton')));
        await rig.tester.pumpAndSettle();
        await rig.tester.tap(find.byKey(const Key('submitButton')));
      },
    );

    // --- Profile ------------------------------------------------------------
    await rig.capture(
      name: 'profile_ready_$tag',
      screen: const ProfileScreen(),
      variant: variant,
      overrides: _profileOverrides(variant),
      size: Size(width, h(1100)),
      sentinel: find.byKey(const Key('profileSaveButton')),
      prepare: () async {
        await rig.tester.pumpAndSettle();
        await rig.tester
            .ensureVisible(find.byKey(const Key('profileSaveButton')));
      },
    );
  }
}

// ---------------------------------------------------------------------------
// Fixtures and override builders
// ---------------------------------------------------------------------------

CircleSummary _myCircle({bool longFixture = false}) => CircleSummary(
      id: _circleId,
      name: longFixture
          ? 'حلقة تلاوة Quran Recitation Circle — جزء عمّ وأحكام التجويد للمبتدئين الجدد'
          : 'حلقة الفجر',
      description: longFixture
          ? 'بِسْمِ ٱللَّهِ ٱلرَّحْمَٰنِ ٱلرَّحِيمِ — تلاوة ومراجعة مع تصحيح التلاوة'
          : 'تلاوة جزء عمّ بعد الفجر',
      maxCapacity: 12,
      genderRestriction: 'mixed',
      language: 'ar',
      createdAt: DateTime.utc(2026, 8, 1),
    );

CircleSummary _publicCircle() => CircleSummary(
      id: 'circle-2',
      name: 'Tajweed Foundations — أساسيات التجويد',
      description: 'أحكام النون الساكنة والتنوين للمبتدئين',
      maxCapacity: 15,
      genderRestriction: 'mixed',
      language: 'ar',
      createdAt: DateTime.utc(2026, 8, 3),
    );

List<ChatMessage> _chatHistory({bool diacritics = false}) => [
      ChatMessage(
        id: 'm1',
        senderId: 'other-1',
        circleId: _circleId,
        content: 'السلام عليكم، هل راجعتم ورد اليوم؟',
        type: ChatMessageType.text,
        sentAt: DateTime.utc(2026, 9, 3, 12),
        deliveryStatus: ChatDeliveryStatus.delivered,
        senderName: 'مريم',
      ),
      ChatMessage(
        id: 'm2',
        senderId: _meId,
        circleId: _circleId,
        content: 'وعليكم السلام، نعم الحمد لله',
        type: ChatMessageType.text,
        sentAt: DateTime.utc(2026, 9, 3, 12, 1),
        deliveryStatus: ChatDeliveryStatus.read,
      ),
      ChatMessage(
        id: 'm3',
        senderId: 'other-1',
        circleId: _circleId,
        content: 'أحسنت، نبدأ التلاوة بعد المغرب إن شاء الله',
        type: ChatMessageType.text,
        sentAt: DateTime.utc(2026, 9, 3, 12, 2),
        deliveryStatus: ChatDeliveryStatus.delivered,
        senderName: 'مريم',
      ),
      if (diacritics)
        ChatMessage(
          id: 'm4',
          senderId: 'other-1',
          circleId: _circleId,
          content:
              'بِسْمِ ٱللَّهِ ٱلرَّحْمَٰنِ ٱلرَّحِيمِ — الٓمٓ ذَٰلِكَ ٱلْكِتَٰبُ لَا رَيْبَ فِيهِ هُدًى لِّلْمُتَّقِينَ',
          type: ChatMessageType.text,
          sentAt: DateTime.utc(2026, 9, 3, 12, 3),
          deliveryStatus: ChatDeliveryStatus.delivered,
          senderName: 'مريم',
        ),
    ];

List<Override> _authOverrides() => [
      authControllerProvider
          .overrideWith((_) => _CaptureAuth.unauthenticated()),
    ];

List<Override> _profileOverrides(_Variant variant) => [
      profileControllerProvider
          .overrideWith((_) => _CaptureProfile(locale: variant.locale)),
      authControllerProvider.overrideWith((_) => _CaptureAuth.authenticated()),
    ];

CircleDiscoveryController _discoveryController(_CaptureCircleApi api) =>
    CircleDiscoveryController(
      apiClient: api,
      loadFirebaseIdToken: () async => 'capture-token',
      readAuthState: () => const AuthState(
        status: AuthStatus.authenticated,
        sessionId: 'capture-session',
      ),
      logout: () async {},
    );

/// Standalone home/discovery/chats screens: auth + discovery controller.
List<Override> _circleOverrides({bool longFixture = false}) {
  final api = _CaptureCircleApi()
    ..myCircles = [_myCircle(longFixture: longFixture)]
    ..publicCircles = [_publicCircle()];
  return [
    authControllerProvider.overrideWith((_) => _CaptureAuth.authenticated()),
    circleDiscoveryControllerProvider
        .overrideWith((_) => _discoveryController(api)),
  ];
}

/// Shell captures: the authenticated router also reads the create-circle and
/// profile controllers for the remaining destinations.
List<Override> _shellCircleOverrides({bool longFixture = false}) {
  final api = _CaptureCircleApi()
    ..myCircles = [_myCircle(longFixture: longFixture)]
    ..publicCircles = [_publicCircle()];
  return [
    authControllerProvider.overrideWith((_) => _CaptureAuth.authenticated()),
    profileControllerProvider.overrideWith((_) => _CaptureProfile()),
    circleDiscoveryControllerProvider
        .overrideWith((_) => _discoveryController(api)),
    createCircleControllerProvider.overrideWith(
      (_) => CreateCircleController(
        apiClient: api,
        loadFirebaseIdToken: () async => 'capture-token',
        readAuthState: () => const AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'capture-session',
        ),
        logout: () async {},
      ),
    ),
  ];
}

/// Circle detail as a student: stubbed detail/members futures plus an idle
/// sessions controller so the screen renders its full ready state.
List<Override> _circleDetailOverrides({bool longFixture = false}) => [
      authControllerProvider
          .overrideWith((_) => _CaptureAuth.authenticated(userId: 'student-1')),
      circleDetailProvider(_circleId).overrideWith(
        (_) => Future.value(
          CircleResponse(
            id: _circleId,
            name: _myCircle(longFixture: longFixture).name,
            inviteCode: 'HLQ-7X2K',
            inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
            createdAt: DateTime.utc(2026, 8, 1),
          ),
        ),
      ),
      circleMembersProvider(_circleId).overrideWith(
        (_) => Future.value([
          CircleMember(
            userId: 'teacher-1',
            displayName: 'الشيخ أحمد',
            role: CircleRole.teacher,
            joinedAt: DateTime.utc(2026, 8, 1),
          ),
          CircleMember(
            userId: 'student-1',
            displayName: 'علي',
            role: CircleRole.student,
            joinedAt: DateTime.utc(2026, 8, 2),
          ),
        ]),
      ),
      circleSessionsControllerProvider(_circleId)
          .overrideWith((_) => _CaptureCircleSessions()),
    ];

Future<({String token, String sessionId})> _sessionCredentials() async =>
    (token: 'token', sessionId: 'capture-session');

// ---------------------------------------------------------------------------
// Stubs (no Firebase, Dio transport, or platform plugins)
// ---------------------------------------------------------------------------

class _CaptureAuth extends StateNotifier<AuthState> implements AuthController {
  _CaptureAuth.authenticated({String userId = 'user-1'})
      : super(AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'capture-session',
          user: BackendUser(
            id: userId,
            firebaseUid: 'fb-$userId',
            preferredLanguage: 'ar',
            createdAt: DateTime.utc(2026, 1, 1),
          ),
        ));

  _CaptureAuth.unauthenticated()
      : super(const AuthState(status: AuthStatus.unauthenticated));

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
  Future<void> logout() async {}
}

class _CaptureProfile extends StateNotifier<ProfileState>
    implements ProfileController {
  _CaptureProfile({Locale locale = const Locale('ar')})
      : _language = locale.languageCode,
        super(const ProfileState());

  final String _language;

  @override
  Future<void> loadProfile() async {
    final ar = _language == 'ar';
    state = ProfileState(
      profile: ProfileUser(
        id: 'user-1',
        firebaseUid: 'firebase-1',
        fullName: ar ? 'علي محمود' : 'Ali Mahmoud',
        displayName: ar ? 'علي' : 'Ali',
        bio: ar ? 'طالب علم' : 'Seeker of knowledge',
        country: 'EG',
        preferredLanguage: _language,
        avatarUrl: null,
        phone: null,
        createdAt: DateTime.utc(2026, 1, 1),
      ),
    );
  }

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async =>
      true;
}

class _CaptureCircleApi extends CircleApiClient {
  _CaptureCircleApi() : super(Dio());

  List<CircleSummary> myCircles = const [];
  List<CircleSummary> publicCircles = const [];

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async =>
      myCircles;

  @override
  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) async =>
      CircleDiscoveryPage(circles: publicCircles);
}

class _CaptureCircleSessions extends StateNotifier<CircleSessionsState>
    implements CircleSessionsController {
  _CaptureCircleSessions()
      : super(const CircleSessionsState(status: CircleSessionsStatus.ready));

  @override
  String get circleId => _circleId;

  @override
  Future<void> load() async {}

  @override
  Future<SessionModel?> create() async => null;
}

/// Queue fixture shared by the student and manager room captures: one
/// reciting entry (the captured student's own turn), one waiting, one
/// skipped — mirrors the T044 dialog fixture.
class _CaptureQueueApi extends QueueApiClient {
  _CaptureQueueApi() : super(Dio());

  @override
  Future<QueueState> getQueue({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      QueueState.fromJson({
        'session_id': liveSessionId,
        'round_id': 'round-1',
        'round_number': 1,
        'round_type': 'revision',
        'lifecycle': 'active',
        'surah_id': 2,
        'from_ayah': 1,
        'to_ayah': 5,
        'grading_required': false,
        'selected_entry_id': 'reciting-entry',
        'version': 1,
        'policy': const {
          'population': 'present_at_activation',
          'unfinished_finalization': 'mark_unfinished_skipped',
          'opt_out': 'approval_required',
          'grade_visibility': 'managers_and_student',
          'grade_correction': 'audited_any_time',
          'version': 1,
        },
        'preorder': const [],
        'entries': const [
          {
            'id': 'reciting-entry',
            'student_id': 'student-1',
            'student_name': 'علي',
            'position': 1,
            'status': 'reciting',
            'version': 1,
          },
          {
            'id': 'waiting-entry',
            'student_id': 'student-2',
            'student_name': 'فاطمة',
            'position': 2,
            'status': 'waiting',
            'version': 1,
          },
          {
            'id': 'skipped-entry',
            'student_id': 'student-3',
            'student_name': 'عمر',
            'position': 3,
            'status': 'skipped',
            'version': 1,
          },
        ],
      });
}

class _CaptureSessionApi extends SessionApiClient {
  _CaptureSessionApi({required this.isModerator}) : super(Dio());

  final bool isModerator;

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      SessionConnection(
        session: const SessionModel(
          id: _roomId,
          circleId: _circleId,
          status: 'active',
          mediaMode: 'audio',
          participantCount: 3,
          isLocked: false,
        ),
        mediaConnection: MediaConnection(
          endpoint: 'wss://media.example',
          credential: 'capture-credential',
          expiresAt: DateTime.utc(2026, 9, 1),
        ),
        isModerator: isModerator,
      );

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      const [];
}

class _CaptureMediaSession implements MediaSession {
  @override
  Future<void> connect(MediaConnection connection) async {}

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class _CaptureSessionRealtime implements RealtimeSessionClient {
  final StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>.broadcast();

  @override
  Future<void> dispose() => _events.close();

  @override
  Future<void> lowerHand(String liveSessionId) async {}

  @override
  Future<void> raiseHand(String liveSessionId) async {}

  @override
  Stream<RealtimeSessionEvent> sessionEvents(
    String liveSessionId, {
    required String token,
    required String backendSessionId,
  }) =>
      _events.stream;
}

/// Canned history page for the group conversation capture.
class _CaptureChatApi extends ChatApiClient {
  _CaptureChatApi() : super(Dio());

  ChatMessagePage? page;

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async =>
      page ?? const ChatMessagePage(messages: [], hasMore: false);
}

class _CaptureChatRealtime
    implements ChatRealtimeClient, ChatRealtimePresenceClient {
  final StreamController<ChatRealtimeEvent> _events =
      StreamController<ChatRealtimeEvent>.broadcast(sync: true);

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(
    String circleId, {
    required String token,
    required String backendSessionId,
  }) =>
      _events.stream;

  @override
  Stream<ChatRealtimeEvent> directChatEvents(
    String peerId, {
    required String token,
    required String backendSessionId,
  }) =>
      const Stream.empty();

  @override
  Future<void> sendTyping({
    String? circleId,
    String? dmPeerId,
    required bool isTyping,
  }) async {}

  @override
  Future<void> dispose() => _events.close();
}

// ---------------------------------------------------------------------------
// Composer platform seams (record / just_audio / native picker stay out of
// widget tests)
// ---------------------------------------------------------------------------

ChatUploadResult _chatUploadResult() => ChatUploadResult(
      url: 'https://media.example.com/chat/voice/capture.m4a?sig=1',
      uploadId: 'upload-1',
      urlExpiresAt: DateTime.parse('2026-09-18T12:00:00Z'),
    );

class _CaptureRecorder implements VoiceRecorder {
  @override
  Future<bool> hasPermission() async => false;

  @override
  Future<void> start(String filePath) async {}

  @override
  Future<String?> stop() async => null;

  @override
  Stream<double> amplitudeStream() => const Stream.empty();

  @override
  Future<void> dispose() async {}
}

class _CapturePlayer implements PreviewPlayer {
  @override
  Future<void> playToCompletion(String filePath) async {}

  @override
  Future<void> playUrlToCompletion(String url) async {}

  @override
  Stream<Duration> get positionStream => const Stream.empty();

  @override
  Future<void> dispose() async {}
}

class _CapturePicker implements ChatAttachmentPicker {
  @override
  Future<String?> pickImage() async => null;

  @override
  Future<String?> pickPdf() async => null;
}
