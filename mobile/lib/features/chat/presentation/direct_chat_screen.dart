import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/chat/application/direct_chat_controller.dart';

class DirectChatScreen extends ConsumerStatefulWidget {
  const DirectChatScreen({super.key, required this.peerId});

  final String peerId;

  @override
  ConsumerState<DirectChatScreen> createState() => _DirectChatScreenState();
}

class _DirectChatScreenState extends ConsumerState<DirectChatScreen> {
  final _composer = TextEditingController();

  @override
  void initState() {
    super.initState();
    Future<void>.microtask(() => ref
        .read(directChatControllerProvider(widget.peerId).notifier)
        .open(widget.peerId));
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(directChatControllerProvider(widget.peerId));
    final rtl = Directionality.of(context) == TextDirection.rtl;
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
              Expanded(
                child: ListView.builder(
                  reverse: true,
                  itemCount: state.messages.length,
                  itemBuilder: (context, index) => ListTile(
                    title: Text(state.messages[index].content),
                  ),
                ),
              ),
              SafeArea(
                child: Row(
                  children: [
                    Expanded(child: TextField(controller: _composer)),
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
    _composer.dispose();
    super.dispose();
  }
}
