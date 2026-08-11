import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_members_screen.dart';

class CircleDetailScreen extends ConsumerWidget {
  const CircleDetailScreen({super.key, required this.circleId});

  final String circleId;

  String _privacyLabel(bool isPrivate) => isPrivate ? 'خاصة' : 'عامة';

  String _genderLabel(String genderRestriction) {
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

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final circleAsync = ref.watch(circleDetailProvider(circleId));

    return Directionality(
      textDirection: TextDirection.rtl,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('تفاصيل الحلقة'),
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
                    child: const Row(
                      children: [
                        Icon(Icons.archive, color: Colors.amber),
                        SizedBox(width: 8),
                        Expanded(
                          child: Text(
                            'هذه الحلقة مؤرشفة. القراءة فقط متاحة.',
                            style: TextStyle(color: Colors.black87),
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
                        title: const Text('السعة القصوى'),
                        subtitle: Text('${circle.maxCapacity}'),
                      ),
                      ListTile(
                        leading: const Icon(Icons.lock_outline),
                        title: const Text('نوع الحلقة'),
                        subtitle: Text(_privacyLabel(circle.isPrivate)),
                      ),
                      ListTile(
                        leading: const Icon(Icons.wc),
                        title: const Text('الفئة المستهدفة'),
                        subtitle: Text(_genderLabel(circle.genderRestriction)),
                      ),
                      ListTile(
                        leading: const Icon(Icons.language),
                        title: const Text('اللغة'),
                        subtitle: Text(circle.language),
                      ),
                      if (circle.rules != null && circle.rules!.isNotEmpty)
                        ListTile(
                          leading: const Icon(Icons.rule),
                          title: const Text('قواعد الحلقة'),
                          subtitle: Text(circle.rules!),
                        ),
                    ],
                  ),
                ),
                const Divider(height: 32),
                ListTile(
                  leading: const Icon(Icons.people),
                  title: const Text('الأعضاء'),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: () {
                    Navigator.of(context).push(
                      MaterialPageRoute(
                        builder: (_) =>
                            CircleMembersScreen(circleId: circle.id),
                      ),
                    );
                  },
                ),
              ],
            );
          },
          loading: () => const Center(child: CircularProgressIndicator()),
          error: (error, stack) => Center(
            child: Text('حدث خطأ في تحميل بيانات الحلقة'),
          ),
        ),
      ),
    );
  }
}
