import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class CircleMembersScreen extends ConsumerWidget {
  const CircleMembersScreen({super.key, required this.circleId});

  final String circleId;

  String _getRoleLabel(CircleRole role, bool rtl) {
    if (!rtl) {
      return switch (role) {
        CircleRole.student => 'Student',
        CircleRole.supervisor => 'Supervisor',
        CircleRole.teacher => 'Teacher',
      };
    }
    switch (role) {
      case CircleRole.student:
        return 'طالب';
      case CircleRole.supervisor:
        return 'مشرف';
      case CircleRole.teacher:
        return 'معلم';
    }
  }

  Color _getRoleColor(CircleRole role) {
    switch (role) {
      case CircleRole.student:
        return Colors.blue;
      case CircleRole.supervisor:
        return Colors.green;
      case CircleRole.teacher:
        return Colors.purple;
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final membersAsync = ref.watch(circleMembersProvider(circleId));
    final circleAsync = ref.watch(circleDetailProvider(circleId));
    final isArchived = circleAsync.valueOrNull?.isArchived ?? false;
    final rtl = Directionality.of(context) == TextDirection.rtl;

    return Scaffold(
      appBar: AppBar(
        title: Text(rtl ? 'أعضاء الحلقة' : 'Circle members'),
      ),
      body: Column(
        children: [
            if (isArchived)
              Container(
                key: const Key('circleArchivedReadOnlyBanner'),
                padding: const EdgeInsets.all(12),
                width: double.infinity,
                color: Colors.amber.shade100,
                child: Row(
                  children: [
                    const Icon(Icons.archive, color: Colors.amber),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        rtl
                            ? 'الحلقة مؤرشفة. لا يمكن تعديل الأعضاء.'
                            : 'This circle is archived. Members cannot be changed.',
                        style: const TextStyle(color: Colors.black87),
                      ),
                    ),
                  ],
                ),
              ),
            Expanded(
              child: membersAsync.when(
                data: (members) {
                  if (members.isEmpty) {
                    return Center(
                      child: Text(
                        rtl ? 'لا يوجد أعضاء بعد.' : 'No members yet.',
                      ),
                    );
                  }
                  return ListView.builder(
                    key: const Key('circleMembersList'),
                    itemCount: members.length,
                    itemBuilder: (context, index) {
                      final member = members[index];
                      final roleLabel = _getRoleLabel(member.role, rtl);
                      return ListTile(
                        leading: CircleAvatar(
                          child: Text(
                            member.displayName.isNotEmpty
                                ? member.displayName[0]
                                : '?',
                          ),
                        ),
                        title: Text(member.displayName),
                        subtitle: Text(
                          '${rtl ? 'انضم في' : 'Joined'}: ${member.joinedAt.year}/${member.joinedAt.month}/${member.joinedAt.day}',
                        ),
                        trailing: Semantics(
                          label: rtl
                              ? 'دور العضو ${member.displayName}: $roleLabel'
                              : '${member.displayName} role: $roleLabel',
                          excludeSemantics: true,
                          child: Chip(
                            label: Text(
                              roleLabel,
                              style: const TextStyle(
                                color: Colors.white,
                                fontSize: 12,
                              ),
                            ),
                            backgroundColor: _getRoleColor(member.role),
                          ),
                        ),
                      );
                    },
                  );
                },
                loading: () => const Center(
                  child: CircularProgressIndicator(
                    key: Key('circleMembersLoading'),
                  ),
                ),
                error: (error, stack) => Center(
                  child: Text(rtl
                      ? 'حدث خطأ أثناء تحميل الأعضاء'
                      : 'Could not load circle members'),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
