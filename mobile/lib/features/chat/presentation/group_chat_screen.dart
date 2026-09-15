import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_discovery_controller.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_status_widgets.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_media_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_discovery_widgets.dart';

/// Arabic-first RTL-aware group thread for one circle. Owns no chat state:
/// it projects the authoritative [GroupChatController] and renders
/// deterministic loading/empty/error/ready states (FR-038).
class GroupChatScreen extends ConsumerStatefulWidget {
  const GroupChatScreen({
    super.key,
    required this.circleId,
    this.circleName,
    this.readOnly = false,
  });

  final String circleId;
  final String? circleName;
  final bool readOnly;

  @override
  ConsumerState<GroupChatScreen> createState() => _GroupChatScreenState();
}

class _GroupChatScreenState extends ConsumerState<GroupChatScreen> {
  late final GroupChatController _controller;
  late final ChatDiscoveryController _discovery;

  @override
  void initState() {
    super.initState();
    _controller =
        ref.read(groupChatControllerProvider(widget.circleId).notifier);
    _discovery =
        ref.read(chatDiscoveryControllerProvider(widget.circleId).notifier);
    _controller.presence?.addListener(_onPresenceChanged);
    // Riverpod forbids provider writes during mount; defer the projection
    // flag until the first frame while opening the authoritative history.
    Future<void>.microtask(() {
      if (mounted) _controller.setReadOnly(widget.readOnly);
    });
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_discovery.loadPinned(widget.circleId));
    });
    unawaited(_controller.open(widget.circleId));
  }

  @override
  void dispose() {
    unawaited(_controller.close());
    super.dispose();
  }

  void _onPresenceChanged(ChatPresenceState _) {
    if (mounted) setState(() {});
  }

  Future<void> _showSearch() async {
    await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(
            _ScreenLabels(Directionality.of(context) == TextDirection.rtl)
                .search),
        content: TextField(
          autofocus: true,
          textDirection: Directionality.of(context),
          onChanged: (query) =>
              unawaited(_discovery.search(widget.circleId, query)),
        ),
      ),
    );
  }

  /// Opens the edit sheet for one terminally failed draft (FR-008): the
  /// controller rotates the idempotency key, so the edited text is a fresh
  /// logical send rather than a key-conflicting replay.
  Future<void> _editTerminalDraft(
      BuildContext context, ChatMessage message) async {
    final controller = TextEditingController(text: message.content);
    final edited = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(
            _ScreenLabels(Directionality.of(context) == TextDirection.rtl)
                .editDraft),
        content: TextField(
          controller: controller,
          autofocus: true,
          maxLines: null,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: Text(
                _ScreenLabels(Directionality.of(context) == TextDirection.rtl)
                    .cancelEdit),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, controller.text),
            child: Text(
                _ScreenLabels(Directionality.of(context) == TextDirection.rtl)
                    .saveEdit),
          ),
        ],
      ),
    );
    controller.dispose();
    if (edited != null && mounted) {
      await _controller.editPending(message.id, edited);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(groupChatControllerProvider(widget.circleId));
    final discovery =
        ref.watch(chatDiscoveryControllerProvider(widget.circleId));
    final currentUserId = ref.watch(authControllerProvider).user?.id;
    final labels =
        _ScreenLabels(Directionality.of(context) == TextDirection.rtl);
    // Oldest-first view of the newest-first controller projection.
    final displayMessages = state.messages.reversed.toList();
    final presence = _controller.presence;

    return Scaffold(
      appBar: AppBar(
        title: Text(widget.circleName ?? labels.title),
        actions: [
          Semantics(
            button: true,
            label: labels.search,
            child: IconButton(
              tooltip: labels.search,
              onPressed: _showSearch,
              icon: const Icon(Icons.search),
            ),
          ),
        ],
      ),
      body: switch (state.status) {
        GroupChatStatus.idle ||
        GroupChatStatus.loading =>
          _LoadingStatus(labels: labels),
        GroupChatStatus.error || GroupChatStatus.accessLost => _ErrorStatus(
            labels: labels,
            onRetry: () => unawaited(_controller.open(widget.circleId)),
          ),
        GroupChatStatus.ready => Padding(
            padding: const EdgeInsets.all(8),
            child: Column(
              children: [
                if (discovery.pinnedMessages.isNotEmpty)
                  ChatPinnedBar(messages: discovery.pinnedMessages),
                if (discovery.searchResults.isNotEmpty)
                  ChatSearchResults(messages: discovery.searchResults),
                if (discovery.pinLimitReached) const ChatPinLimitNotice(),
                if (discovery.failure != null)
                  ChatDiscoveryErrorView(failure: discovery.failure),
                if (presence != null)
                  ChatTypingIndicator(
                    userNames:
                        presence.typingUserIdsFor(circleId: widget.circleId),
                  ),
                if (state.actionErrorMessage != null)
                  _ActionErrorLabel(labels: labels),
                if (state.hasMore)
                  ChatLoadOlderButton(onPressed: _controller.loadOlder),
                Expanded(
                  // Controller state is newest-first; the thread displays it
                  // reversed once per build so the newest message sits above
                  // the composer.
                  child: state.messages.isEmpty
                      ? Center(child: Text(labels.empty))
                      : ListView.builder(
                          itemCount: state.messages.length,
                          itemBuilder: (context, index) {
                            final message = displayMessages[index];
                            final terminalError =
                                state.terminalFailures[message.id];
                            return Column(
                              children: [
                                GestureDetector(
                                  onLongPress: () =>
                                      _discovery.selectReplyTarget(
                                    ChatReplyPreview(
                                      id: message.id,
                                      senderName: message.senderName ??
                                          labels.memberFallback,
                                      preview: message.content,
                                      deleted: message.deletedAt != null,
                                    ),
                                  ),
                                  child: ChatMessageBubble(
                                    message: message,
                                    isOwn: message.senderId == currentUserId,
                                  ),
                                ),
                                if (!state.readOnly)
                                  Semantics(
                                    button: true,
                                    label: message.pinnedAt == null
                                        ? labels.pin
                                        : labels.unpin,
                                    child: IconButton(
                                      tooltip: message.pinnedAt == null
                                          ? labels.pin
                                          : labels.unpin,
                                      icon: Icon(message.pinnedAt == null
                                          ? Icons.push_pin_outlined
                                          : Icons.push_pin),
                                      onPressed: () => unawaited(
                                        message.pinnedAt == null
                                            ? _discovery.pin(
                                                widget.circleId, message.id)
                                            : _discovery.unpin(
                                                widget.circleId, message.id),
                                      ),
                                    ),
                                  ),
                                if (terminalError != null)
                                  _TerminalFailureActions(
                                    labels: labels,
                                    message: message,
                                    onEdit: () =>
                                        _editTerminalDraft(context, message),
                                    onDiscard: () => unawaited(
                                        _controller.discardPending(message.id)),
                                    onRetry: () => unawaited(
                                        _controller.retryPending(
                                            idempotencyKey: message.id)),
                                  ),
                              ],
                            );
                          },
                        ),
                ),
                if (!state.readOnly) ...[
                  ChatMediaComposerBar(circleId: widget.circleId),
                  if (discovery.replyTarget != null)
                    ChatReplyPreviewView(reply: discovery.replyTarget!),
                  ChatComposer(
                    onSend: (content) async {
                      final accepted = await _controller.sendText(
                        content,
                        replyToId: discovery.replyTarget?.id,
                      );
                      if (accepted) _discovery.clearReplyTarget();
                      return accepted;
                    },
                    onTyping: _controller.setTyping,
                  ),
                ] else
                  Semantics(
                    container: true,
                    label: labels.readOnly,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Text(labels.readOnly),
                    ),
                  ),
              ],
            ),
          ),
      },
    );
  }
}

