import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_picker.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';

/// Lifecycle of one image/PDF attachment (voice attaches via
/// [MediaAttachmentController.attachVoice] after its own upload).
enum MediaAttachmentPhase { idle, previewing, uploading, sending, failed }

/// Which native picker produced the held file.
enum MediaAttachmentKind { image, pdf }

/// Semantic failure kinds; the widget layer maps each to localized copy, so
/// no user-facing strings live here.
enum MediaAttachmentError {
  voiceTooLong,
  voiceTooLarge,
  imageTooLarge,
  fileTooLarge,
  unsupportedType,
  invalid,
  rateLimited,
  network,
  attachFailed,
}

class MediaAttachmentState {
  const MediaAttachmentState({
    this.phase = MediaAttachmentPhase.idle,
    this.kind,
    this.filePath,
    this.fileName,
    this.progressPercent = 0,
    this.error,
    this.errorIsRetryable = false,
  });

  final MediaAttachmentPhase phase;
  final MediaAttachmentKind? kind;
  final String? filePath;
  final String? fileName;
  final int progressPercent;
  final MediaAttachmentError? error;
  final bool errorIsRetryable;

  MediaAttachmentState copyWith({
    MediaAttachmentPhase? phase,
    int? progressPercent,
  }) =>
      MediaAttachmentState(
        phase: phase ?? this.phase,
        kind: kind,
        filePath: filePath,
        fileName: fileName,
        progressPercent: progressPercent ?? this.progressPercent,
        error: error,
        errorIsRetryable: errorIsRetryable,
      );
}

/// Upload seam: stages a file and reports Dio send progress.
typedef MediaUpload = Future<ChatUploadResult> Function(
  String filePath,
  void Function(int count, int total) onProgress,
);

/// Attach seam: sends the media message with a stable idempotency key.
typedef MediaAttach = Future<ChatMessage> Function(
  ChatMessageType type,
  String uploadId,
  String idempotencyKey,
);

/// Owns the pick → preview → upload → attach flow for image/PDF attachments
/// and the attach step of the voice flow (FR-020/FR-021). The idempotency
/// key is created on the first attempt of a logical send and kept until it
/// succeeds or is cancelled: the server attaches an upload exactly once and
/// rejects a fresh-key re-attach with `409`, so every retry MUST reuse the
/// original key.
class MediaAttachmentController extends StateNotifier<MediaAttachmentState> {
  MediaAttachmentController({
    required ChatAttachmentPicker picker,
    required MediaUpload uploadImage,
    required MediaUpload uploadFile,
    required MediaAttach attach,
  })  : _picker = picker,
        _uploadImage = uploadImage,
        _uploadFile = uploadFile,
        _attach = attach,
        super(const MediaAttachmentState());

  final ChatAttachmentPicker _picker;
  final MediaUpload _uploadImage;
  final MediaUpload _uploadFile;
  final MediaAttach _attach;

  String? _idempotencyKey;
  ChatUploadResult? _pendingVoiceUpload;
  ChatUploadResult? _pendingMediaUpload;

  /// Opens the native image picker; a cancelled pick keeps the composer
  /// idle, an accepted one enters the preview state without uploading.
  Future<void> pickImage() => _pick(MediaAttachmentKind.image);

  /// Opens the native PDF picker with the same contract as [pickImage].
  Future<void> pickPdf() => _pick(MediaAttachmentKind.pdf);

  Future<void> _pick(MediaAttachmentKind kind) async {
    if (state.phase != MediaAttachmentPhase.idle) return;
    String? path;
    try {
      path = kind == MediaAttachmentKind.image
          ? await _picker.pickImage()
          : await _picker.pickPdf();
    } on PlatformException {
      state = const MediaAttachmentState(
        phase: MediaAttachmentPhase.failed,
        error: MediaAttachmentError.network,
        errorIsRetryable: true,
      );
      return;
    }
    if (!mounted || path == null) return;
    state = MediaAttachmentState(
      phase: MediaAttachmentPhase.previewing,
      kind: kind,
      filePath: path,
      fileName: _basename(path),
    );
  }

