import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

const chatSelfDeleteWindow = Duration(minutes: 10);

class ChatModerationState {
  const ChatModerationState(
      {this.messages = const [],
      this.deletedMessageIds = const {},
      this.conflict = false,
      this.actionError});
  final List<ChatMessage> messages;
  final Set<String> deletedMessageIds;
  final bool conflict;
  final String? actionError;
}

typedef ChatModerationCredentials
    = Future<({String token, String sessionId, String userId})> Function();

class ChatModerationController extends StateNotifier<ChatModerationState> {
  ChatModerationController(this._api, this._credentials)
      : super(const ChatModerationState());

  final ChatModerationApi _api;
  final ChatModerationCredentials _credentials;

  void setMessages(Iterable<ChatMessage> messages) =>
      state = ChatModerationState(
        messages: List.unmodifiable(messages),
        deletedMessageIds: {...state.deletedMessageIds},
      );

  bool canDelete(ChatMessage message,
      {required String userId, bool isTeacher = false, DateTime? now}) {
    if (message.deletedAt != null) return false;
    if (isTeacher) return true;
    return message.senderId == userId &&
        !(now ?? DateTime.now().toUtc())
            .isAfter(message.sentAt.add(chatSelfDeleteWindow));
  }

  Future<bool> deleteCircleMessage(String circleId, ChatMessage message,
      {required DateTime now, bool isTeacher = false}) async {
    final credentials = await _credentials();
    if (!canDelete(message,
        userId: credentials.userId, isTeacher: isTeacher, now: now)) {
      return false;
    }
    try {
      await _api.deleteCircleMessage(
          token: credentials.token,
          sessionId: credentials.sessionId,
          circleId: circleId,
          messageId: message.id);
      _redact(message.id, now);
      return true;
    } on ChatApiException catch (error) {
      state = ChatModerationState(
          messages: state.messages,
          deletedMessageIds: state.deletedMessageIds,
          conflict: error.code == 'ERR_CONFLICT',
          actionError: error.code == 'ERR_CONFLICT'
              ? 'Chat action conflicts with current state.'
              : 'Chat request failed.');
      return false;
    }
  }

  Future<bool> deleteDirectMessage(String peerId, ChatMessage message,
      {required DateTime now}) async {
    final credentials = await _credentials();
    if (!canDelete(message, userId: credentials.userId, now: now)) return false;
    try {
      await _api.deleteDirectMessage(
          token: credentials.token,
          sessionId: credentials.sessionId,
          userId: peerId,
          messageId: message.id);
      _redact(message.id, now);
      return true;
    } on ChatApiException catch (error) {
      state = ChatModerationState(
          messages: state.messages,
          deletedMessageIds: state.deletedMessageIds,
          conflict: error.code == 'ERR_CONFLICT',
          actionError: error.code == 'ERR_CONFLICT'
              ? 'Chat action conflicts with current state.'
              : 'Chat request failed.');
      return false;
    }
  }

  void handleDeletionEvent(ChatMessageDeletedEvent event) =>
      _redact(event.messageId, event.deletedAt);

  void _redact(String messageId, DateTime deletedAt) {
    if (state.deletedMessageIds.contains(messageId)) return;
    final ids = {...state.deletedMessageIds, messageId};
    state = ChatModerationState(
      messages: state.messages
          .map((message) => message.id == messageId
              ? ChatMessage(
                  id: message.id,
                  senderId: message.senderId,
                  circleId: message.circleId,
                  dmPeerId: message.dmPeerId,
                  content: '',
                  type: message.type,
                  sentAt: message.sentAt,
                  deliveryStatus: message.deliveryStatus,
                  senderName: message.senderName,
                  replyToId: message.replyToId,
                  replyPreview: message.replyPreview?.copyWithDeleted(),
                  deletedAt: deletedAt,
                  readReceipts: message.readReceipts,
                )
              : message)
          .toList(growable: false),
      deletedMessageIds: ids,
    );
  }
}

extension on ChatReplyPreviewProjection {
  ChatReplyPreviewProjection copyWithDeleted() => ChatReplyPreviewProjection(
      id: id, senderName: senderName, preview: '', deleted: true);
}

final chatModerationControllerProvider =
    StateNotifierProvider<ChatModerationController, ChatModerationState>((ref) {
  final auth = ref.watch(authControllerProvider);
  Future<({String token, String sessionId, String userId})>
      credentials() async {
    final user = ref.read(firebaseAuthProvider).currentUser;
    final token = await user?.getIdToken();
    final sessionId = auth.sessionId;
    final userId = auth.user?.id;
    if (token == null || token.isEmpty || sessionId == null || userId == null) {
      throw StateError('User not authenticated');
    }
    return (token: token, sessionId: sessionId, userId: userId);
  }

  return ChatModerationController(
      ref.watch(chatApiClientProvider), credentials);
});
