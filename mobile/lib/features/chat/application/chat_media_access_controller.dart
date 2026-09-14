import 'dart:async';
import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/voice_note_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';

Future<String?> downloadChatPdf(String url) async {
  final client = HttpClient();
  try {
    final request = await client.getUrl(Uri.parse(url));
    final response = await request.close();
    if (response.statusCode < 200 || response.statusCode >= 300) {
      await response.drain<void>();
      throw HttpException('Chat media download failed (${response.statusCode})',
          uri: Uri.parse(url));
    }
    final bytes = await response
        .fold<List<int>>(<int>[], (all, chunk) => all..addAll(chunk));
    final file = File(
        '${Directory.systemTemp.path}/halaqaty-chat-${DateTime.now().microsecondsSinceEpoch}.pdf');
    await file.writeAsBytes(bytes, flush: true);
    return file.path;
  } finally {
    client.close(force: true);
  }
}

enum ChatMediaAccessPhase {
  idle,
  loading,
  ready,
  playing,
  downloaded,
  failed,
  denied
}

class ChatMediaAccessState {
  const ChatMediaAccessState(
      {this.phase = ChatMediaAccessPhase.idle,
      this.access,
      this.downloadedPath});
  final ChatMediaAccessPhase phase;
  final ChatMediaAccess? access;

  /// Local path of a completed PDF download so the UI can show where the
  /// file was saved (US3-AC4); null unless [phase] is `downloaded`.
  final String? downloadedPath;
}

/// Renews authorization before opening received media; denied links are removed.
class ChatMediaAccessController extends StateNotifier<ChatMediaAccessState> {
  ChatMediaAccessController(
      {required Future<ChatMediaAccess> Function() renew,
      required PreviewPlayer player,
      required Future<String?> Function(String) download})
      : _renew = renew,
        _player = player,
        _download = download,
        super(const ChatMediaAccessState());
  final Future<ChatMediaAccess> Function() _renew;
  final PreviewPlayer _player;
  final Future<String?> Function(String) _download;

  Future<void> renew() => _open();
  Future<void> play() => _open(play: true);
  Future<void> download() => _open(download: true);

  Future<void> _open({bool play = false, bool download = false}) async {
    if (state.phase == ChatMediaAccessPhase.loading ||
        state.phase == ChatMediaAccessPhase.playing ||
        state.phase == ChatMediaAccessPhase.denied) {
      return;
    }
    state = const ChatMediaAccessState(phase: ChatMediaAccessPhase.loading);
    try {
      final access = await _renew();
      if (!mounted) return;
      state = ChatMediaAccessState(
          phase:
              play ? ChatMediaAccessPhase.playing : ChatMediaAccessPhase.ready,
          access: access);
      if (play) await _player.playUrlToCompletion(access.url);
      final saved = download ? await _download(access.url) : null;
      if (!mounted) return;
      state = ChatMediaAccessState(
          phase: saved != null
              ? ChatMediaAccessPhase.downloaded
              : ChatMediaAccessPhase.ready,
          access: access,
          downloadedPath: saved);
    } catch (error) {
      if (!mounted) return;
      final denied = error is ChatApiException &&
          [401, 403, 404].contains(error.statusCode);
      state = ChatMediaAccessState(
          phase: denied
              ? ChatMediaAccessPhase.denied
              : ChatMediaAccessPhase.failed);
    }
  }

  Future<void> stop() async {
    await _player.dispose();
    if (mounted && state.phase == ChatMediaAccessPhase.playing) {
      state = ChatMediaAccessState(
          phase: ChatMediaAccessPhase.ready, access: state.access);
    }
  }

  @override
  void dispose() {
    unawaited(_player.dispose());
    super.dispose();
  }
}

final chatMediaAccessControllerProvider = StateNotifierProvider.autoDispose
    .family<ChatMediaAccessController, ChatMediaAccessState, String>(
        (ref, messageId) {
  return ChatMediaAccessController(
    player: JustAudioPreviewPlayer(),
    renew: () async {
      final sessionId = ref.read(authControllerProvider).sessionId;
      final token =
          await ref.read(firebaseAuthProvider).currentUser?.getIdToken();
      if (sessionId == null || token == null) {
        throw StateError('User not authenticated');
      }
      try {
        return await ref.read(chatMediaApiClientProvider).renewMediaUrl(
            token: token, sessionId: sessionId, messageId: messageId);
      } on ChatApiException catch (error) {
        if (error.statusCode == 401) {
          await ref.read(authControllerProvider.notifier).logout();
        }
        rethrow;
      }
    },
    download: downloadChatPdf,
  );
});