  /// Uploads and attaches the previewed file. Returns true on success.
  Future<bool> confirmSend() => _send(
        filePath: state.filePath!,
        kind: state.kind!,
      );

  /// Retries a failed send with the original file and idempotency key.
  Future<bool> retry() {
    final path = state.filePath;
    final kind = state.kind;
    if (state.phase != MediaAttachmentPhase.failed ||
        path == null ||
        kind == null) {
      return Future.value(false);
    }
    return _send(filePath: path, kind: kind);
  }

  Future<bool> _send({
    required String filePath,
    required MediaAttachmentKind kind,
  }) async {
    _idempotencyKey ??= newChatIdempotencyKey();
    state = MediaAttachmentState(
      phase: MediaAttachmentPhase.uploading,
      kind: kind,
      filePath: filePath,
      fileName: _basename(filePath),
    );
    try {
      final upload = _pendingMediaUpload ??= kind == MediaAttachmentKind.image
          ? await _uploadImage(filePath, _onProgress)
          : await _uploadFile(filePath, _onProgress);
      if (!mounted) return false;
      state = state.copyWith(phase: MediaAttachmentPhase.sending);
      await _attach(_attachType(kind), upload.uploadId!, _idempotencyKey!);
      if (!mounted) return false;
      _reset();
      return true;
    } catch (error) {
      if (!mounted) return false;
      final (mediaError, retryable) = _mapError(error, kind);
      state = MediaAttachmentState(
        phase: MediaAttachmentPhase.failed,
        kind: kind,
        filePath: filePath,
        fileName: _basename(filePath),
        error: mediaError,
        errorIsRetryable: retryable,
      );
      return false;
    }
  }

  /// Attaches an already-staged voice upload; the voice controller owns the
  /// record/preview/upload flow, this owns the durable message attach. The
  /// key is always fresh for the voice flow: reusing a failed attachment's
  /// key would make the server replay the wrong message.
  Future<bool> attachVoice(ChatUploadResult upload) =>
      _attachStaged(upload, newChatIdempotencyKey());

  /// Retries a failed voice attach with the SAME key and staged upload.
  Future<bool> retryVoiceAttach() {
    final upload = _pendingVoiceUpload;
    if (upload == null || state.phase != MediaAttachmentPhase.failed) {
      return Future.value(false);
    }
    return _attachStaged(upload, _idempotencyKey!);
  }

  Future<bool> _attachStaged(ChatUploadResult upload, String key) async {
    _pendingVoiceUpload = upload;
    _idempotencyKey = key;
    state = MediaAttachmentState(
      phase: MediaAttachmentPhase.sending,
      kind: null,
    );
    try {
      await _attach(ChatMessageType.voice, upload.uploadId!, key);
      if (!mounted) return false;
      _reset();
      return true;
    } catch (_) {
      if (!mounted) return false;
      // A same-key retry converges on the idempotent attach, so voice
      // attach rejections stay retryable regardless of status.
      state = const MediaAttachmentState(
        phase: MediaAttachmentPhase.failed,
        error: MediaAttachmentError.attachFailed,
        errorIsRetryable: true,
      );
      return false;
    }
  }

  /// Discards the preview/failed attachment, its staged voice upload, and
  /// the unsubmitted picked file (spec: cancellation discards the local
  /// envelope and any unsubmitted local file).
  Future<void> cancel() async {
    if (state.phase == MediaAttachmentPhase.idle) return;
    final pickedPath =
        state.kind == null ? null : state.filePath; // voice files are owned
    _reset();                                                  // by the recorder
    if (pickedPath == null) return;
    try {
      final file = File(pickedPath);
      if (await file.exists()) await file.delete();
    } on FileSystemException {
      // Best-effort local cleanup: an undeletable temp file never blocks
      // returning the composer to idle.
    }
  }

  void _reset() {
    _idempotencyKey = null;
    _pendingVoiceUpload = null;
    _pendingMediaUpload = null;
    state = const MediaAttachmentState();
  }

