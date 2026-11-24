import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_management_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class CircleInvitationManagementScreen extends ConsumerStatefulWidget {
  const CircleInvitationManagementScreen({
    super.key,
    required this.circleId,
    required this.canInviteTeacher,
    required this.canRevokeInvitations,
  });

  final String circleId;
  final bool canInviteTeacher;
  final bool canRevokeInvitations;

  @override
  ConsumerState<CircleInvitationManagementScreen> createState() =>
      _CircleInvitationManagementScreenState();
}

class _CircleInvitationManagementScreenState
    extends ConsumerState<CircleInvitationManagementScreen> {
  final _search = TextEditingController();
  List<CircleUser> _results = const [];
  CircleRole _role = CircleRole.student;
  bool _searching = false;

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final invitations = ref.watch(circleInvitationsProvider(widget.circleId));
    return Scaffold(
      appBar: AppBar(
        title: Text(rtl ? 'دعوات الحلقة' : 'Circle invitations'),
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          TextField(
            key: const Key('circleInvitationSearch'),
            controller: _search,
            decoration: InputDecoration(
              labelText: rtl ? 'ابحث عن مستخدم' : 'Search for a user',
              hintText:
                  rtl ? 'اكتب حرفين على الأقل' : 'Enter at least 2 characters',
              prefixIcon: const Icon(Icons.search),
            ),
            onChanged: _searchUsers,
          ),
          const SizedBox(height: 8),
          DropdownButtonFormField<CircleRole>(
            key: const Key('circleInvitationRole'),
            initialValue: _role,
            decoration: InputDecoration(labelText: rtl ? 'الدور' : 'Role'),
            items: [
              DropdownMenuItem(
                value: CircleRole.student,
                child: Text(rtl ? 'طالب' : 'Student'),
              ),
              if (widget.canInviteTeacher)
                DropdownMenuItem(
                  value: CircleRole.teacher,
                  child: Text(rtl ? 'معلم' : 'Teacher'),
                ),
            ],
            onChanged: (role) {
              if (role != null) setState(() => _role = role);
            },
          ),
          if (_searching)
            const LinearProgressIndicator(
                key: Key('circleInvitationSearching')),
          for (final user in _results)
            ListTile(
              key: Key('circleInvitationUser-${user.id}'),
              title: Text(user.displayName),
              trailing: IconButton(
                tooltip: rtl ? 'إرسال الدعوة' : 'Send invitation',
                icon: const Icon(Icons.send),
                onPressed: () => _sendInvitation(user),
              ),
            ),
          const Divider(height: 32),
          Text(
            rtl ? 'الدعوات المعلقة' : 'Pending invitations',
            style: Theme.of(context).textTheme.titleMedium,
          ),
          invitations.when(
            loading: () => const Center(child: CircularProgressIndicator()),
            error: (_, __) => Text(
              rtl ? 'تعذر تحميل الدعوات' : 'Could not load invitations',
            ),
            data: (pending) => pending.isEmpty
                ? Text(rtl ? 'لا توجد دعوات معلقة' : 'No pending invitations')
                : Column(
                    children: [
                      for (final invitation in pending)
                        ListTile(
                          key: Key('pendingInvitation-${invitation.id}'),
                          title: Text(_roleLabel(invitation.role, rtl)),
                          subtitle: Text(invitation.recipientUserId),
                          trailing: widget.canRevokeInvitations
                              ? IconButton(
                                  key: Key('revokeInvitation-${invitation.id}'),
                                  tooltip: rtl
                                      ? 'إلغاء الدعوة'
                                      : 'Revoke invitation',
                                  icon: const Icon(Icons.cancel_outlined),
                                  onPressed: () =>
                                      _revokeInvitation(invitation),
                                )
                              : null,
                        ),
                    ],
                  ),
          ),
        ],
      ),
    );
  }

  Future<void> _searchUsers(String query) async {
    if (query.trim().length < 2) {
      setState(() {
        _results = const [];
        _searching = false;
      });
      return;
    }
    setState(() => _searching = true);
    final results = await ref
        .read(circleManagementControllerProvider)
        .searchUsers(query.trim());
    if (!mounted || _search.text.trim() != query.trim()) return;
    setState(() {
      _results = results;
      _searching = false;
    });
  }

  Future<void> _sendInvitation(CircleUser user) async {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final sent = await ref
        .read(circleManagementControllerProvider)
        .inviteMember(widget.circleId, user.id, _role);
    if (!mounted) return;
    if (sent) {
      setState(() => _results = const []);
      _search.clear();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(_role == CircleRole.teacher
              ? (rtl
                  ? 'أُرسلت دعوة المعلم؛ يلزم قبولها'
                  : 'Teacher invitation sent; acceptance is required')
              : (rtl ? 'أُرسلت دعوة الطالب' : 'Student invitation sent')),
        ),
      );
    } else {
      _showError(rtl);
    }
  }

  Future<void> _revokeInvitation(CircleInvitation invitation) async {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(rtl ? 'إلغاء الدعوة؟' : 'Revoke invitation?'),
        content: Text(rtl
            ? 'لن يتمكن المستلم من قبول هذه الدعوة.'
            : 'The recipient will no longer be able to accept this invitation.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: Text(rtl ? 'إبقاء' : 'Keep'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: Text(rtl ? 'إلغاء الدعوة' : 'Revoke'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    final revoked = await ref
        .read(circleManagementControllerProvider)
        .revokeInvitation(widget.circleId, invitation.id);
    if (!mounted) return;
    if (!revoked) _showError(rtl);
  }

  void _showError(bool rtl) => ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          key: const Key('circleInvitationError'),
          content: Text(
              rtl ? 'تعذر إكمال العملية' : 'Could not complete the action'),
        ),
      );

  String _roleLabel(CircleRole role, bool rtl) => switch (role) {
        CircleRole.teacher => rtl ? 'دعوة معلم' : 'Teacher invitation',
        CircleRole.student => rtl ? 'دعوة طالب' : 'Student invitation',
        CircleRole.supervisor => rtl ? 'مشرف' : 'Supervisor',
      };
}
