import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_moderation_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_status_widgets.dart';

class DirectChatScreen extends ConsumerStatefulWidget {
  const DirectChatScreen({super.key, required this.peerId});

  final String peerId;

  @override
  ConsumerState<DirectChatScreen> createState() => _DirectChatScreenState();
}

class _DirectChatScreenState extends ConsumerState<DirectChatScreen> {
  final _composer = TextEditingController();
  late final DirectChatController _controller;
  bool _typing = false;
  bool _sendFailed = false;

  @override
  void initState() {
    super.initState();
    _controller =
        ref.read(directChatControllerProvider(widget.peerId).notifier);
    _controller.presence?.addListener(_onPresenceChanged);
    Future<void>.microtask(() => _controller.open(widget.peerId));
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(directChatControllerProvider(widget.peerId));
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final currentUserId = ref.watch(authControllerProvider).user?.id;
    final moderation = ref.read(chatModerationControllerProvider.notifier);
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? 'محادثة مباشرة' : 'Direct chat')),
      body: switch (state.status) {
        DirectChatStatus.loading =>
          const Center(child: CircularProgressIndicator()),
        // Recoverable load failure keeps an explicit retry path.
        DirectChatStatus.error => Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  rtl ? ChatUiLabels.historyError : ChatUiLabels.historyErrorEn,
                  style: Theme.of(context)
                      .textTheme
                      .bodyMedium
                      ?.copyWith(color: Theme.of(context).colorScheme.error),
                ),
                const SizedBox(height: 8),
                Semantics(
                  button: true,
                  label: rtl ? ChatUiLabels.retry : ChatUiLabels.retryEn,
                  child: ConstrainedBox(
                    constraints:
                        const BoxConstraints(minWidth: 48, minHeight: 48),
                    child: OutlinedButton(
                      onPressed: () =>
                          unawaited(_controller.open(widget.peerId)),
                      child: ExcludeSemantics(
                        child: Text(
                            rtl ? ChatUiLabels.retry : ChatUiLabels.retryEn),
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        // FR-009: lost access is terminal — say retry is unavailable and
        // offer a safe exit instead of a retry that loops without effect.
        DirectChatStatus.accessLost => Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  rtl ? ChatUiLabels.accessLost : ChatUiLabels.accessLostEn,
                  style: Theme.of(context)
                      .textTheme
                      .bodyMedium
                      ?.copyWith(color: Theme.of(context).colorScheme.error),
                ),
                const SizedBox(height: 8),
                Semantics(
                  button: true,
                  label: rtl ? ChatUiLabels.back : ChatUiLabels.backEn,
                  child: ConstrainedBox(
                    constraints:
                        const BoxConstraints(minWidth: 48, minHeight: 48),
                    child: OutlinedButton(
                      onPressed: () => unawaited(Navigator.maybePop(context)),
                      child: ExcludeSemantics(
                        child:
                            Text(rtl ? ChatUiLabels.back : ChatUiLabels.backEn),
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        _ => Padding(
            padding: const EdgeInsets.all(8),
            child: Column(
              children: [
                if (_controller.presence case final presence?)
                  ChatTypingIndicator(
                    userNames:
                        presence.typingUserIdsFor(dmPeerId: widget.peerId),
                  ),
                Expanded(
                  child: state.messages.isEmpty
                      ? Center(
                          child: Text(
                              rtl ? ChatUiLabels.empty : ChatUiLabels.emptyEn))
                      : ListView.builder(
                          reverse: true,
                          itemCount: state.messages.length,
                          itemBuilder: (context, index) => ChatMessageBubble(
                            message: state.messages[index],
                            isOwn:
                                state.messages[index].senderId == currentUserId,
                            canDelete: moderation.canDelete(
                                state.messages[index],
                                userId: currentUserId ?? '',
                                now: DateTime.now().toUtc()),
                            onDelete: () => unawaited(_deleteMessage(
                                moderation, state.messages[index])),
                          ),
                        ),
                ),
                if (_sendFailed)
                  Semantics(
                    container: true,
                    liveRegion: true,
                    label: rtl
                        ? ChatUiLabels.actionFailed
                        : ChatUiLabels.actionFailedEn,
                    child: ExcludeSemantics(
                      child: Text(
                        rtl
                            ? ChatUiLabels.actionFailed
                            : ChatUiLabels.actionFailedEn,
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                            color: Theme.of(context).colorScheme.error),
                      ),
                    ),
                  ),
                SafeArea(
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          controller: _composer,
                          onChanged: (value) {
                            final isTyping = value.isNotEmpty;
                            if (isTyping != _typing) {
                              _typing = isTyping;
                              unawaited(_controller.setTyping(isTyping));
                            }
                          },
                        ),
                      ),
                      // Empty drafts cannot be sent; a rejected send keeps
                      // the draft and announces the failure above (FR-038).
                      ValueListenableBuilder<TextEditingValue>(
                        valueListenable: _composer,
                        builder: (context, value, _) => IconButton(
                          tooltip: rtl ? 'إرسال' : 'Send',
                          icon: const Icon(Icons.send),
                          onPressed: value.text.trim().isEmpty
                              ? null
                              : () async {
                                  final sent = await ref
                                      .read(directChatControllerProvider(
                                              widget.peerId)
                                          .notifier)
                                      .sendText(_composer.text);
                                  if (!mounted) return;
                                  setState(() => _sendFailed = !sent);
                                  if (sent) _composer.clear();
                                },
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
      },
    );
  }

  @override
  void dispose() {
    if (_typing) unawaited(_controller.setTyping(false));
    _composer.dispose();
    super.dispose();
  }

  void _onPresenceChanged(ChatPresenceState _) {
    if (mounted) setState(() {});
  }

  Future<void> _deleteMessage(
      ChatModerationController moderation, ChatMessage message) async {
    final deletedAt = DateTime.now().toUtc();
    final deleted = await moderation.deleteDirectMessage(widget.peerId, message,
        now: deletedAt);
    if (deleted && mounted) {
      _controller.handleRealtimeEvent(ChatMessageDeletedEvent(
        eventId: 'local-delete-${message.id}',
        messageId: message.id,
        deletedAt: deletedAt,
      ));
    }
  }
}
