import 'package:flutter/material.dart';

import 'package:halaqaty_mobile/features/chat/application/chat_discovery_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';

/// Safe quoted reply preview. Deleted targets intentionally expose no text.
class ChatReplyPreviewView extends StatelessWidget {
  const ChatReplyPreviewView({required this.reply, super.key});

  final ChatReplyPreview reply;

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final label =
        rtl ? 'الرد على ${reply.senderName}' : 'Reply to ${reply.senderName}';
    final quote = reply.deleted
        ? (rtl ? 'الرسالة الأصلية محذوفة' : 'Original message deleted')
        : reply.preview;
    return Semantics(
      container: true,
      label: label,
      value: quote,
      excludeSemantics: true,
      child: Card(
        child: Padding(
          padding: const EdgeInsets.all(8),
          child: Row(
            children: [
              const Icon(Icons.reply, size: 18),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(reply.senderName,
                        maxLines: 1, overflow: TextOverflow.ellipsis),
                    Text(
                      reply.deleted
                          ? '$quote (Original message deleted)'
                          : reply.preview,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class ChatReplyComposer extends StatefulWidget {
  const ChatReplyComposer({
    required this.reply,
    required this.onSend,
    this.onCancel,
    super.key,
  });

  final ChatReplyPreview? reply;
  final ValueChanged<String> onSend;
  final VoidCallback? onCancel;

  @override
  State<ChatReplyComposer> createState() => _ChatReplyComposerState();
}

class _ChatReplyComposerState extends State<ChatReplyComposer> {
  final _textController = TextEditingController();

  @override
  void dispose() {
    _textController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final reply = widget.reply;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (reply != null) ChatReplyPreviewView(reply: reply),
        Row(
          children: [
            Expanded(
              child: TextField(
                controller: _textController,
                textDirection: Directionality.of(context),
                decoration: InputDecoration(
                  hintText: Directionality.of(context) == TextDirection.rtl
                      ? 'اكتب ردًا'
                      : 'Write a reply',
                ),
                maxLength: ChatLimits.maxContentLength,
              ),
            ),
            IconButton(
              tooltip: Directionality.of(context) == TextDirection.rtl
                  ? 'إرسال'
                  : 'Send',
              icon: const Icon(Icons.send),
              onPressed: () {
                final text = _textController.text.trim();
                if (validateChatText(text) != ChatTextValidation.valid) return;
                widget.onSend(text);
                _textController.clear();
              },
            ),
            if (widget.onCancel != null)
              IconButton(
                tooltip: Directionality.of(context) == TextDirection.rtl
                    ? 'إلغاء الرد'
                    : 'Cancel reply',
                icon: const Icon(Icons.close),
                onPressed: widget.onCancel,
              ),
          ],
        ),
      ],
    );
  }
}

class ChatSearchResults extends StatelessWidget {
  const ChatSearchResults({required this.messages, super.key});

  final List<ChatMessage> messages;

  @override
  Widget build(BuildContext context) => ListView(
        shrinkWrap: true,
        physics: const NeverScrollableScrollPhysics(),
        children: [
          for (final message in messages)
            Card(
              child: ListTile(
                key: ValueKey(message.id),
                title: Text(message.content),
                subtitle: message.senderName == null
                    ? null
                    : Text(message.senderName!),
              ),
            ),
        ],
      );
}

class ChatPinnedBar extends StatelessWidget {
  const ChatPinnedBar({required this.messages, super.key});

  final List<ChatMessage> messages;

  @override
  Widget build(BuildContext context) => ListView(
        shrinkWrap: true,
        physics: const NeverScrollableScrollPhysics(),
        children: [
          for (final message in messages)
            Card(
              child: ListTile(
                key: ValueKey('pinned-${message.id}'),
                leading: const Icon(Icons.push_pin),
                title: Text(message.content),
              ),
            ),
        ],
      );
}

class ChatPinLimitNotice extends StatelessWidget {
  const ChatPinLimitNotice({super.key});

  @override
  Widget build(BuildContext context) => Semantics(
        container: true,
        label: 'تم تثبيت خمس رسائل بالفعل',
        excludeSemantics: true,
        child: const Row(
          children: [
            Icon(Icons.info_outline),
            SizedBox(width: 8),
            Expanded(child: Text('تم تثبيت خمس رسائل بالفعل')),
          ],
        ),
      );
}

class ChatDiscoveryErrorView extends StatelessWidget {
  const ChatDiscoveryErrorView({required this.failure, super.key});

  final ChatDiscoveryFailure? failure;

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final text = switch (failure) {
      ChatDiscoveryFailure.unauthorized =>
        rtl ? 'لا يمكنك الوصول إلى هذه المحادثة' : 'Chat access is unavailable',
      ChatDiscoveryFailure.conflict => rtl
          ? 'تعارضت العملية مع حالة المحادثة الحالية'
          : 'Chat action conflicts with current state',
      ChatDiscoveryFailure.unavailable => rtl
          ? 'خدمة المحادثة غير متاحة مؤقتًا'
          : 'Chat service is temporarily unavailable',
      _ => rtl ? 'تعذر تنفيذ طلب المحادثة' : 'Chat request failed',
    };
    return Semantics(
      container: true,
      liveRegion: true,
      label: text,
      child: ExcludeSemantics(child: Text(text)),
    );
  }
}
