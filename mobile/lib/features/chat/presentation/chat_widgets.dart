import 'dart:async';

import 'package:flutter/material.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_media_widgets.dart';

/// One chat message: sender attribution, plain-text content (markup is never
/// interpreted), and — for the sender's own messages — an icon+text delivery
/// status so meaning never depends on color alone (FR-038).
class ChatMessageBubble extends StatelessWidget {
  const ChatMessageBubble({
    super.key,
    required this.message,
    required this.isOwn,
    this.canDelete = false,
    this.onDelete,
    this.hasTerminalFailure = false,
  });

  final ChatMessage message;
  final bool isOwn;
  final bool canDelete;
  final VoidCallback? onDelete;

  /// A terminally failed draft shows the failure strip instead of the
  /// delivery badge — one message never claims "sending" and "failed" at
  /// once.
  final bool hasTerminalFailure;

  @override
  Widget build(BuildContext context) {
    final labels = _ChatLabels(Directionality.of(context) == TextDirection.rtl);
    final colorScheme = Theme.of(context).colorScheme;
    final senderLabel = message.senderName ?? labels.memberFallback;

    return Align(
      // Own messages sit on the directional end: right in LTR, left in RTL.
      alignment: isOwn
          ? AlignmentDirectional.centerEnd
          : AlignmentDirectional.centerStart,
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 4),
        padding: const EdgeInsets.all(12),
        constraints: BoxConstraints(
          maxWidth: MediaQuery.sizeOf(context).width * 0.8,
        ),
        decoration: BoxDecoration(
          color: isOwn
              ? colorScheme.primaryContainer
              : colorScheme.surfaceContainerHighest,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (!isOwn)
              Semantics(
                container: true,
                label: senderLabel,
                child: ExcludeSemantics(
                  child: Text(
                    senderLabel,
                    style: Theme.of(context)
                        .textTheme
                        .bodySmall
                        ?.copyWith(fontWeight: FontWeight.bold),
                  ),
                ),
              ),
            if (message.deletedAt != null)
              Text(Directionality.of(context) == TextDirection.rtl
                  ? 'الرسالة محذوفة'
                  : 'Message deleted')
            else if (message.type == ChatMessageType.text)
              Text(message.content)
            else
              ChatMediaMessageBody(key: ValueKey(message.id), message: message),
            if (canDelete && onDelete != null)
              Semantics(
                button: true,
                container: true,
                explicitChildNodes: true,
                label: Directionality.of(context) == TextDirection.rtl
                    ? 'حذف الرسالة'
                    : 'Delete message',
                child: ConstrainedBox(
                  constraints:
                      const BoxConstraints(minWidth: 48, minHeight: 48),
                  child: IconButton(
                    tooltip: Directionality.of(context) == TextDirection.rtl
                        ? 'حذف الرسالة'
                        : 'Delete message',
                    // The button renders inside the own-message bubble; the
                    // default onSurfaceVariant fails contrast on
                    // primaryContainer in light mode.
                    style: IconButton.styleFrom(
                        foregroundColor: colorScheme.onSurface),
                    onPressed: onDelete,
                    icon: const Icon(Icons.delete_outline),
                  ),
                ),
              ),
            if (isOwn && !hasTerminalFailure)
              _DeliveryStatusBadge(
                status: message.deliveryStatus,
                labels: labels,
                colorScheme: colorScheme,
              ),
          ],
        ),
      ),
    );
  }
}

class _DeliveryStatusBadge extends StatelessWidget {
  const _DeliveryStatusBadge({
    required this.status,
    required this.labels,
    required this.colorScheme,
  });

  final ChatDeliveryStatus status;
  final _ChatLabels labels;
  final ColorScheme colorScheme;

  @override
  Widget build(BuildContext context) {
    final label = labels.status(status);
    return Padding(
      padding: const EdgeInsets.only(top: 4),
      child: Semantics(
        container: true,
        label: label,
        child: ExcludeSemantics(
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(_icon, size: 16),
              const SizedBox(width: 4),
              Text(label, style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ),
      ),
    );
  }

  IconData get _icon => switch (status) {
        ChatDeliveryStatus.pending => Icons.hourglass_top,
        ChatDeliveryStatus.sent => Icons.done,
        ChatDeliveryStatus.delivered => Icons.done_all,
        ChatDeliveryStatus.read => Icons.visibility,
      };
}

/// Text composer with deterministic validation states: send is enabled only
/// for valid text and never while a send is in flight (no double-tap
/// duplicates), an overlong draft shows a live-region error label plus a
/// character counter, and empty drafts cannot be sent (FR-038). A failed
/// send keeps the draft for retry.
///
/// Isolated leaf widget: the TextEditingController draft is local UI state,
/// so `setState` here does not leak into feature state management.
class ChatComposer extends StatefulWidget {
  const ChatComposer({super.key, required this.onSend, this.onTyping});

