import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
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
                state.errorMessage ??
                    (rtl
                        ? SessionUiLabels.unableToConnect
                        : 'Unable to connect'),
                style: TextStyle(color: Theme.of(context).colorScheme.error)),
            const SizedBox(height: 8),
          ],
          if (state.status == SessionRoomStatus.connected)
            Text(
                rtl ? SessionUiLabels.connected : 'Connected. Audio is ready.'),
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
