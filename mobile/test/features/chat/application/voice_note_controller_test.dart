import 'dart:async';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/voice_note_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';

ChatUploadResult _uploadResult() => ChatUploadResult(
      objectKey: 'chat/voice/abc.m4a',
      url: 'https://media.example.com/chat/voice/abc.m4a?sig=1',
      uploadId: '55555555-5555-5555-5555-555555555555',
      urlExpiresAt: DateTime.parse('2026-09-15T12:00:00Z'),
    );

// testWidgets gives the fake clock (tester.pump) needed by the 1-second
// recording timer while letting pure-microtask futures be awaited directly.
void main() {
  testWidgets('granted permission moves requestingPermission → recording',
      (tester) async {
    final harness = _Harness();
    final started = harness.controller.startRecording();
    expect(harness.controller.state.phase,
        VoiceNotePhase.requestingPermission);
    harness.recorder.permission.complete(true);
    await started;
    await tester.pump();

    expect(harness.controller.state.phase, VoiceNotePhase.recording);
    expect(harness.recorder.startedPath, isNotNull);
    harness.controller.dispose();
  });

  testWidgets('denied permission surfaces denial with settings hint',
      (tester) async {
    final harness = _Harness();
    harness.recorder.permission.complete(false);
    await harness.controller.startRecording();

    final state = harness.controller.state;
    expect(state.phase, VoiceNotePhase.permissionDenied);
    expect(state.showPermissionSettingsHint, isTrue);
    expect(harness.recorder.startCalls, 0);

    // The composer stays usable: a retry re-requests permission.
    harness.recorder.permission = Completer<bool>()..complete(true);
    await harness.controller.startRecording();
    expect(harness.controller.state.phase, VoiceNotePhase.recording);
    harness.controller.dispose();
  });

  testWidgets('amplitude events accumulate as a bounded waveform sample list',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();

    for (var i = 0; i < 130; i++) {
      harness.recorder.amplitudes.add(-12.5 + i * 0.1);
      await tester.pump();
    }

    expect(harness.controller.state.amplitudes, hasLength(100));
    // The newest samples are retained, not the first ones.
    expect(harness.controller.state.amplitudes.last,
        closeTo(-12.5 + 129 * 0.1, 0.0001));
    harness.controller.dispose();
  });

  testWidgets('manual stop freezes duration and keeps a previewable recording',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();
    await tester.pump(const Duration(seconds: 3));

    await harness.controller.stopRecording();

    expect(harness.controller.state.phase, VoiceNotePhase.recorded);
    expect(harness.controller.state.duration, const Duration(seconds: 3));
    harness.controller.dispose();
  });

  testWidgets('recording auto-stops at the 300-second contract limit',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();

    await tester.pump(const Duration(seconds: 300));
    await tester.pump();

    expect(harness.controller.state.phase, VoiceNotePhase.recorded);
    expect(harness.controller.state.duration, const Duration(seconds: 300));
    expect(harness.recorder.stopCalls, 1);
    harness.controller.dispose();
  });

  testWidgets('send hands file and duration to the upload and resets on success',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();
    await tester.pump(const Duration(seconds: 7));
    await harness.controller.stopRecording();
    final recordedPath = harness.recorder.startedPath!;
    File(recordedPath).writeAsBytesSync(List<int>.filled(8, 1));

    final result = await harness.controller.send();

    expect(result?.uploadId, '55555555-5555-5555-5555-555555555555');
    expect(harness.uploadCalls, hasLength(1));
    expect(harness.uploadCalls.single.$1, recordedPath);
    expect(harness.uploadCalls.single.$2, 7);
    expect(harness.controller.state.phase, VoiceNotePhase.idle);
    // The submitted local file is cleaned up after durable staging.
    expect(File(recordedPath).existsSync(), isFalse);
    harness.controller.dispose();
  });

  testWidgets('network upload failure is retryable and keeps the recording',
      (tester) async {
    final harness = _Harness()
      ..grantPermission()
      ..uploadError = const ChatApiException(
        statusCode: null,
        code: ChatApiErrors.requestFailed,
        message: 'offline',
      );
    await harness.controller.startRecording();
    await harness.controller.stopRecording();
    final recordedPath = harness.recorder.startedPath!;
    File(recordedPath).writeAsBytesSync(List<int>.filled(8, 1));

    expect(await harness.controller.send(), isNull);

    final state = harness.controller.state;
    expect(state.phase, VoiceNotePhase.failed);
    expect(state.failure, VoiceNoteFailure.send);
    expect(state.failureIsRetryable, isTrue);
    expect(File(recordedPath).existsSync(), isTrue);

    // Explicit retry with recovered connectivity succeeds.
    harness.uploadError = null;
    expect(await harness.controller.send(), isNotNull);
    expect(harness.controller.state.phase, VoiceNotePhase.idle);
    harness.controller.dispose();
  });

  testWidgets('a 413 upload rejection is terminal, not retryable',
      (tester) async {
    final harness = _Harness()
      ..grantPermission()
      ..uploadError = const ChatApiException(
        statusCode: 413,
        code: 'ERR_UPLOAD_TOO_LARGE',
        message: 'Attachment exceeds the size limit.',
      );
    await harness.controller.startRecording();
    await harness.controller.stopRecording();
    File(harness.recorder.startedPath!)
        .writeAsBytesSync(List<int>.filled(8, 1));

    expect(await harness.controller.send(), isNull);

    final state = harness.controller.state;
    expect(state.phase, VoiceNotePhase.failed);
    expect(state.failure, VoiceNoteFailure.send);
    expect(state.failureIsRetryable, isFalse);
    harness.controller.dispose();
  });

  testWidgets('a client-side limit rejection is terminal, not retryable',
      (tester) async {
    final harness = _Harness()
      ..grantPermission()
      ..uploadError =
          const ChatMediaLimitException(ChatMediaLimitKind.voiceTooLong);
    await harness.controller.startRecording();
    await harness.controller.stopRecording();
    File(harness.recorder.startedPath!)
        .writeAsBytesSync(List<int>.filled(8, 1));

    expect(await harness.controller.send(), isNull);

    final state = harness.controller.state;
    expect(state.phase, VoiceNotePhase.failed);
    expect(state.failureIsRetryable, isFalse);
    harness.controller.dispose();
  });

  testWidgets('discard from recorded resets to idle and deletes the local file',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();
    await harness.controller.stopRecording();
    final recordedPath = harness.recorder.startedPath!;
    File(recordedPath).writeAsBytesSync(List<int>.filled(8, 1));

    await harness.controller.discard();

    expect(harness.controller.state.phase, VoiceNotePhase.idle);
    expect(File(recordedPath).existsSync(), isFalse);
    harness.controller.dispose();
  });

  testWidgets(
      'preview tracks playback position and returns to recorded on completion',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();
    await harness.controller.stopRecording();

    unawaited(harness.controller.startPreview());
    await tester.pump();
    expect(harness.controller.state.phase, VoiceNotePhase.previewing);
    expect(harness.player.playedPath, harness.recorder.startedPath);

    harness.player.positions.add(const Duration(seconds: 2));
    await tester.pump();
    expect(harness.controller.state.previewPosition,
        const Duration(seconds: 2));

    harness.player.playback.complete();
    await tester.pump();
    expect(harness.controller.state.phase, VoiceNotePhase.recorded);
    harness.controller.dispose();
  });

  testWidgets('preview failure keeps the recording and exposes a retry',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();
    await harness.controller.stopRecording();
    final recordedPath = harness.recorder.startedPath!;
    File(recordedPath).writeAsBytesSync(List<int>.filled(8, 1));
    harness.player.playError = StateError('decoder failed');

    await harness.controller.startPreview();

    final state = harness.controller.state;
    expect(state.phase, VoiceNotePhase.recorded);
    expect(state.failure, VoiceNoteFailure.preview);
    expect(state.failureIsRetryable, isTrue);
    expect(File(recordedPath).existsSync(), isTrue);

    // Retry after the failure works.
    harness.player.playError = null;
    unawaited(harness.controller.startPreview());
    await tester.pump();
    expect(harness.controller.state.phase, VoiceNotePhase.previewing);
    harness.controller.dispose();
  });

  testWidgets('stop failure with a valid partial file keeps it previewable',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();
    // The plugin already wrote bytes before the interruption hit.
    final partialPath = harness.recorder.startedPath!;
    File(partialPath).writeAsBytesSync(List<int>.filled(64, 1));
    harness.recorder.stopError = StateError('recorder interrupted');

    await harness.controller.stopRecording();

    expect(harness.controller.state.phase, VoiceNotePhase.recorded);
    expect(harness.controller.state.failure, VoiceNoteFailure.recorder);
    expect(File(partialPath).existsSync(), isTrue);

    // The partial recording is still previewable.
    unawaited(harness.controller.startPreview());
    await tester.pump();
    expect(harness.controller.state.phase, VoiceNotePhase.previewing);
    harness.controller.dispose();
  });

  testWidgets('stop failure without a usable file offers discard from failed',
      (tester) async {
    final harness = _Harness()..grantPermission();
    await harness.controller.startRecording();
    harness.recorder.stopError = StateError('recorder interrupted');
    // No file was ever written.

    await harness.controller.stopRecording();

    final state = harness.controller.state;
    expect(state.phase, VoiceNotePhase.failed);
    expect(state.failure, VoiceNoteFailure.recorder);

    await harness.controller.discard();
    expect(harness.controller.state.phase, VoiceNotePhase.idle);
    harness.controller.dispose();
  });

  testWidgets('send outside a recorded state is a no-op returning null',
      (tester) async {
    final harness = _Harness()..grantPermission();

    expect(await harness.controller.send(), isNull);
    expect(harness.uploadCalls, isEmpty);
    expect(harness.controller.state.phase, VoiceNotePhase.idle);
    harness.controller.dispose();
  });
}

