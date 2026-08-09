import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';

class CircleJoinScreen extends ConsumerStatefulWidget {
  const CircleJoinScreen({super.key});

  @override
  ConsumerState<CircleJoinScreen> createState() => _CircleJoinScreenState();
}

class _CircleJoinScreenState extends ConsumerState<CircleJoinScreen> {
  final _formKey = GlobalKey<FormState>();
  final _invite = TextEditingController();

  @override
  void dispose() {
    _invite.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(circleDiscoveryControllerProvider);
    final rtl = Directionality.of(context) == TextDirection.rtl;
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? 'الانضمام إلى حلقة' : 'Join a circle')),
      body: SafeArea(
        child: Form(
          key: _formKey,
          child: ListView(
            padding: const EdgeInsets.all(24),
            children: [
              Text(
                rtl
                    ? 'أدخل رمز الدعوة أو الرابط الذي شاركه معك المعلّم.'
                    : 'Enter the invite code or link shared by your teacher.',
              ),
              const SizedBox(height: 16),
              Semantics(
                textField: true,
                label: rtl ? 'رمز أو رابط الدعوة' : 'Invite code or link',
                child: TextFormField(
                  key: const Key('circleInviteField'),
                  controller: _invite,
                  textCapitalization: TextCapitalization.characters,
                  autocorrect: false,
                  decoration: InputDecoration(
                    labelText: rtl ? 'رابط الدعوة' : 'Invite link',
                    hintText: 'HLQ-7X2K',
                  ),
                  validator: (value) => ref
                              .read(circleDiscoveryControllerProvider.notifier)
                              .normalizeInvite(value ?? '') ==
                          null
                      ? (rtl
                          ? 'رابط الدعوة غير صالح'
                          : 'The invite link is invalid')
                      : null,
                ),
              ),
              const SizedBox(height: 16),
              ElevatedButton(
                key: const Key('circleInviteSubmitButton'),
                onPressed: state.joiningCircleId == null ? _confirmJoin : null,
                child: state.joiningCircleId == null
                    ? Text(rtl ? 'متابعة' : 'Continue')
                    : const SizedBox.square(
                        dimension: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
              ),
              if (state.failure case final failure?)
                Padding(
                  padding: const EdgeInsets.only(top: 16),
                  child: Semantics(
                    liveRegion: true,
                    child: Text(
                      circleFailureText(failure, rtl),
                      key: const Key('circleJoinError'),
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _confirmJoin() async {
    if (!_formKey.currentState!.validate()) return;
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(rtl ? 'تأكيد الانضمام' : 'Confirm joining'),
        content: Text(
          rtl
              ? 'هل تريد الانضمام باستخدام رابط الدعوة؟'
              : 'Join using this invite link?',
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
        .joinInvite(_invite.text);
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
}

String circleFailureText(CircleJoinFailure failure, bool rtl) {
  if (!rtl) {
    return switch (failure) {
      CircleJoinFailure.invalidInvite => 'The invite link is invalid',
      CircleJoinFailure.alreadyMember => 'You are already a member',
      CircleJoinFailure.full => 'This circle is full',
      CircleJoinFailure.archived =>
        'This circle is archived and available read-only',
      CircleJoinFailure.membershipLimit =>
        'You cannot join more than 5 circles',
      CircleJoinFailure.privateCircle => 'This circle requires an invite link',
      CircleJoinFailure.sessionExpired => 'Please sign in again',
      CircleJoinFailure.network => 'Unable to connect. Try again',
      CircleJoinFailure.unknown => 'Unable to complete the request. Try again',
    };
  }
  return switch (failure) {
    CircleJoinFailure.invalidInvite => 'رابط الدعوة غير صالح',
    CircleJoinFailure.alreadyMember => 'أنت عضو في هذه الحلقة بالفعل',
    CircleJoinFailure.full => 'الحلقة مكتملة السعة',
    CircleJoinFailure.archived =>
      'هذه الحلقة مؤرشفة ومتاحة للقراءة فقط',
    CircleJoinFailure.membershipLimit =>
      'لا يمكنك الانضمام إلى أكثر من 5 حلقات',
    CircleJoinFailure.privateCircle => 'هذه الحلقة خاصة وتتطلب رابط دعوة',
    CircleJoinFailure.sessionExpired => 'انتهت الجلسة. سجّل الدخول مرة أخرى',
    CircleJoinFailure.network => 'تعذر الاتصال. حاول مرة أخرى',
    CircleJoinFailure.unknown => 'تعذر إكمال الطلب. حاول مرة أخرى',
  };
}
