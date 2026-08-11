import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class CircleMembersScreen extends ConsumerWidget {
  const CircleMembersScreen({super.key, required this.circleId});

  final String circleId;

  String _getRoleLabel(CircleRole role) {
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

    return Directionality(
      textDirection: TextDirection.rtl,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('أعضاء الحلقة'),
        ),
        body: Column(
          children: [
            if (isArchived)
              Container(
                key: const Key('circleArchivedReadOnlyBanner'),
                padding: const EdgeInsets.all(12),
                width: double.infinity,
                color: Colors.amber.shade100,
                child: const Row(
                  children: [
                    Icon(Icons.archive, color: Colors.amber),
                    SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        'الحلقة مؤرشفة. لا يمكن تعديل الأعضاء.',
                        style: TextStyle(color: Colors.black87),
                      ),
                    ),
                  ],
                ),
              ),
            Expanded(
              child: membersAsync.when(
                data: (members) {
                  if (members.isEmpty) {
                    return const Center(child: Text('لا يوجد أعضاء بعد.'));
                  }
                  return ListView.builder(
                    key: const Key('circleMembersList'),
                    itemCount: members.length,
                    itemBuilder: (context, index) {
                      final member = members[index];
                      final roleLabel = _getRoleLabel(member.role);
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
                          'انضم في: ${member.joinedAt.year}/${member.joinedAt.month}/${member.joinedAt.day}',
                        ),
                        trailing: Semantics(
                          label: 'دور العضو ${member.displayName}: $roleLabel',
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
                  child: Text(
                    kDebugMode
                        ? 'حدث خطأ أثناء تحميل الأعضاء: $error'
                        : 'حدث خطأ أثناء تحميل الأعضاء',
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
