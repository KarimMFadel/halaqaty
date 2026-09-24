import 'package:flutter/material.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';

/// Delivery state projection that communicates through icon and text.
class ChatDeliveryStatusView extends StatelessWidget {
  const ChatDeliveryStatusView({required this.status, super.key});
  final ChatDeliveryStatus status;

  String get _label => switch (status) {
        ChatDeliveryStatus.pending => 'Pending',
        ChatDeliveryStatus.sent => 'Sent',
        ChatDeliveryStatus.delivered => 'Delivered',
        ChatDeliveryStatus.read => 'Read',
      };

  IconData get _icon => switch (status) {
        ChatDeliveryStatus.pending => Icons.schedule,
        ChatDeliveryStatus.sent => Icons.check,
        ChatDeliveryStatus.delivered => Icons.done_all,
        ChatDeliveryStatus.read => Icons.visibility,
      };

  @override
  Widget build(BuildContext context) => Semantics(
        container: true,
        label: _label,
        child: Row(mainAxisSize: MainAxisSize.min, children: [
          Icon(_icon, size: 14),
          const SizedBox(width: 4),
          Text(_label),
        ]),
      );
}

/// Textual typing indicator; expiry is owned by the presence controller.
class ChatTypingIndicator extends StatelessWidget {
  const ChatTypingIndicator({required this.userNames, super.key});
  final List<String> userNames;

  @override
  Widget build(BuildContext context) {
    if (userNames.isEmpty) return const SizedBox.shrink();
    return Semantics(
      liveRegion: true,
      label: '${userNames.join(', ')} typing',
      child: Text('${userNames.join(', ')} typing…'),
    );
  }
}
