import 'dart:async';
import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:just_audio/just_audio.dart';
import 'package:record/record.dart';

import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';

/// Lifecycle of one voice-note interaction. [recorded] means a finished
/// (possibly interrupted-but-usable) local file awaits preview/discard/send.
enum VoiceNotePhase {
  idle,
  requestingPermission,
  permissionDenied,
  recording,
  recorded,
  previewing,
  sending,
  failed,
}

/// Semantic failure kinds; the widget layer (T050/T051) maps each to
/// localized copy, so no user-facing strings live here.
enum VoiceNoteFailure { send, preview, recorder }

/// Semantic state for the voice-note composer surface (FR-019). [duration]
/// is the recording elapsed while recording and the final length once
/// recorded; [amplitudes] is the newest tail of bounded waveform samples.
class VoiceNoteState {
  const VoiceNoteState({
    this.phase = VoiceNotePhase.idle,
    this.duration = Duration.zero,
    this.amplitudes = const <double>[],
    this.showPermissionSettingsHint = false,
    this.previewPosition = Duration.zero,
    this.failure,
    this.failureIsRetryable = false,
  });

  final VoiceNotePhase phase;
  final Duration duration;
  final List<double> amplitudes;
  final bool showPermissionSettingsHint;
  final Duration previewPosition;
  final VoiceNoteFailure? failure;
  final bool failureIsRetryable;

  VoiceNoteState copyWith({
    VoiceNotePhase? phase,
    Duration? duration,
    List<double>? amplitudes,
    bool? showPermissionSettingsHint,
    Duration? previewPosition,
    VoiceNoteFailure? failure,
    bool clearFailure = false,
    bool? failureIsRetryable,
  }) =>
      VoiceNoteState(
        phase: phase ?? this.phase,
        duration: duration ?? this.duration,
        amplitudes: amplitudes ?? this.amplitudes,
        showPermissionSettingsHint:
            showPermissionSettingsHint ?? this.showPermissionSettingsHint,
        previewPosition: previewPosition ?? this.previewPosition,
        failure: clearFailure ? null : (failure ?? this.failure),
        failureIsRetryable: clearFailure
            ? false
            : (failureIsRetryable ?? this.failureIsRetryable),
      );
}

/// `record`-plugin seam: the real plugin is platform-channel based and cannot
/// run in unit tests, so the controller depends on this pure interface and
/// production wiring injects [RecordVoiceRecorder].
abstract class VoiceRecorder {
  /// Checks (and requests when undetermined) microphone permission.
  Future<bool> hasPermission();

  /// Starts recording to [filePath].
  Future<void> start(String filePath);

  /// Stops recording and returns the output path if any.
  Future<String?> stop();

  /// Emits dBFS amplitude samples while recording.
  Stream<double> amplitudeStream();

  Future<void> dispose();
}

/// [VoiceRecorder] over the real `record` plugin.
class RecordVoiceRecorder implements VoiceRecorder {
  RecordVoiceRecorder() : _recorder = AudioRecorder();

  final AudioRecorder _recorder;

  @override
  Future<bool> hasPermission() => _recorder.hasPermission();

  @override
  Future<void> start(String filePath) => _recorder.start(
        // ponytail: default aacLc/MP4 config is codec-neutral and inside the
        // contract's supported containers (OGG, MPEG, MP4, WebM); revisit only
        // if server validation rejects a platform default.
        const RecordConfig(),
        path: filePath,
      );

  @override
  Future<String?> stop() => _recorder.stop();

  @override
  Stream<double> amplitudeStream() => _recorder
      .onAmplitudeChanged(const Duration(milliseconds: 200))
      .map((amplitude) => amplitude.current);

  @override
  Future<void> dispose() => _recorder.dispose();
}

/// `just_audio` seam for foreground preview playback; the controller depends
/// on this pure interface and production wiring injects
/// [JustAudioPreviewPlayer].
abstract class PreviewPlayer {
  /// Loads [filePath] and plays it to natural completion; throws on load or
  /// playback failure.
  Future<void> playToCompletion(String filePath);

  /// Plays a (renewed, presigned) media [url] to natural completion; throws
  /// on load or playback failure. Used for received voice messages.
  Future<void> playUrlToCompletion(String url);

  /// Playback position while playing.
  Stream<Duration> get positionStream;

  Future<void> dispose();
}

/// [PreviewPlayer] over the real `just_audio` plugin. The [AudioPlayer] is
/// created lazily so a disposed player can transparently play again.
class JustAudioPreviewPlayer implements PreviewPlayer {
  AudioPlayer? _player;

  @override
  Future<void> playToCompletion(String filePath) async {
    final player = _player ??= AudioPlayer();
    await player.setFilePath(filePath);
    await player.play();
  }

  @override
  Future<void> playUrlToCompletion(String url) async {
    final player = _player ??= AudioPlayer();
    await player.setUrl(url);
    await player.play();
  }

  @override
  Stream<Duration> get positionStream =>
      (_player ??= AudioPlayer()).positionStream;

  @override
  Future<void> dispose() async {
    await _player?.dispose();
    _player = null;
  }
}

