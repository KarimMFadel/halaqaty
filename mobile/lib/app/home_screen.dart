import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_name_text.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_load_error.dart';

/// Home tab: branded overview of the user's circles and quick actions.
///
/// Sessions stay reachable from circle detail (their owning flow); this tab
/// only surfaces what existing APIs can derive client-side.
class HomeScreen extends ConsumerStatefulWidget {
  const HomeScreen({super.key});

  @override
  ConsumerState<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends ConsumerState<HomeScreen> {
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
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(
        title: Text(isRtl ? 'الرئيسية' : 'Home'),
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: () => ref
              .read(circleDiscoveryControllerProvider.notifier)
              .loadMyCircles(),
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              // Recoverable failure keeps loaded circles visible under a
              // retryable error card instead of clearing safe context.
              if (state.failure != null)
                CircleLoadError(
                  failure: state.failure!,
                  onRetry: () => ref
                      .read(circleDiscoveryControllerProvider.notifier)
                      .loadMyCircles(),
                ),
              if (state.failure != null) const SizedBox(height: 16),
              Row(
                children: [
                  const HalaqatyLogo(size: 40),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      isRtl ? 'مرحباً بك' : 'Welcome',
                      style: Theme.of(context)
                          .textTheme
                          .headlineSmall
                          ?.copyWith(color: scheme.primary),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 24),
              SectionHeader(
                title: isRtl ? 'حلقاتي' : 'My circles',
              ),
              if (state.isLoading && state.myCircles.isEmpty)
                const HalaqatyLoading(key: Key('homeLoading')),
              if (!state.isLoading && state.myCircles.isEmpty)
                EmptyStateCard(
                  key: const Key('homeNoCircles'),
                  title: isRtl ? 'لا توجد حلقات بعد' : 'No circles yet',
                  hint: isRtl
                      ? 'اكتشف الحلقات العامة أو انضم برمز دعوة'
                      : 'Discover public circles or join with an invite code',
                ),
              ...state.myCircles.map(
                (circle) => _CircleCard(circle: circle, isRtl: isRtl),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _CircleCard extends StatelessWidget {
  const _CircleCard({required this.circle, required this.isRtl});

  final CircleSummary circle;
  final bool isRtl;

  @override
  Widget build(BuildContext context) {
    final chevron = isRtl ? Icons.chevron_left : Icons.chevron_right;
    return Semantics(
      button: true,
      label: circle.name,
      child: Card(
        margin: const EdgeInsets.only(bottom: 12),
        child: ListTile(
          key: Key('homeCircle-${circle.id}'),
          contentPadding:
              const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          leading: Icon(
            Icons.auto_stories,
            color: Theme.of(context).colorScheme.primary,
          ),
          title: CircleNameText(name: circle.name),
          subtitle: circle.description == null || circle.description!.isEmpty
              ? null
              : Text(
                  circle.description!,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
          trailing: Icon(chevron),
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute<void>(
              builder: (_) => CircleDetailScreen(circleId: circle.id),
            ),
          ),
        ),
      ),
    );
  }
}
