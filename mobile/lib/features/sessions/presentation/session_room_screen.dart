import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/domain/session_models.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

class SessionRoomScreen extends ConsumerWidget {
  const SessionRoomScreen(
      {super.key, required this.sessionId, this.canStart = false});
  final String sessionId;
  final bool canStart;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(sessionRoomControllerProvider(sessionId));
    final controller =
        ref.read(sessionRoomControllerProvider(sessionId).notifier);
    final rtl = Directionality.of(context) == TextDirection.rtl;
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? SessionUiLabels.title : 'Live session')),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child:
            Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(rtl ? SessionUiLabels.audioOnly : 'Audio-only session',
              style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 16),
          if (state.status == SessionRoomStatus.loading)
            const Center(child: CircularProgressIndicator()),
          if (state.status == SessionRoomStatus.loading)
            Text(rtl
                ? SessionUiLabels.loadingParticipants
                : 'Loading participants...'),
          if (state.status == SessionRoomStatus.error) ...[
            Text(
                rtl
                    ? (state.recovery == SessionRoomRecovery.terminal
                        ? SessionUiLabels.terminalConnectionError
                        : SessionUiLabels.unableToConnect)
                    : (state.recovery == SessionRoomRecovery.terminal
                        ? 'Session access has ended'
                        : 'Connection was interrupted'),
                style: TextStyle(color: Theme.of(context).colorScheme.error)),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              alignment: WrapAlignment.center,
              children: [
                if (state.recovery == SessionRoomRecovery.retryable)
                  OutlinedButton(
                    onPressed: controller.retry,
                    child: Text(rtl ? SessionUiLabels.retry : 'Retry'),
                  ),
                FilledButton(
                  onPressed: controller.leave,
                  child: Text(rtl ? SessionUiLabels.leave : 'Leave'),
                ),
              ],
            ),
          ],
          if (state.status == SessionRoomStatus.connected)
            Text(
                rtl ? SessionUiLabels.connected : 'Connected. Audio is ready.'),
          if (state.status == SessionRoomStatus.ended)
            Text(rtl ? SessionUiLabels.sessionEnded : 'Session ended'),
          // Action failures never render raw errors; the room stays connected.
          if (state.actionErrorMessage != null)
            Text(rtl ? SessionUiLabels.actionFailed : 'Action failed',
                style: TextStyle(color: Theme.of(context).colorScheme.error)),
          if (state.status == SessionRoomStatus.connected) ...[
            const SizedBox(height: 8),
            Text(rtl ? SessionUiLabels.participantsTitle : 'Participants',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 4),
            Expanded(
                child: _ParticipantList(
                    participants: state.participants,
                    isModerator: state.isModerator,
                    rtl: rtl,
                    onMute: controller.muteParticipant,
                    onRemove: controller.removeParticipant)),
            const SizedBox(height: 8),
            _RoomControls(
                state: state,
                rtl: rtl,
                onRaiseHand: controller.raiseHand,
                onLowerHand: controller.lowerHand,
                onToggleLock: () => controller.setLock(!state.isLocked),
                onMuteAll: controller.muteAll,
                onEnd: controller.endSession),
          ],
          const Spacer(),
          FilledButton(
              onPressed: state.status == SessionRoomStatus.loading
                  ? null
                  : () => canStart
                      ? controller.start(sessionId)
                      : controller.join(sessionId),
              child: Text(canStart
                  ? (rtl ? SessionUiLabels.start : 'Start session')
                  : (rtl ? SessionUiLabels.join : 'Join'))),
        ]),
      ),
    );
  }
}

class _ParticipantList extends StatelessWidget {
  const _ParticipantList(
      {required this.participants,
      required this.isModerator,
      required this.rtl,
      required this.onMute,
      required this.onRemove});

  final List<SessionParticipant> participants;
  final bool isModerator;
  final bool rtl;
  final void Function(String userId) onMute;
  final void Function(String userId) onRemove;

  @override
  Widget build(BuildContext context) {
    return ListView(
      children: [
        for (final participant
            in participants.where((p) => p.isCurrentlyPresent))
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: participant.isHandRaised
                ? Semantics(
                    container: true,
                    label: rtl ? SessionUiLabels.handRaised : 'Hand raised',
                    child: Icon(Icons.pan_tool,
                        color: Theme.of(context).colorScheme.primary),
                  )
                : null,
            title: Text(participant.displayName),
            subtitle: Text(_roleLabel(participant.role, rtl)),
            trailing: isModerator
                ? Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      TextButton(
                        onPressed: () => onMute(participant.userId),
                        child: Text(
                            rtl ? SessionUiLabels.muteParticipant : 'Mute'),
                      ),
                      TextButton(
                        onPressed: () => onRemove(participant.userId),
                        child: Text(
                            rtl ? SessionUiLabels.removeParticipant : 'Remove'),
                      ),
                    ],
                  )
                : null,
          ),
      ],
    );
  }

  String _roleLabel(CircleRole role, bool rtl) => switch (role) {
        CircleRole.teacher => rtl ? SessionUiLabels.roleTeacher : 'Teacher',
        CircleRole.supervisor =>
          rtl ? SessionUiLabels.roleSupervisor : 'Supervisor',
        CircleRole.student => rtl ? SessionUiLabels.roleStudent : 'Student',
      };
}

class _RoomControls extends StatelessWidget {
  const _RoomControls(
      {required this.state,
      required this.rtl,
      required this.onRaiseHand,
      required this.onLowerHand,
      required this.onToggleLock,
      required this.onMuteAll,
      required this.onEnd});

  final SessionRoomState state;
  final bool rtl;
  final VoidCallback onRaiseHand;
  final VoidCallback onLowerHand;
  final VoidCallback onToggleLock;
  final VoidCallback onMuteAll;
  final VoidCallback onEnd;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      alignment: WrapAlignment.center,
      children: [
        OutlinedButton(
          onPressed: onRaiseHand,
          child: Text(rtl ? SessionUiLabels.raiseHand : 'Raise hand'),
        ),
        OutlinedButton(
          onPressed: onLowerHand,
          child: Text(rtl ? SessionUiLabels.lowerHand : 'Lower hand'),
        ),
        if (state.isModerator) ...[
          FilledButton.tonal(
            onPressed: onToggleLock,
            child: Text(state.isLocked
                ? (rtl ? SessionUiLabels.unlockSession : 'Unlock session')
                : (rtl ? SessionUiLabels.lockSession : 'Lock session')),
          ),
          FilledButton.tonal(
            onPressed: onMuteAll,
            child: Text(rtl ? SessionUiLabels.muteAll : 'Mute all'),
          ),
          FilledButton.tonal(
            onPressed: onEnd,
            style: FilledButton.styleFrom(
                backgroundColor: Theme.of(context).colorScheme.errorContainer),
            child: Text(rtl ? SessionUiLabels.endSession : 'End session'),
          ),
        ],
      ],
    );
  }
}
