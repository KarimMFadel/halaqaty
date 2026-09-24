import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_load_error.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_name_text.dart';

/// Chats tab: group chats of the user's circles.
///
/// There is no backend conversations endpoint; direct chats remain reachable
/// from circle members (their owning flow). This list is derived client-side
/// from the user's circle memberships.
class ChatsScreen extends ConsumerStatefulWidget {
  const ChatsScreen({super.key});

  @override
  ConsumerState<ChatsScreen> createState() => _ChatsScreenState();
}

class _ChatsScreenState extends ConsumerState<ChatsScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      if (mounted) {
        await ref
            .read(circleDiscoveryControllerProvider.notifier)
            .loadMyCircles();
      }
    });
  }

  Future<void> _reload() =>
      ref.read(circleDiscoveryControllerProvider.notifier).loadMyCircles();

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(circleDiscoveryControllerProvider);
    final isRtl = Directionality.of(context) == TextDirection.rtl;

    return Scaffold(
      appBar: AppBar(
        title: Text(isRtl ? 'المحادثات' : 'Chats'),
      ),
      body: SafeArea(
        child: switch ((
          state.isLoading,
          state.failure != null,
          state.myCircles.isEmpty,
        )) {
          // Initial wait: branded loading, never a bare spinner.
          (true, _, true) => const HalaqatyLoading(key: Key('chatsLoading')),
          // Failed load with nothing to show: error, not a fake empty state.
          (false, true, true) => CircleLoadError(
              failure: state.failure!,
              onRetry: _reload,
            ),
          // Genuinely empty after a successful load.
          (false, false, true) => EmptyStateCard(
              key: const Key('chatsEmpty'),
              title: isRtl ? 'لا توجد محادثات' : 'No chats yet',
              hint: isRtl
                  ? 'انضم إلى حلقة لتظهر محادثتها هنا'
                  : 'Join a circle to see its group chat here',
            ),
          // Refresh failure keeps the last safe list under a retry notice.
          _ => RefreshIndicator(
              onRefresh: _reload,
              child: ListView(
                padding: const EdgeInsets.all(16),
                children: [
                  if (state.failure != null) ...[
                    CircleLoadError(
                      failure: state.failure!,
                      onRetry: _reload,
                    ),
                    const SizedBox(height: 12),
                  ],
                  for (var i = 0; i < state.myCircles.length; i++) ...[
                    if (i > 0) const SizedBox(height: 12),
                    _ChatCircleTile(circle: state.myCircles[i]),
                  ],
                ],
              ),
            ),
        },
      ),
    );
  }
}

class _ChatCircleTile extends StatelessWidget {
  const _ChatCircleTile({required this.circle});

  final CircleSummary circle;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        key: Key('chatCircle-${circle.id}'),
        leading: Icon(
          Icons.forum,
          color: Theme.of(context).colorScheme.primary,
        ),
        title: CircleNameText(name: circle.name),
        // Material mirrors this direction-aware icon once for RTL (FR-010).
        trailing: const Icon(Icons.chevron_right),
        onTap: () => Navigator.of(context).push(
          MaterialPageRoute<void>(
            builder: (_) => GroupChatScreen(
              circleId: circle.id,
              circleName: circle.name,
            ),
          ),
        ),
      ),
    );
  }
}
