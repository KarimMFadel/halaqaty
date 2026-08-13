import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_join_screen.dart';

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
              child: OutlinedButton.icon(
                key: const Key('openInviteJoinButton'),
                onPressed: widget.onOpenInvite ?? _openInvite,
                icon: const Icon(Icons.link),
                label: Text(
                  rtl ? 'لديّ رابط دعوة' : 'I have an invite link',
                ),
              ),
            ),
            if (state.failure case final failure?)
              Padding(
                padding: const EdgeInsets.all(16),
                child: Semantics(
                  liveRegion: true,
                  child: Text(
                    circleFailureText(failure, rtl),
                    key: const Key('circleDiscoveryError'),
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.error,
                    ),
                  ),
                ),
              ),
            Expanded(child: _content(state, rtl)),
          ],
        ),
      ),
    );
  }

  Widget _content(CircleDiscoveryState state, bool rtl) {
    if (state.isLoading) {
      return const Center(
        child: CircularProgressIndicator(
          key: Key('circleDiscoveryLoading'),
        ),
      );
    }
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
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
        if (state.publicCircles.isEmpty)
          Text(rtl ? 'لا توجد حلقات عامة متاحة' : 'No public circles')
        else
          ...state.publicCircles.map(
            (circle) => _circleCard(circle, state, rtl),
          ),
      ],
    );
  }

  Widget _myCircleCard(CircleSummary circle, bool rtl) {
    return Card(
      child: ListTile(
        key: Key('openCircle-${circle.id}'),
        title: Text(circle.name),
        subtitle: circle.description == null ? null : Text(circle.description!),
        trailing: Icon(rtl ? Icons.chevron_left : Icons.chevron_right),
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
              Text(circle.name, style: Theme.of(context).textTheme.titleLarge),
              if (circle.description case final description?) ...[
                const SizedBox(height: 6),
                Text(description),
              ],
              const SizedBox(height: 8),
              Text('${rtl ? 'السعة' : 'Capacity'}: ${circle.maxCapacity}'),
              Text('${rtl ? 'اللغة' : 'Language'}: ${circle.language}'),
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
    if (joined && mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            rtl ? 'تم الانضمام إلى الحلقة' : 'Joined the circle',
          ),
        ),
      );
    }
  }

  void _openInvite() {
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => const CircleJoinScreen()),
    );
  }

  String _genderText(String gender, bool rtl) {
    if (!rtl) return 'Audience: $gender';
    return switch (gender) {
      'male' => 'الفئة: ذكور',
      'female' => 'الفئة: إناث',
      'mixed' => 'الفئة: مختلط',
      _ => 'الفئة: غير محدد',
    };
  }
}
