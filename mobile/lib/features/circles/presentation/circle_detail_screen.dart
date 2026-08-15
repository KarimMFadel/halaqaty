import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_management_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_members_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_retirement_screen.dart';

class CircleDetailScreen extends ConsumerWidget {
  const CircleDetailScreen({
    super.key,
    required this.circleId,
    this.currentUserId,
  });

  final String circleId;
  final String? currentUserId;

  String _privacyLabel(bool isPrivate, bool rtl) =>
      isPrivate ? (rtl ? 'خاصة' : 'Private') : (rtl ? 'عامة' : 'Public');

  String _genderLabel(String genderRestriction, bool rtl) {
    if (!rtl) {
      return switch (genderRestriction) {
        'male' => 'Male',
        'female' => 'Female',
        'mixed' => 'Mixed',
        _ => 'Unspecified',
      };
    }
    switch (genderRestriction) {
      case 'male':
        return 'ذكور';
      case 'female':
        return 'إناث';
      case 'mixed':
        return 'مختلطة';
      default:
        return 'غير محدد';
    }
  }

  CircleRole? _currentRole(List<CircleMember> members, String? userId) {
    for (final member in members) {
      if (member.userId == userId) return member.role;
    }
    return null;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final circleAsync = ref.watch(circleDetailProvider(circleId));
    final userId = currentUserId ?? ref.watch(authControllerProvider).user?.id;
    final members = userId == null
        ? const <CircleMember>[]
        : ref.watch(circleMembersProvider(circleId)).valueOrNull ??
            const <CircleMember>[];
    final currentRole = _currentRole(members, userId);
    final rtl = Directionality.of(context) == TextDirection.rtl;

    return Scaffold(
      appBar: AppBar(
        title: Text(rtl ? 'تفاصيل الحلقة' : 'Circle details'),
      ),
      body: circleAsync.when(
        data: (circle) {
          return ListView(
            padding: const EdgeInsets.all(16.0),
            children: [
              if (circle.isArchived)
                Container(
                  padding: const EdgeInsets.all(12),
                  color: Colors.amber.shade100,
                  child: Row(
                    children: [
                      const Icon(Icons.archive, color: Colors.amber),
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          rtl
                              ? 'هذه الحلقة مؤرشفة. القراءة فقط متاحة.'
                              : 'This circle is archived and read-only.',
                          style: const TextStyle(color: Colors.black87),
                        ),
                      ),
                    ],
                  ),
                ),
              const SizedBox(height: 16),
              Text(
                circle.name,
                style: Theme.of(context).textTheme.headlineMedium,
              ),
              if (circle.description != null) ...[
                const SizedBox(height: 8),
                Text(circle.description!),
              ],
              const SizedBox(height: 16),
              Card(
                child: Column(
                  children: [
                    ListTile(
                      leading: const Icon(Icons.group),
                      title: Text(rtl ? 'السعة القصوى' : 'Maximum capacity'),
                      subtitle: Text('${circle.maxCapacity}'),
                    ),
                    ListTile(
                      leading: const Icon(Icons.lock_outline),
                      title: Text(rtl ? 'نوع الحلقة' : 'Circle visibility'),
                      subtitle: Text(_privacyLabel(circle.isPrivate, rtl)),
                    ),
                    ListTile(
                      leading: const Icon(Icons.wc),
                      title: Text(rtl ? 'الفئة المستهدفة' : 'Audience'),
                      subtitle:
                          Text(_genderLabel(circle.genderRestriction, rtl)),
                    ),
                    ListTile(
                      leading: const Icon(Icons.language),
                      title: Text(rtl ? 'اللغة' : 'Language'),
                      subtitle: Text(circle.language),
                    ),
                    if (circle.rules != null && circle.rules!.isNotEmpty)
                      ListTile(
                        leading: const Icon(Icons.rule),
                        title: Text(rtl ? 'قواعد الحلقة' : 'Circle rules'),
                        subtitle: Text(circle.rules!),
                      ),
                  ],
                ),
              ),
              const Divider(height: 32),
              ListTile(
                leading: const Icon(Icons.people),
                title: Text(rtl ? 'الأعضاء' : 'Members'),
                trailing: Icon(rtl ? Icons.chevron_left : Icons.chevron_right),
                onTap: () {
                  Navigator.of(context).push(
                    MaterialPageRoute(
                      builder: (_) => CircleMembersScreen(circleId: circle.id),
                    ),
                  );
                },
              ),
              if (!circle.isArchived &&
                  userId != null &&
                  (currentRole == CircleRole.teacher ||
                      currentRole == CircleRole.supervisor))
                ListTile(
                  key: const Key('openCircleManagement'),
                  leading: const Icon(Icons.manage_accounts),
                  title: Text(rtl ? 'إدارة الحلقة' : 'Manage circle'),
                  onTap: () => Navigator.of(context).push(
                    MaterialPageRoute(
                      builder: (_) => CircleManagementScreen(
                        circleId: circle.id,
                        currentUserId: userId,
                      ),
                    ),
                  ),
                ),
              if (!circle.isArchived &&
                  userId != null &&
                  currentRole == CircleRole.teacher)
                ListTile(
                  key: const Key('openCircleRetirement'),
                  leading: const Icon(Icons.archive_outlined),
                  title: Text(rtl ? 'أرشفة الحلقة' : 'Archive circle'),
                  onTap: () => Navigator.of(context).push(
                    MaterialPageRoute(
                      builder: (_) => CircleRetirementScreen(
                        circleId: circle.id,
                        currentUserId: userId,
                      ),
                    ),
                  ),
                ),
            ],
          );
        },
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, stack) => Center(
          child: Text(
            rtl
                ? 'حدث خطأ في تحميل بيانات الحلقة'
                : 'Could not load circle details',
          ),
        ),
      ),
    );
  }
}