/// Upload hand-off injected by the provider: keeps the controller free of
/// Dio/credentials and unit tests pure.
typedef VoiceNoteUpload = Future<ChatUploadResult> Function(
    String filePath, int durationSeconds);

/// Owns the record → preview → send voice-note state machine (FR-019,
/// FR-020): permission denial keeps the composer usable, recording
/// interruption preserves a previewable partial file when valid, preview
/// failures expose retry without touching the recording, and sending hands
/// the file plus duration to the injected upload callback.
class VoiceNoteController extends StateNotifier<VoiceNoteState> {
  VoiceNoteController({
    required VoiceNoteUpload upload,
    required VoiceRecorder recorder,
    required PreviewPlayer player,
  })  : _upload = upload,
        _recorder = recorder,
        _player = player,
        super(const VoiceNoteState());

  final VoiceNoteUpload _upload;
  final VoiceRecorder _recorder;
  final PreviewPlayer _player;

  // ponytail: keep only the newest 100 waveform samples; a longer window can
  // switch to fixed-bucket downsampling if the UI ever needs full history.
  static const int _amplitudeSampleCap = 100;

  Timer? _tickTimer;
  StreamSubscription<double>? _amplitudeSubscription;
  StreamSubscription<Duration>? _positionSubscription;
  int _ticks = 0;
  String? _recordedPath;

  /// Requests microphone permission and starts recording. A denial is
  /// surfaced as [VoiceNotePhase.permissionDenied] (with a settings hint),
  /// never thrown, so the text composer stays usable.
  Future<void> startRecording() async {
    final blockedPhases = const [
      VoiceNotePhase.requestingPermission,
      VoiceNotePhase.recording,
      VoiceNotePhase.recorded,
      VoiceNotePhase.previewing,
      VoiceNotePhase.sending,
    ];
    if (blockedPhases.contains(state.phase)) return;

    state = state.copyWith(
      phase: VoiceNotePhase.requestingPermission,
      showPermissionSettingsHint: false,
      clearFailure: true,
    );

    final bool granted;
    try {
      granted = await _recorder.hasPermission();
    } catch (_) {
      // Permission plumbing failure is recoverable by retrying.
      state = state.copyWith(
        phase: VoiceNotePhase.failed,
        failure: VoiceNoteFailure.recorder,
        failureIsRetryable: true,
      );
      return;
    }
    if (!mounted) return;
    if (!granted) {
      state = state.copyWith(
        phase: VoiceNotePhase.permissionDenied,
        showPermissionSettingsHint: true,
      );
      return;
    }

    try {
      _recordedPath = _newRecordingPath();
      _ticks = 0;
      await _recorder.start(_recordedPath!);
    } catch (_) {
      state = state.copyWith(
        phase: VoiceNotePhase.failed,
        failure: VoiceNoteFailure.recorder,
        failureIsRetryable: true,
      );
      return;
    }
    if (!mounted) return;

    state = state.copyWith(
      phase: VoiceNotePhase.recording,
      duration: Duration.zero,
      amplitudes: const <double>[],
      clearFailure: true,
    );
    _amplitudeSubscription = _recorder.amplitudeStream().listen((sample) {
      final samples = [...state.amplitudes, sample];
      if (samples.length > _amplitudeSampleCap) {
        samples.removeRange(0, samples.length - _amplitudeSampleCap);
      }
      state = state.copyWith(amplitudes: samples);
    });
    _tickTimer = Timer.periodic(
      const Duration(seconds: 1),
      (_) => _onTick(),
    );
  }

  /// Stops recording and freezes the previewable result.
  Future<void> stopRecording() async {
    if (state.phase != VoiceNotePhase.recording) return;
    _teardownRecordingClock();
    try {
      final path = await _recorder.stop();
      if (!mounted) return;
      _recordedPath = path ?? _recordedPath;
      state = state.copyWith(
        phase: VoiceNotePhase.recorded,
        duration: Duration(seconds: _ticks),
        clearFailure: true,
      );
    } catch (_) {
      // Interruption: keep the partial file previewable when it is usable;
      // otherwise land in failed so discard is the offered action.
      if (!mounted) return;
      final path = _recordedPath;
      final usable = path != null &&
          File(path).existsSync() &&
          File(path).lengthSync() > 0;
      state = usable
          ? state.copyWith(
              phase: VoiceNotePhase.recorded,
              duration: Duration(seconds: _ticks),
              failure: VoiceNoteFailure.recorder,
            )
          : state.copyWith(
              phase: VoiceNotePhase.failed,
              duration: Duration(seconds: _ticks),
              failure: VoiceNoteFailure.recorder,
            );
    }
  }

  /// Stops an active recording when the app loses foreground focus.
  Future<void> interrupt() => stopRecording();

