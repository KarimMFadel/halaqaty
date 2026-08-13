import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_management_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_ui_labels.dart';

const _confirmLabel = 'تأكيد';
const _inviteCopiedLabel = 'تم نسخ رابط الدعوة';

class CircleManagementScreen extends ConsumerWidget {
  const CircleManagementScreen({
    super.key,
    required this.circleId,
    required this.currentUserId,
  });

  final String circleId;
  final String currentUserId;

  bool _canManage(CircleRole role) =>
      role == CircleRole.teacher || role == CircleRole.supervisor;

  String _roleLabel(CircleRole role) => switch (role) {
        CircleRole.teacher => 'معلم',
        CircleRole.supervisor => 'مشرف',
        CircleRole.student => 'طالب',
      };

  Future<void> _confirmRole(
    BuildContext context,
    WidgetRef ref,
    CircleMember member,
  ) async {
    final role = await showModalBottomSheet<CircleRole>(
      context: context,
      builder: (context) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: CircleRole.values
              .map((role) => ListTile(
                    key: Key('circleRole-${role.name}'),
                    title: Text(_roleLabel(role)),
                    onTap: () => Navigator.pop(context, role),
                  ))
              .toList(growable: false),
        ),
      ),
    );
    if (role == null || !context.mounted) return;
    await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('تأكيد تغيير الدور'),
        content:
            Text('تغيير دور ${member.displayName} إلى ${_roleLabel(role)}؟'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text(circleCancelLabel)),
          FilledButton(
            key: const Key('confirmCircleRoleChange'),
            onPressed: () async {
              final succeeded = await ref
                  .read(circleManagementControllerProvider)
                  .assignRole(circleId, member.userId, role);
              if (!context.mounted) return;
              if (succeeded) {
                Navigator.pop(context);
              } else {
                _showMutationError(context);
              }
            },
            child: const Text(_confirmLabel),
          ),
        ],
      ),
    );
  }

  Future<void> _confirmRefresh(BuildContext context, WidgetRef ref) =>
      showDialog<void>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('تحديث رابط الدعوة'),
          content: const Text('سيتوقف الرابط السابق عن العمل.'),
          actions: [
            TextButton(
                onPressed: () => Navigator.pop(context),
                child: const Text(circleCancelLabel)),
            FilledButton(
              key: const Key('confirmCircleInviteRefresh'),
              onPressed: () async {
                final succeeded = await ref
                    .read(circleManagementControllerProvider)
                    .refreshInvite(circleId);
                if (!context.mounted) return;
                if (succeeded) {
                  Navigator.pop(context);
                } else {
                  _showMutationError(context);
                }
              },
              child: const Text('تحديث'),
            ),
          ],
        ),
      );

  Future<void> _confirmRemoval(
    BuildContext context,
    WidgetRef ref,
    CircleMember member,
  ) =>
      showDialog<void>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('إزالة عضو'),
          content: Text('إزالة ${member.displayName} من الحلقة؟'),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text(circleCancelLabel),
            ),
            FilledButton(
              key: const Key('confirmCircleMemberRemoval'),
              onPressed: () async {
                final succeeded = await ref
                    .read(circleManagementControllerProvider)
                    .removeMember(circleId, member.userId);
                if (!context.mounted) return;
                if (succeeded) {
                  Navigator.pop(context);
                } else {
                  _showMutationError(context);
                }
              },
              child: const Text('إزالة'),
            ),
          ],
        ),
      );

  void _showMutationError(BuildContext context) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        key: Key('circleMutationError'),
        content: Text(circleMutationErrorLabel),
      ),
    );
  }

  void _shareInvite(BuildContext context, String inviteLink) {
    Clipboard.setData(ClipboardData(text: inviteLink));
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text(_inviteCopiedLabel)),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final circle = ref.watch(circleDetailProvider(circleId));
    final members = ref.watch(circleMembersProvider(circleId));
    return Scaffold(
      appBar: AppBar(title: const Text('إدارة الحلقة')),
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
            final canManage = !circle.isArchived &&
                current != null &&
                _canManage(current.role);
            if (!canManage) {
              return const Center(
                  key: Key('circleManagementDenied'),
                  child: Text('لا تملك صلاحية إدارة الحلقة'));
            }
            return ListView(
              padding: const EdgeInsets.all(16),
              children: [
                if (current.role == CircleRole.teacher) ...[
                  SelectableText(circle.inviteLink,
                      key: const Key('circleInviteLink')),
                  Row(children: [
                    IconButton(
                        key: const Key('shareCircleInvite'),
                        tooltip: 'مشاركة رابط الدعوة',
                        onPressed: () =>
                            _shareInvite(context, circle.inviteLink),
                        icon: const Icon(Icons.share)),
                    IconButton(
                        key: const Key('refreshCircleInvite'),
                        tooltip: 'تحديث رابط الدعوة',
                        onPressed: () => _confirmRefresh(context, ref),
                        icon: const Icon(Icons.refresh)),
                  ]),
                ],
                for (final member in members)
                  ListTile(
                    title: Text(member.displayName),
                    subtitle: Text(_roleLabel(member.role)),
                    trailing: member.userId == currentUserId
                        ? null
                        : Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              IconButton(
                                key: Key('manageRole-${member.userId}'),
                                tooltip: 'إدارة الدور',
                                onPressed: () =>
                                    _confirmRole(context, ref, member),
                                icon: const Icon(Icons.manage_accounts),
                              ),
                              if (current.role == CircleRole.teacher)
                                IconButton(
                                  key: Key('removeMember-${member.userId}'),
                                  tooltip: 'إزالة العضو',
                                  onPressed: () =>
                                      _confirmRemoval(context, ref, member),
                                  icon: const Icon(Icons.person_remove),
                                ),
                            ],
                          ),
                  ),
              ],
            );
          },
        ),
      ),
    );
  }
}
