import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';

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

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(circleDiscoveryControllerProvider);
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    final chevron = isRtl ? Icons.chevron_left : Icons.chevron_right;

    return Scaffold(
      appBar: AppBar(
        title: Text(isRtl ? 'المحادثات' : 'Chats'),
      ),
      body: SafeArea(
        child: state.isLoading && state.myCircles.isEmpty
            ? const Center(child: CircularProgressIndicator())
            : state.myCircles.isEmpty
                ? EmptyStateCard(
                    key: const Key('chatsEmpty'),
                    title: isRtl ? 'لا توجد محادثات' : 'No chats yet',
                    hint: isRtl
                        ? 'انضم إلى حلقة لتظهر محادثتها هنا'
                        : 'Join a circle to see its group chat here',
                  )
                : RefreshIndicator(
                    onRefresh: () => ref
                        .read(circleDiscoveryControllerProvider.notifier)
                        .loadMyCircles(),
                    child: ListView.separated(
                      padding: const EdgeInsets.all(16),
                      itemCount: state.myCircles.length,
                      separatorBuilder: (_, __) => const SizedBox(height: 12),
                      itemBuilder: (context, index) {
                        final circle = state.myCircles[index];
                        return Card(
                          child: ListTile(
                            key: Key('chatCircle-${circle.id}'),
                            leading: Icon(
                              Icons.forum,
                              color: Theme.of(context).colorScheme.primary,
                            ),
                            title: Text(
                              circle.name,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                            ),
                            trailing: Icon(chevron),
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
                      },
                    ),
                  ),
      ),
    );
  }
}