  /// Returns true when the content was accepted (server acknowledged);
  /// false keeps the draft so a failed send never discards user input.
  final Future<bool> Function(String content) onSend;

  /// Emits start/stop transitions only; the authenticated transport owns
  /// delivery, authorization, and eventual expiry.
  final Future<void> Function(bool isTyping)? onTyping;

  @override
  State<ChatComposer> createState() => _ChatComposerState();
}

class _ChatComposerState extends State<ChatComposer> {
  final _controller = TextEditingController();
  bool _sending = false;
  bool _typing = false;

  @override
  void dispose() {
    if (_typing) unawaited(widget.onTyping?.call(false));
    _controller.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    if (_sending) return;
    _sending = true;
    setState(() {});
    // Clear only on acceptance: a rejected send keeps the draft for retry.
    final accepted = await widget.onSend(_controller.text);
    _sending = false;
    if (!mounted) return;
    if (accepted) _controller.clear();
    setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final labels = _ChatLabels(Directionality.of(context) == TextDirection.rtl);
    final text = _controller.text;
    final validation = validateChatText(text);
    // Disabled while a send is in flight so double-taps cannot duplicate.
    final canSend = !_sending && validation == ChatTextValidation.valid;

    return Column(
      children: [
        if (validation == ChatTextValidation.tooLong)
          Semantics(
            container: true,
            liveRegion: true,
            label: labels.tooLong,
            child: ExcludeSemantics(
              child: Text(
                labels.tooLong,
                style: Theme.of(context)
                    .textTheme
                    .bodyMedium
                    ?.copyWith(color: Theme.of(context).colorScheme.error),
              ),
            ),
          ),
        Row(
          children: [
            Expanded(
              child: Semantics(
                container: true,
                label: labels.composerHint,
                child: TextField(
                  controller: _controller,
                  textInputAction: TextInputAction.send,
                  onSubmitted: canSend ? (_) => _send() : null,
                  onChanged: (value) {
                    final isTyping = value.isNotEmpty;
                    if (isTyping != _typing) {
                      _typing = isTyping;
                      unawaited(widget.onTyping?.call(isTyping));
                    }
                    setState(() {});
                  },
                ),
              ),
            ),
            Semantics(
              button: true,
              label: labels.send,
              child: ConstrainedBox(
                constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
                // Direction-neutral icon: no mirroring needed in RTL.
                child: IconButton(
                  onPressed: canSend ? _send : null,
                  icon: const Icon(Icons.arrow_upward),
                ),
              ),
            ),
          ],
        ),
        if (text.isNotEmpty)
          Align(
            alignment: AlignmentDirectional.centerEnd,
            child: Text(
              labels.counter(text.length),
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ),
      ],
    );
  }
}

/// Pagination trigger for the next older history page (cursor-based).
class ChatLoadOlderButton extends StatelessWidget {
  const ChatLoadOlderButton({super.key, required this.onPressed});

  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    final labels = _ChatLabels(Directionality.of(context) == TextDirection.rtl);
    return Semantics(
      button: true,
      label: labels.loadOlder,
      child: ConstrainedBox(
        constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
        child: OutlinedButton(
          onPressed: onPressed,
          child: ExcludeSemantics(child: Text(labels.loadOlder)),
        ),
      ),
    );
  }
}

class _ChatLabels {
  const _ChatLabels(this.rtl);

  final bool rtl;

  String get memberFallback =>
      rtl ? ChatUiLabels.memberFallback : ChatUiLabels.memberFallbackEn;
  String get composerHint =>
      rtl ? ChatUiLabels.composerHint : ChatUiLabels.composerHintEn;
  String get send => rtl ? ChatUiLabels.send : ChatUiLabels.sendEn;
  String get tooLong => rtl
      ? ChatUiLabels.tooLong
      : 'Message is too long; limit is ${ChatLimits.maxContentLength} characters';
  String get loadOlder =>
      rtl ? ChatUiLabels.loadOlder : ChatUiLabels.loadOlderEn;

  String counter(int length) => '$length/${ChatLimits.maxContentLength}';

  String status(ChatDeliveryStatus status) => switch (status) {
        ChatDeliveryStatus.pending =>
          rtl ? ChatUiLabels.statusPending : ChatUiLabels.statusPendingEn,
        ChatDeliveryStatus.sent =>
          rtl ? ChatUiLabels.statusSent : ChatUiLabels.statusSentEn,
        ChatDeliveryStatus.delivered =>
          rtl ? ChatUiLabels.statusDelivered : ChatUiLabels.statusDeliveredEn,
        ChatDeliveryStatus.read =>
          rtl ? ChatUiLabels.statusRead : ChatUiLabels.statusReadEn,
      };
}