  void _onProgress(int count, int total) {
    if (!mounted || total <= 0) return;
    state = state.copyWith(
      progressPercent: (count * 100 / total).round().clamp(0, 100),
    );
  }

  static ChatMessageType _attachType(MediaAttachmentKind kind) =>
      kind == MediaAttachmentKind.image
          ? ChatMessageType.image
          : ChatMessageType.file;

  /// Maps transport/limit failures to semantic kinds; limits and hard
  /// validations (413/415/422) are terminal, rate/transient are retryable.
  static (MediaAttachmentError, bool retryable) _mapError(
    Object error,
    MediaAttachmentKind kind,
  ) {
    MediaAttachmentError tooLargeFor() => switch (kind) {
          MediaAttachmentKind.image => MediaAttachmentError.imageTooLarge,
          MediaAttachmentKind.pdf => MediaAttachmentError.fileTooLarge,
        };
    if (error is ChatMediaLimitException) {
      return switch (error.kind) {
        ChatMediaLimitKind.voiceTooLong => (
            MediaAttachmentError.voiceTooLong,
            false
          ),
        ChatMediaLimitKind.voiceTooLarge => (
            MediaAttachmentError.voiceTooLarge,
            false
          ),
        ChatMediaLimitKind.imageTooLarge => (
            MediaAttachmentError.imageTooLarge,
            false
          ),
        ChatMediaLimitKind.fileTooLarge => (
            MediaAttachmentError.fileTooLarge,
            false
          ),
      };
    }
    if (error is ChatApiException) {
      final status = error.statusCode;
      return switch (status) {
        413 => (tooLargeFor(), false),
        415 => (MediaAttachmentError.unsupportedType, false),
        422 => (MediaAttachmentError.invalid, false),
        429 => (MediaAttachmentError.rateLimited, true),
        null => (MediaAttachmentError.network, true),
        _ when status >= 500 => (MediaAttachmentError.network, true),
        _ => (MediaAttachmentError.attachFailed, false),
      };
    }
    return (MediaAttachmentError.network, true);
  }

  static String _basename(String path) =>
      path.split(Platform.pathSeparator).last;
}

/// autoDispose + family(circleId): one attachment surface per circle chat,
/// mirroring `voiceNoteControllerProvider`. Uploads and attaches reuse the
/// shared credential pattern from that provider.
final mediaAttachmentControllerProvider = StateNotifierProvider.autoDispose
    .family<MediaAttachmentController, MediaAttachmentState, String>(
        (ref, circleId) {
  final auth = ref.watch(authControllerProvider);

  Future<({String token, String sessionId})> credentials() async {
    final user = ref.read(firebaseAuthProvider).currentUser;
    final sessionId = auth.sessionId;
    final token = await user?.getIdToken();
    if (token == null || token.isEmpty || sessionId == null) {
      throw StateError('User not authenticated');
    }
    return (token: token, sessionId: sessionId);
  }

  return MediaAttachmentController(
    picker: ref.watch(chatAttachmentPickerProvider),
    uploadImage: (filePath, onProgress) async {
      final c = await credentials();
      return ref.read(chatMediaApiClientProvider).uploadImage(
            token: c.token,
            sessionId: c.sessionId,
            filePath: filePath,
            circleId: circleId,
            onProgress: onProgress,
          );
    },
    uploadFile: (filePath, onProgress) async {
      final c = await credentials();
      return ref.read(chatMediaApiClientProvider).uploadFile(
            token: c.token,
            sessionId: c.sessionId,
            filePath: filePath,
            circleId: circleId,
            onProgress: onProgress,
          );
    },
    attach: (type, uploadId, idempotencyKey) async {
      final c = await credentials();
      return ref.read(chatApiClientProvider).sendMediaMessage(
            token: c.token,
            sessionId: c.sessionId,
            circleId: circleId,
            type: type,
            uploadId: uploadId,
            idempotencyKey: idempotencyKey,
          );
    },
  );
});
