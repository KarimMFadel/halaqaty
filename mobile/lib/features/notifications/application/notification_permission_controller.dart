import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../../auth/application/auth_controller.dart';

const _promptShownKey = 'halaqaty_notif_prompt_shown';

/// FCM permission state for the notification onboarding path (FR-017).
class NotificationPermissionState {
  const NotificationPermissionState({
    this.status,
    this.gentlePromptVisible = false,
  });

  /// Evaluated [AuthorizationStatus]; `null` while the state is unavailable
  /// (the app stays fully functional and re-evaluates later).
  final AuthorizationStatus? status;

  /// Whether the single gentle denied-permission hint should be displayed.
  final bool gentlePromptVisible;

  NotificationPermissionState copyWith({
    AuthorizationStatus? status,
    bool? gentlePromptVisible,
  }) {
    return NotificationPermissionState(
      status: status ?? this.status,
      gentlePromptVisible: gentlePromptVisible ?? this.gentlePromptVisible,
    );
  }
}

/// F-008 T026: owns the FCM permission lifecycle.
///
/// - The platform permission is requested exactly once at the first
///   authenticated push-delivery moment (token registration for that user
///   follows in the auth feature; never duplicated here).
/// - When permission is denied the app stays fully functional; after the
///   first session ends a single non-blocking gentle prompt appears
///   (FR-017 / JOURNEY.md). Permanent denial is respected: only the prompt's
///   accept action may retry, and the prompt is shown once per install.
class NotificationPermissionController
    extends StateNotifier<NotificationPermissionState> {
  NotificationPermissionController({
    required Future<NotificationSettings> Function() loadSettings,
    required Future<NotificationSettings> Function() requestPermission,
    required FlutterSecureStorage secureStorage,
  })  : _loadSettings = loadSettings,
        _requestPermission = requestPermission,
        _secureStorage = secureStorage,
        super(const NotificationPermissionState());

  final Future<NotificationSettings> Function() _loadSettings;
  final Future<NotificationSettings> Function() _requestPermission;
  final FlutterSecureStorage _secureStorage;

  bool _resolving = false;

  /// Reads the platform permission, requesting it when still undetermined.
  Future<void> ensurePermissionRequested() async {
    final status = state.status;
    if (_resolving ||
        status == AuthorizationStatus.authorized ||
        status == AuthorizationStatus.provisional) {
      return;
    }
    _resolving = true;
    try {
      var settings = await _loadSettings();
      if (settings.authorizationStatus == AuthorizationStatus.notDetermined) {
        settings = await _requestPermission();
      }
      state = state.copyWith(status: settings.authorizationStatus);
    } on FirebaseException {
      // Permission state is temporarily unavailable; the app remains fully
      // functional and the next evaluation retries (FR-017, JOURNEY.md).
    } finally {
      _resolving = false;
    }
  }

  /// Fired by the session room when a session reaches its ended state (both
  /// teacher-end and the metadata-only `session.ended` event). Shows the
  /// gentle prompt for denied permission only, at most once per install.
  Future<void> onFirstSessionEnded() async {
    if (state.gentlePromptVisible) return;
    if (state.status == null) {
      await ensurePermissionRequested();
    }
    if (state.status != AuthorizationStatus.denied) return;
    if (await _secureStorage.read(key: _promptShownKey) != null) return;
    state = state.copyWith(gentlePromptVisible: true);
  }

  /// Prompt closure. `retry: true` is the accept action: re-asks the
  /// platform once and records the result; dismissal only marks the prompt
  /// shown so it never reappears (not a nag).
  Future<void> acknowledgePrompt({required bool retry}) async {
    state = state.copyWith(gentlePromptVisible: false);
    if (retry) {
      try {
        final settings = await _requestPermission();
        state = state.copyWith(status: settings.authorizationStatus);
      } on FirebaseException {
        // The app stays fully functional without re-asking (FR-017).
      }
    }
    await _secureStorage.write(key: _promptShownKey, value: '1');
  }
}

final notificationPermissionControllerProvider = StateNotifierProvider<
    NotificationPermissionController, NotificationPermissionState>((ref) {
  return NotificationPermissionController(
    // FirebaseMessaging is resolved lazily so constructing this provider in
    // widget tests stays channel-free.
    loadSettings: () =>
        ref.read(firebaseMessagingProvider).getNotificationSettings(),
    requestPermission: () =>
        ref.read(firebaseMessagingProvider).requestPermission(
              alert: true,
              badge: true,
              sound: true,
            ),
    secureStorage: ref.watch(flutterSecureStorageProvider),
  );
});