  /// Plays the recorded note once; on completion or failure the state returns
  /// to [VoiceNotePhase.recorded] so preview/discard/send remain available.
  Future<void> startPreview() async {
    final path = _recordedPath;
    if (path == null || state.phase != VoiceNotePhase.recorded) return;
    state = state.copyWith(
      phase: VoiceNotePhase.previewing,
      previewPosition: Duration.zero,
      clearFailure: true,
    );
    _positionSubscription = _player.positionStream.listen((position) {
      if (state.phase == VoiceNotePhase.previewing) {
        state = state.copyWith(previewPosition: position);
      }
    });

    var playbackFailed = false;
    try {
      await _player.playToCompletion(path);
    } catch (_) {
      // Playback failure must not change the recording (spec edge case);
      // the file stays recorded and a retry is exposed below.
      playbackFailed = true;
    }
    // ponytail: cancel is fire-and-forget; a broadcast cancel future must
    // never gate this state transition (and hangs under fake test clocks).
    unawaited(_positionSubscription?.cancel());
    _positionSubscription = null;
    if (!mounted || state.phase != VoiceNotePhase.previewing) return;
    state = playbackFailed
        ? state.copyWith(
            phase: VoiceNotePhase.recorded,
            failure: VoiceNoteFailure.preview,
            failureIsRetryable: true,
          )
        : state.copyWith(phase: VoiceNotePhase.recorded);
  }

  /// Sends the recorded file through the injected upload. Returns the staged
  /// [ChatUploadResult], or null when the current state cannot send or the
  /// upload failed (failure details land in the state).
  Future<ChatUploadResult?> send() async {
    final path = _recordedPath;
    final sendable = state.phase == VoiceNotePhase.recorded ||
        (state.phase == VoiceNotePhase.failed &&
            state.failure == VoiceNoteFailure.send);
    if (path == null || !sendable) return null;

    state = state.copyWith(
      phase: VoiceNotePhase.sending,
      clearFailure: true,
    );
    try {
      final result = await _upload(path, _ticks);
      if (!mounted) return null;
      _recordedPath = null;
      _ticks = 0;
      state = const VoiceNoteState();
      _deleteLocalFile(path);
      return result;
    } catch (error) {
      if (!mounted) return null;
      state = state.copyWith(
        phase: VoiceNotePhase.failed,
        failure: VoiceNoteFailure.send,
        failureIsRetryable: _isRetryable(error),
      );
      return null;
    }
  }

  /// Resets the interaction and discards the local recording file.
  Future<void> discard() async {
    final path = _recordedPath;
    _teardownRecordingClock();
    unawaited(_positionSubscription?.cancel());
    _positionSubscription = null;
    unawaited(_player.dispose());
    _recordedPath = null;
    _ticks = 0;
    state = const VoiceNoteState();
    if (path != null) _deleteLocalFile(path);
  }

  @override
  void dispose() {
    if (!mounted) return;
    _teardownRecordingClock();
    unawaited(_positionSubscription?.cancel());
    unawaited(_recorder.dispose());
    unawaited(_player.dispose());
    super.dispose();
  }

  void _onTick() {
    _ticks++;
    if (_ticks >= ChatLimits.maxVoiceDurationSeconds) {
      // Auto-stop at the 300-second contract limit (FR-020).
      unawaited(stopRecording());
      return;
    }
    state = state.copyWith(duration: Duration(seconds: _ticks));
  }

  void _teardownRecordingClock() {
    _tickTimer?.cancel();
    _tickTimer = null;
    unawaited(_amplitudeSubscription?.cancel());
    _amplitudeSubscription = null;
  }

  static bool _isRetryable(Object error) {
    if (error is ChatMediaLimitException) return false;
    if (error is ChatApiException) {
      final status = error.statusCode;
      return status == null || status == 429 || status >= 500;
    }
    return true;
  }

  static void _deleteLocalFile(String path) {
    try {
      final file = File(path);
      if (file.existsSync()) file.deleteSync();
    } catch (_) {
      // Best-effort local cleanup; an orphaned temp file is harmless.
    }
  }

  // ponytail: systemTemp is sufficient for tests and IO platforms; T051 may
  // swap in a path-provider directory if a visible cache location is needed.
  static String _newRecordingPath() =>
      '${Directory.systemTemp.path}/halaqaty_voice_'
      '${DateTime.now().microsecondsSinceEpoch}.m4a';
}

/// autoDispose + family(circleId): one recording surface per circle chat,
/// dropped when the chat screen closes. The upload callback captures the
/// shared credential pattern from `groupChatControllerProvider`.
final voiceNoteControllerProvider = StateNotifierProvider.autoDispose
    .family<VoiceNoteController, VoiceNoteState, String>((ref, circleId) {
  final auth = ref.watch(authControllerProvider);
  return VoiceNoteController(
    recorder: RecordVoiceRecorder(),
    player: JustAudioPreviewPlayer(),
    upload: (filePath, durationSeconds) async {
      final user = ref.read(firebaseAuthProvider).currentUser;
      final sessionId = auth.sessionId;
      final token = await user?.getIdToken();
      if (token == null ||
          token.isEmpty ||
          sessionId == null ||
          sessionId.isEmpty) {
        throw StateError('User not authenticated');
      }
      return ref.read(chatMediaApiClientProvider).uploadVoice(
            token: token,
            sessionId: sessionId,
            filePath: filePath,
            durationSeconds: durationSeconds,
            circleId: circleId,
          );
    },
  );
});
