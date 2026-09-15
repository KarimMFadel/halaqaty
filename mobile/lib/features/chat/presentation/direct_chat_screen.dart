import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_moderation_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
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
        DirectChatStatus.error || DirectChatStatus.accessLost => Center(
            child: Text(rtl
                ? 'لا يمكن الوصول إلى هذه المحادثة'
                : 'This conversation is unavailable'),
          ),
        _ => Column(
            children: [
              if (_controller.presence case final presence?)
                ChatTypingIndicator(
                  userNames: presence.typingUserIdsFor(dmPeerId: widget.peerId),
                ),
              Expanded(
                child: ListView.builder(
                  reverse: true,
                  itemCount: state.messages.length,
                  itemBuilder: (context, index) => ChatMessageBubble(
                    message: state.messages[index],
                    isOwn: state.messages[index].senderId == currentUserId,
                    canDelete: moderation.canDelete(state.messages[index],
                        userId: currentUserId ?? '',
                        now: DateTime.now().toUtc()),
                    onDelete: () => unawaited(
                        _deleteMessage(moderation, state.messages[index])),
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
                    IconButton(
                      tooltip: rtl ? 'إرسال' : 'Send',
                      icon: const Icon(Icons.send),
                      onPressed: () async {
                        final sent = await ref
                            .read(directChatControllerProvider(widget.peerId)
                                .notifier)
                            .sendText(_composer.text);
                        if (sent) _composer.clear();
                      },
                    ),
                  ],
                ),
              ),
            ],
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
