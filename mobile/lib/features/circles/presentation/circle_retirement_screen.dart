import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_retirement_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_ui_labels.dart';

const _archiveTitle = 'أرشفة الحلقة';
const _archiveDetail =
    'ستبقى السجلات والتقارير محفوظة، لكن لن تقبل الحلقة نشاطاً جديداً.';
const _archiveLabel = 'أرشفة';

class CircleRetirementScreen extends ConsumerWidget {
  const CircleRetirementScreen({
    super.key,
    required this.circleId,
    required this.currentUserId,
  });

  final String circleId;
  final String currentUserId;

  Future<void> _confirmArchive(BuildContext context, WidgetRef ref) =>
      showDialog<void>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text(_archiveTitle),
          content: const Text(_archiveDetail),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text(circleCancelLabel),
            ),
            FilledButton(
              key: const Key('confirmCircleArchive'),
              onPressed: () async {
                final succeeded = await ref
                    .read(circleRetirementControllerProvider)
                    .archive(circleId);
                if (!context.mounted) return;
                if (succeeded) {
                  Navigator.pop(context);
                } else {
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(
                      key: Key('circleMutationError'),
                      content: Text(circleMutationErrorLabel),
                    ),
                  );
                }
              },
              child: const Text(_archiveLabel),
            ),
          ],
        ),
      );

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final circle = ref.watch(circleDetailProvider(circleId));
    final members = ref.watch(circleMembersProvider(circleId));
    return Scaffold(
      appBar: AppBar(title: const Text(_archiveTitle)),
      body: circle.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (_, __) => const Center(child: Text('تعذر تحميل الحلقة')),
        data: (circle) => members.when(
          loading: () => const Center(child: CircularProgressIndicator()),
          error: (_, __) => const Center(child: Text('تعذر تحميل الأعضاء')),
          data: (members) {
            final current = members
                .where((member) => member.userId == currentUserId)
                .firstOrNull;
            if (circle.isArchived) {
              return const Center(
                  child: Text('الحلقة مؤرشفة ومتاحة للقراءة فقط'));
            }
            if (current?.role != CircleRole.teacher) {
              return const Center(child: Text('لا تملك صلاحية أرشفة الحلقة'));
            }
            return Center(
              child: FilledButton.tonalIcon(
                key: const Key('archiveCircleButton'),
                onPressed: () => _confirmArchive(context, ref),
                icon: const Icon(Icons.archive),
                label: const Text(_archiveLabel),
              ),
            );
          },
        ),
      ),
    );
  }
}
