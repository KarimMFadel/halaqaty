import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_join_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_name_text.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_load_error.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_ui_labels.dart';
import 'package:halaqaty_mobile/features/circles/presentation/create_circle_screen.dart';

class CircleDiscoveryScreen extends ConsumerStatefulWidget {
  const CircleDiscoveryScreen({super.key, this.onOpenInvite});

  final VoidCallback? onOpenInvite;

  @override
  ConsumerState<CircleDiscoveryScreen> createState() =>
      _CircleDiscoveryScreenState();
}

class _CircleDiscoveryScreenState extends ConsumerState<CircleDiscoveryScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      if (mounted) {
        final controller = ref.read(circleDiscoveryControllerProvider.notifier);
        await controller.loadMyCircles();
        if (mounted) await controller.discover();
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(circleDiscoveryControllerProvider);
    final rtl = Directionality.of(context) == TextDirection.rtl;
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? 'اكتشاف الحلقات' : 'Discover circles')),
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
              child: TextField(
                key: const Key('circleDiscoverySearchField'),
                textInputAction: TextInputAction.search,
                decoration: InputDecoration(
                  labelText:
                      rtl ? 'ابحث عن حلقة عامة' : 'Search public circles',
                  prefixIcon: const Icon(Icons.search),
                ),
                onSubmitted: (query) => ref
                    .read(circleDiscoveryControllerProvider.notifier)
                    .discover(query: query),
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  FilledButton.icon(
                    key: const Key('openCreateCircleButton'),
                    onPressed: _openCreate,
                    icon: const Icon(Icons.add),
                    label: Text(rtl ? 'إنشاء حلقة' : 'Create circle'),
                  ),
                  const SizedBox(height: 8),
                  OutlinedButton.icon(
                    key: const Key('openInviteJoinButton'),
                    onPressed: widget.onOpenInvite ?? _openInvite,
                    icon: const Icon(Icons.link),
                    label: Text(
                      rtl ? 'لديّ رابط دعوة' : 'I have an invite link',
                    ),
                  ),
                ],
              ),
            ),
            Expanded(child: _content(state, rtl)),
          ],
        ),
      ),
    );
  }

  Widget _content(CircleDiscoveryState state, bool rtl) {
    if (state.isLoading &&
        state.myCircles.isEmpty &&
        state.publicCircles.isEmpty) {
      return const HalaqatyLoading(key: Key('circleDiscoveryLoading'));
    }
    final joinedCircleIds = state.myCircles.map((circle) => circle.id).toSet();
    final publicCircles = state.publicCircles
        .where((circle) => !joinedCircleIds.contains(circle.id))
        .toList();

    return RefreshIndicator(
      onRefresh: _refresh,
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          if (state.failure != null &&
              state.myCircles.isEmpty &&
              state.publicCircles.isEmpty)
            CircleLoadError(
              failure: state.failure!,
              onRetry: _refresh,
            ),
          if (state.myCircles.isNotEmpty) ...[
            Text(
              rtl ? 'حلقاتي' : 'My circles',
              style: Theme.of(context).textTheme.titleLarge,
            ),
            const SizedBox(height: 8),
            ...state.myCircles.map((circle) => _myCircleCard(circle, rtl)),
            const SizedBox(height: 16),
          ],
          Text(
            rtl ? 'الحلقات العامة' : 'Public circles',
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const SizedBox(height: 8),
          if (publicCircles.isEmpty)
            Text(rtl ? 'لا توجد حلقات عامة متاحة' : 'No public circles')
          else
            ...publicCircles.map(
              (circle) => _circleCard(circle, state, rtl),
            ),
        ],
      ),
    );
  }

  Future<void> _refresh() async {
    final controller = ref.read(circleDiscoveryControllerProvider.notifier);
    await controller.loadMyCircles();
    if (mounted) await controller.discover();
  }

  Widget _myCircleCard(CircleSummary circle, bool rtl) {
    return Card(
      child: ListTile(
        key: Key('openCircle-${circle.id}'),
        title: CircleNameText(name: circle.name),
        subtitle: circle.description == null
            ? null
            : Text(
                circle.description!,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
        trailing: const Icon(Icons.chevron_right),
        onTap: () => Navigator.of(context).push(
          MaterialPageRoute<void>(
            builder: (_) => CircleDetailScreen(circleId: circle.id),
          ),
        ),
      ),
    );
  }

  Widget _circleCard(
    CircleSummary circle,
    CircleDiscoveryState state,
    bool rtl,
  ) {
    return Semantics(
      container: true,
      explicitChildNodes: true,
      label: '${rtl ? 'حلقة عامة' : 'Public circle'}: ${circle.name}',
      child: Card(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              CircleNameText(
                name: circle.name,
                maxLines: 2,
                style: Theme.of(context).textTheme.titleLarge,
              ),
              if (circle.description case final description?) ...[
                const SizedBox(height: 6),
                Text(
                  description,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
              const SizedBox(height: 8),
              Text('${rtl ? 'السعة' : 'Capacity'}: ${circle.maxCapacity}'),
              Text(
                '${rtl ? 'اللغة' : 'Language'}: '
                '${circleLanguageLabel(circle.language, rtl)}',
              ),
              Text(_genderText(circle.genderRestriction, rtl)),
              const SizedBox(height: 12),
              FilledButton(
                key: Key('joinCircle-${circle.id}'),
                onPressed: state.joiningCircleId == null
                    ? () => _confirmJoin(circle)
                    : null,
                child: state.joiningCircleId == circle.id
                    ? const SizedBox.square(
                        dimension: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Text(rtl ? 'انضمام' : 'Join'),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _confirmJoin(CircleSummary circle) async {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(rtl ? 'تأكيد الانضمام' : 'Confirm joining'),
        content: Text(
          rtl
              ? 'هل تريد الانضمام إلى ${circle.name}؟'
              : 'Do you want to join ${circle.name}?',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: Text(rtl ? 'إلغاء' : 'Cancel'),
          ),
          FilledButton(
            key: const Key('confirmCircleJoinButton'),
            onPressed: () => Navigator.pop(context, true),
            child: Text(rtl ? 'انضمام' : 'Join'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    final joined = await ref
        .read(circleDiscoveryControllerProvider.notifier)
        .joinPublic(circle);
    // Retained confirmation (FR-008): open the joined circle itself.
    if (joined && mounted) {
      Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (_) => CircleDetailScreen(circleId: circle.id),
        ),
      );
    }
  }

  void _openInvite() {
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => const CircleJoinScreen()),
    );
  }

  Future<void> _openCreate() async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => CreateCircleScreen(onCreated: _openCreatedCircle),
      ),
    );
    // Pick up a circle created (or list changes) while the form was open.
    if (mounted) {
      await ref
          .read(circleDiscoveryControllerProvider.notifier)
          .loadMyCircles();
    }
  }

  void _openCreatedCircle(CircleResponse circle) {
    Navigator.of(context).pushReplacement(
      MaterialPageRoute<void>(
        builder: (_) => CircleDetailScreen(circleId: circle.id),
      ),
    );
  }

  String _genderText(String gender, bool rtl) {
    final label = circleAudienceLabel(gender, rtl);
    return rtl ? 'الفئة: $label' : 'Audience: $label';
  }
}