class _LoadingStatus extends StatelessWidget {
  const _LoadingStatus({required this.labels});

  final _ScreenLabels labels;

  @override
  Widget build(BuildContext context) => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const CircularProgressIndicator(),
            const SizedBox(height: 8),
            Text(labels.loading),
          ],
        ),
      );
}

class _ErrorStatus extends StatelessWidget {
  const _ErrorStatus({required this.labels, required this.onRetry});

  final _ScreenLabels labels;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // Never the raw controller error: safe localized copy only.
            Text(
              labels.historyError,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
            const SizedBox(height: 8),
            Semantics(
              button: true,
              label: labels.retry,
              child: ConstrainedBox(
                constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
                child: OutlinedButton(
                  onPressed: onRetry,
                  child: ExcludeSemantics(child: Text(labels.retry)),
                ),
              ),
            ),
          ],
        ),
      );
}

class _ActionErrorLabel extends StatelessWidget {
  const _ActionErrorLabel({required this.labels});

  final _ScreenLabels labels;

  @override
  Widget build(BuildContext context) => Semantics(
        container: true,
        liveRegion: true,
        label: labels.actionFailed,
        child: ExcludeSemantics(
          child: Text(
            labels.actionFailed,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        ),
      );
}

/// FR-008 terminal-failure affordance for one failed draft: non-color-only
/// failure semantics plus labeled 48dp edit/discard/retry controls.
class _TerminalFailureActions extends StatelessWidget {
  const _TerminalFailureActions({
    required this.labels,
    required this.message,
    required this.onEdit,
    required this.onDiscard,
    required this.onRetry,
  });