/// Shared wiring: real controller over pure-Dart fake seams.
class _Harness {
  _Harness() {
    controller = VoiceNoteController(
      upload: (filePath, durationSeconds) async {
        uploadCalls.add((filePath, durationSeconds));
        final error = uploadError;
        if (error != null) throw error;
        return _uploadResult();
      },
      recorder: recorder,
      player: player,
    );
  }

  late final VoiceNoteController controller;
  final recorder = _FakeRecorder();
  final player = _FakePlayer();
  final uploadCalls = <(String, int)>[];
  Object? uploadError;

  void grantPermission() {
    recorder.permission.complete(true);
  }
}

class _FakeRecorder implements VoiceRecorder {
  Completer<bool> permission = Completer<bool>();
  int startCalls = 0;
  String? startedPath;
  int stopCalls = 0;
  Object? stopError;
  final amplitudes = StreamController<double>.broadcast();
  int disposeCalls = 0;

  @override
  Future<bool> hasPermission() async => permission.future;

  @override
  Future<void> start(String filePath) async {
    startCalls++;
    startedPath = filePath;
  }

  @override
  Future<String?> stop() async {
    stopCalls++;
    if (stopError != null) throw stopError!;
    return startedPath;
  }

  @override
  Stream<double> amplitudeStream() => amplitudes.stream;

  @override
  Future<void> dispose() async {
    disposeCalls++;
  }
}

class _FakePlayer implements PreviewPlayer {
  Object? playError;
  String? playedPath;
  final playback = Completer<void>();
  final positions = StreamController<Duration>.broadcast();
  int disposeCalls = 0;

  @override
  Future<void> playToCompletion(String filePath) async {
    playedPath = filePath;
    if (playError != null) throw playError!;
    await playback.future;
  }

  @override
  Future<void> playUrlToCompletion(String url) async {
    throw UnimplementedError();
  }

  @override
  Stream<Duration> get positionStream => positions.stream;

  @override
  Future<void> dispose() async {
    disposeCalls++;
  }
}
