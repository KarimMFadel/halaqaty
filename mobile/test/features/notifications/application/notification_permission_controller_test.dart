import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/notifications/application/notification_permission_controller.dart';

class _FakeSecureStorage implements FlutterSecureStorage {
  _FakeSecureStorage();

  final Map<String, String> values = {};

  @override
  Future<String?> read({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) =>
      Future<String?>.value(values[key]);

  @override
  Future<void> write({
    required String key,
    required String? value,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async {
    values[key] = value ?? '';
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _Harness {
  _Harness(this.currentStatus) {
    storage = _FakeSecureStorage();
    controller = NotificationPermissionController(
      loadSettings: _loadSettings,
      requestPermission: _requestPermission,
      secureStorage: storage,
    );
  }

  /// Status served by the settings loader; `null` makes the loader throw
  /// [FirebaseException] instead (permission state unavailable).
  final AuthorizationStatus? currentStatus;

  late final NotificationPermissionController controller;
  late final _FakeSecureStorage storage;

  int loadSettingsCalls = 0;
  int requestPermissionCalls = 0;

  /// Status the platform request resolves to ([AuthorizationStatus.denied]
  /// unless the test overrides it).
  AuthorizationStatus requestResultStatus = AuthorizationStatus.denied;

  Future<NotificationSettings> _loadSettings() {
    loadSettingsCalls++;
    final status = currentStatus;
    if (status == null) {
      return Future<NotificationSettings>.error(
        FirebaseException(plugin: 'firebase_messaging', code: 'unavailable'),
      );
    }
    return Future<NotificationSettings>.value(_settings(status));
  }

  Future<NotificationSettings> _requestPermission() {
    requestPermissionCalls++;
    return Future<NotificationSettings>.value(_settings(requestResultStatus));
  }

  void dispose() => controller.dispose();
}

NotificationSettings _settings(AuthorizationStatus status) =>
    NotificationSettings(
      authorizationStatus: status,
      alert: AppleNotificationSetting.notSupported,
      announcement: AppleNotificationSetting.notSupported,
      badge: AppleNotificationSetting.notSupported,
      carPlay: AppleNotificationSetting.notSupported,
      lockScreen: AppleNotificationSetting.notSupported,
      notificationCenter: AppleNotificationSetting.notSupported,
      showPreviews: AppleShowPreviewSetting.notSupported,
      sound: AppleNotificationSetting.notSupported,
      timeSensitive: AppleNotificationSetting.notSupported,
      criticalAlert: AppleNotificationSetting.notSupported,
      providesAppNotificationSettings: AppleNotificationSetting.notSupported,
    );

void main() {
  test(
    'first authenticated check requests the platform permission when it has '
    'not been determined and records the outcome',
    () async {
      final harness = _Harness(AuthorizationStatus.notDetermined)
        ..requestResultStatus = AuthorizationStatus.authorized;
      addTearDown(harness.dispose);

      await harness.controller.ensurePermissionRequested();

      expect(harness.loadSettingsCalls, 1);
      expect(harness.requestPermissionCalls, 1);
      expect(harness.controller.state.status, AuthorizationStatus.authorized);
    },
  );

  test(
    'never auto-re-requests a resolved denial (permanent-denial respected); '
    'the gentle prompt is the non-blocking path',
    () async {
      final harness = _Harness(AuthorizationStatus.denied);
      addTearDown(harness.dispose);

      await harness.controller.ensurePermissionRequested();
      await harness.controller.onFirstSessionEnded();

      expect(harness.requestPermissionCalls, 0);
      expect(harness.controller.state.status, AuthorizationStatus.denied);
      expect(harness.controller.state.gentlePromptVisible, isTrue);
    },
  );

  test(
    'does not call the platform request when permission is already granted',
    () async {
      final harness = _Harness(AuthorizationStatus.authorized);
      addTearDown(harness.dispose);

      await harness.controller.ensurePermissionRequested();
      await harness.controller.onFirstSessionEnded();

      expect(harness.requestPermissionCalls, 0);
      expect(harness.controller.state.status, AuthorizationStatus.authorized);
      expect(harness.controller.state.gentlePromptVisible, isFalse);
    },
  );

  test(
    'the gentle prompt appears after the first session ends only when '
    'permission is denied, and shows once ever (FR-017 / JOURNEY.md, '
    'not a nag)',
    () async {
      final harness = _Harness(AuthorizationStatus.denied);
      addTearDown(harness.dispose);

      await harness.controller.onFirstSessionEnded();
      expect(harness.controller.state.gentlePromptVisible, isTrue);

      await harness.controller.acknowledgePrompt(retry: false);
      expect(harness.controller.state.gentlePromptVisible, isFalse);

      // A later session end never re-shows the prompt for this install.
      await harness.controller.onFirstSessionEnded();
      expect(harness.controller.state.gentlePromptVisible, isFalse);
      expect(harness.storage.values['halaqaty_notif_prompt_shown'], '1');
    },
  );

  test(
    'a shown marker from a previous run suppresses the gentle prompt entirely',
    () async {
      final harness = _Harness(AuthorizationStatus.denied);
      addTearDown(harness.dispose);
      harness.storage.values['halaqaty_notif_prompt_shown'] = '1';

      await harness.controller.onFirstSessionEnded();

      expect(harness.controller.state.gentlePromptVisible, isFalse);
    },
  );

  test(
    'accepting the prompt retries the request and keeps the app fully '
    'functional when the retry is denied again',
    () async {
      final harness = _Harness(AuthorizationStatus.denied);
      addTearDown(harness.dispose);
      await harness.controller.onFirstSessionEnded();

      await harness.controller.acknowledgePrompt(retry: true);

      expect(harness.requestPermissionCalls, 1);
      expect(harness.controller.state.status, AuthorizationStatus.denied);
      expect(harness.controller.state.gentlePromptVisible, isFalse);
      expect(harness.storage.values['halaqaty_notif_prompt_shown'], '1');
    },
  );

  test(
    'a granted prompt retry moves the state to authorized',
    () async {
      final harness = _Harness(AuthorizationStatus.denied)
        ..requestResultStatus = AuthorizationStatus.authorized;
      addTearDown(harness.dispose);
      await harness.controller.onFirstSessionEnded();

      await harness.controller.acknowledgePrompt(retry: true);

      expect(harness.controller.state.status, AuthorizationStatus.authorized);
      expect(harness.controller.state.gentlePromptVisible, isFalse);
      expect(harness.storage.values['halaqaty_notif_prompt_shown'], '1');
    },
  );

  test(
    'an unavailable permission state keeps the app functional without a '
    'prompt until the next evaluation',
    () async {
      final harness = _Harness(null);
      addTearDown(harness.dispose);

      await harness.controller.ensurePermissionRequested();
      await harness.controller.onFirstSessionEnded();

      expect(harness.controller.state.status, isNull);
      expect(harness.controller.state.gentlePromptVisible, isFalse);
    },
  );

  test(
    'a session end with an unevaluated status resolves the status first and '
    'only prompts on denial',
    () async {
      final harness = _Harness(AuthorizationStatus.denied);
      addTearDown(harness.dispose);

      await harness.controller.onFirstSessionEnded();

      expect(harness.loadSettingsCalls, 1);
      expect(harness.controller.state.gentlePromptVisible, isTrue);
    },
  );
}
