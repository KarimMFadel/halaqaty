import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

/// Which FR-020/FR-021 product limit rejected the upload locally.
enum ChatMediaLimitKind {
  voiceTooLong,
  voiceTooLarge,
  imageTooLarge,
  fileTooLarge
}

/// Client-side pre-upload rejection, thrown before any network work so
/// over-limit media never starts a 60-second upload round trip. The server
/// remains authoritative (`413`) for anything the client could not check.
class ChatMediaLimitException implements Exception {
  const ChatMediaLimitException(this.kind);

  final ChatMediaLimitKind kind;

  @override
  String toString() => 'ChatMediaLimitException: $kind';
}

/// Contract `UploadResponse`: one staged chat media object ready to attach.
/// The storage `object_key` is deliberately not parsed: the client only
/// needs `upload_id` to attach, and object keys never travel further into
/// UI-adjacent state than the API boundary (SR-006 hygiene).
class ChatUploadResult {
  const ChatUploadResult({
    required this.url,
    this.uploadId,
    this.urlExpiresAt,
  });

  factory ChatUploadResult.fromJson(Map<String, dynamic> json) {
    final expiresRaw = json[ChatJsonKeys.urlExpiresAt] as String?;
    return ChatUploadResult(
      url: json[ChatJsonKeys.url] as String,
      uploadId: json[ChatJsonKeys.uploadId] as String?,
      urlExpiresAt: expiresRaw == null ? null : DateTime.parse(expiresRaw),
    );
  }

  final String url;
  final String? uploadId;
  final DateTime? urlExpiresAt;
}

/// Contract `MediaAccess`: a fresh seven-day presigned URL returned by
/// media-link renewal.
class ChatMediaAccess {
  const ChatMediaAccess({required this.url, required this.expiresAt});

  factory ChatMediaAccess.fromJson(Map<String, dynamic> json) =>
      ChatMediaAccess(
        url: json[ChatJsonKeys.url] as String,
        expiresAt: DateTime.parse(json[ChatJsonKeys.expiresAt] as String),
      );

  final String url;
  final DateTime expiresAt;
}

/// Dio client for the F-004 media surface: voice/image/PDF uploads
/// (`/uploads/*`) and media-link renewal. Reuses the shared authenticated
/// `dioProvider`, session headers, and `ChatApiException` mapping; uploads
/// extend the receive timeout to the contract's 60-second budget.
class ChatMediaApiClient {
  ChatMediaApiClient(this._dio);

  final Dio _dio;

  /// Stages a voice note (`/uploads/voice`): at most 300 seconds and 20 MB
  /// (FR-020). Pass exactly one of [circleId] / [dmPeerId].
  Future<ChatUploadResult> uploadVoice({
    required String token,
    required String sessionId,
    required String filePath,
    required int durationSeconds,
    String? circleId,
    String? dmPeerId,
    void Function(int count, int total)? onProgress,
  }) async {
    if (durationSeconds > ChatLimits.maxVoiceDurationSeconds) {
      throw const ChatMediaLimitException(ChatMediaLimitKind.voiceTooLong);
    }
    return _upload(
      token: token,
      sessionId: sessionId,
      path: ChatMediaApiPaths.uploadsVoice,
      filePath: filePath,
      maxBytes: ChatLimits.maxVoiceBytes,
      sizeLimitKind: ChatMediaLimitKind.voiceTooLarge,
      circleId: circleId,
      dmPeerId: dmPeerId,
      durationSeconds: durationSeconds,
      onProgress: onProgress,
    );
  }

  /// Stages a JPEG/PNG image (`/uploads/image`): at most 5 MB (FR-021).
  Future<ChatUploadResult> uploadImage({
    required String token,
    required String sessionId,
    required String filePath,
    String? circleId,
    String? dmPeerId,
    void Function(int count, int total)? onProgress,
  }) =>
      _upload(
        token: token,
        sessionId: sessionId,
        path: ChatMediaApiPaths.uploadsImage,
        filePath: filePath,
        maxBytes: ChatLimits.maxImageBytes,
        sizeLimitKind: ChatMediaLimitKind.imageTooLarge,
        circleId: circleId,
        dmPeerId: dmPeerId,
        onProgress: onProgress,
      );

  /// Stages a PDF attachment (`/uploads/file`): at most 10 MB (FR-021).
  Future<ChatUploadResult> uploadFile({
    required String token,
    required String sessionId,
    required String filePath,
    String? circleId,
    String? dmPeerId,
    void Function(int count, int total)? onProgress,
  }) =>
      _upload(
        token: token,
        sessionId: sessionId,
        path: ChatMediaApiPaths.uploadsFile,
        filePath: filePath,
        maxBytes: ChatLimits.maxFileBytes,
        sizeLimitKind: ChatMediaLimitKind.fileTooLarge,
        circleId: circleId,
        dmPeerId: dmPeerId,
        onProgress: onProgress,
      );

  /// Renews a message's presigned media URL
  /// (`POST /messages/{messageId}/media-url`) after current authorization.
  Future<ChatMediaAccess> renewMediaUrl({
    required String token,
    required String sessionId,
    required String messageId,
  }) async {
    try {
      final response = await _dio.post<Map<String, dynamic>>(
        ChatMediaApiPaths.renewMediaUrl(messageId),
        options: Options(headers: sessionRequestHeaders(token, sessionId)),
      );
      return ChatMediaAccess.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  Future<ChatUploadResult> _upload({
    required String token,
    required String sessionId,
    required String path,
    required String filePath,
    required int maxBytes,
    required ChatMediaLimitKind sizeLimitKind,
    String? circleId,
    String? dmPeerId,
    int? durationSeconds,
    void Function(int count, int total)? onProgress,
  }) async {
    if ((circleId == null) == (dmPeerId == null)) {
      throw const ChatApiException(
        statusCode: 422,
        code: ChatApiErrors.validationFailed,
        message: ChatApiErrors.uploadTargetInvalid,
      );
    }
    if (File(filePath).lengthSync() > maxBytes) {
      throw ChatMediaLimitException(sizeLimitKind);
    }
    try {
      final formData = FormData.fromMap({
        ChatJsonKeys.file: await MultipartFile.fromFile(filePath),
        if (circleId != null) ChatJsonKeys.circleId: circleId,
        if (dmPeerId != null) ChatJsonKeys.dmPeerId: dmPeerId,
        if (durationSeconds != null)
          ChatJsonKeys.durationSeconds: durationSeconds,
      });
      final response = await _dio.post<Map<String, dynamic>>(
        path,
        data: formData,
        options: Options(
          headers: sessionRequestHeaders(token, sessionId),
          receiveTimeout: const Duration(seconds: 60),
        ),
        onSendProgress: onProgress,
      );
      return ChatUploadResult.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }
}

final chatMediaApiClientProvider = Provider<ChatMediaApiClient>(
  (ref) => ChatMediaApiClient(ref.watch(dioProvider)),
);
