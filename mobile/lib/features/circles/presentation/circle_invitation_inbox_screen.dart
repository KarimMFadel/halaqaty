import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_management_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class CircleInvitationInboxScreen extends ConsumerWidget {
  const CircleInvitationInboxScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final invitations = ref.watch(myCircleInvitationsProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(rtl ? 'دعوات الحلقات' : 'Circle invitations'),
      ),
      body: invitations.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (_, __) => Center(
          child:
              Text(rtl ? 'تعذر تحميل الدعوات' : 'Could not load invitations'),
        ),
        data: (pending) => pending.isEmpty
            ? Center(
                child: Text(
                    rtl ? 'لا توجد دعوات معلقة' : 'No pending invitations'),
              )
            : ListView.builder(
                itemCount: pending.length,
                itemBuilder: (context, index) {
                  final invitation = pending[index];
                  return Card(
                    margin:
                        const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                    child: ListTile(
                      key: Key('inboxInvitation-${invitation.id}'),
                      title: Text(_roleLabel(invitation.role, rtl)),
                      subtitle: Text(
                        rtl
                            ? 'معرّف الحلقة: ${invitation.circleId}'
                            : 'Circle ID: ${invitation.circleId}',
                      ),
                      trailing: FilledButton(
                        key: Key('acceptInvitation-${invitation.id}'),
                        onPressed: () => _accept(context, ref, invitation, rtl),
                        child: Text(rtl ? 'قبول' : 'Accept'),
                      ),
                    ),
                  );
                },
              ),
      ),
    );
  }

  Future<void> _accept(
    BuildContext context,
    WidgetRef ref,
    CircleInvitation invitation,
    bool rtl,
  ) async {
    final accepted = await ref
        .read(circleManagementControllerProvider)
        .acceptInvitation(invitation.circleId, invitation.id);
    if (!accepted || !context.mounted) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            key: const Key('circleInvitationAcceptError'),
            content:
                Text(rtl ? 'تعذر قبول الدعوة' : 'Could not accept invitation'),
          ),
        );
      }
      return;
    }
    await ref.read(circleDiscoveryControllerProvider.notifier).loadMyCircles();
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(rtl ? 'تم قبول الدعوة' : 'Invitation accepted')),
    );
  }

  String _roleLabel(CircleRole role, bool rtl) => switch (role) {
        CircleRole.teacher =>
          rtl ? 'دعوة لتولي دور المعلم' : 'Teacher invitation',
        CircleRole.student =>
          rtl ? 'دعوة للانضمام كطالب' : 'Student invitation',
        CircleRole.supervisor => rtl ? 'مشرف' : 'Supervisor',
      };
}