  final _ScreenLabels labels;
  final ChatMessage message;
  final VoidCallback onEdit;
  final VoidCallback onDiscard;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    return Align(
      alignment: AlignmentDirectional.centerEnd,
      child: Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.end,
          mainAxisSize: MainAxisSize.min,
          children: [
            Semantics(
              container: true,
              liveRegion: true,
              label: labels.sendFailed,
              child: ExcludeSemantics(
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(Icons.error_outline,
                        size: 16, color: colorScheme.error),
                    const SizedBox(width: 4),
                    Text(
                      labels.sendFailed,
                      style: Theme.of(context)
                          .textTheme
                          .bodySmall
                          ?.copyWith(color: colorScheme.error),
                    ),
                  ],
                ),
              ),
            ),
            Wrap(
              children: [
                _terminalAction(labels.editDraft, onEdit),
                _terminalAction(labels.discardDraft, onDiscard),
                _terminalAction(labels.retry, onRetry),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _terminalAction(String label, VoidCallback onPressed) => Semantics(
        button: true,
        label: label,
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 48),
          child: TextButton(
            onPressed: onPressed,
            child: ExcludeSemantics(child: Text(label)),
          ),
        ),
      );
}

class _ScreenLabels {
  const _ScreenLabels(this.rtl);

  final bool rtl;

  String get title => rtl ? ChatUiLabels.title : ChatUiLabels.titleEn;
  String get loading => rtl ? ChatUiLabels.loading : ChatUiLabels.loadingEn;
  String get empty => rtl ? ChatUiLabels.empty : ChatUiLabels.emptyEn;
  String get historyError =>
      rtl ? ChatUiLabels.historyError : ChatUiLabels.historyErrorEn;
  String get retry => rtl ? ChatUiLabels.retry : ChatUiLabels.retryEn;
  String get actionFailed =>
      rtl ? ChatUiLabels.actionFailed : ChatUiLabels.actionFailedEn;
  String get sendFailed =>
      rtl ? ChatUiLabels.sendFailed : ChatUiLabels.sendFailedEn;
  String get editDraft =>
      rtl ? ChatUiLabels.editDraft : ChatUiLabels.editDraftEn;
  String get discardDraft =>
      rtl ? ChatUiLabels.discardDraft : ChatUiLabels.discardDraftEn;
  String get cancelEdit => rtl ? ChatUiLabels.cancel : ChatUiLabels.cancelEn;
  String get saveEdit => rtl ? ChatUiLabels.send : ChatUiLabels.sendEn;
  String get readOnly =>
      rtl ? 'هذه المحادثة للقراءة فقط' : 'This conversation is read-only';
  String get search => rtl ? ChatUiLabels.search : ChatUiLabels.searchEn;
  String get memberFallback =>
      rtl ? ChatUiLabels.memberFallback : ChatUiLabels.memberFallbackEn;
  String get pin => rtl ? 'تثبيت الرسالة' : 'Pin message';
  String get unpin => rtl ? 'إلغاء تثبيت الرسالة' : 'Unpin message';
}
