import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_media_widgets.dart';

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

  @override
  void initState() {
    super.initState();
    _controller =
        ref.read(groupChatControllerProvider(widget.circleId).notifier);
    _controller.setReadOnly(widget.readOnly);
    unawaited(_controller.open(widget.circleId));
  }

  @override
  void dispose() {
    unawaited(_controller.close());
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(groupChatControllerProvider(widget.circleId));
    final currentUserId = ref.watch(authControllerProvider).user?.id;
    final labels =
        _ScreenLabels(Directionality.of(context) == TextDirection.rtl);
    // Oldest-first view of the newest-first controller projection.
    final displayMessages = state.messages.reversed.toList();

    return Scaffold(
      appBar: AppBar(title: Text(widget.circleName ?? labels.title)),
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
                            return ChatMessageBubble(
                              message: message,
                              isOwn: message.senderId == currentUserId,
                            );
                          },
                        ),
                ),
                if (!state.readOnly) ...[
                  ChatMediaComposerBar(circleId: widget.circleId),
                  ChatComposer(onSend: _controller.sendText),
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
  String get readOnly => rtl
      ? 'هذه المحادثة للقراءة فقط'
      : 'This conversation is read-only';
}
