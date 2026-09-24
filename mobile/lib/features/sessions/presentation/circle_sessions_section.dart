import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/sessions/application/circle_sessions_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/circle_session_ui_labels.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';

/// Circle detail section exposing the existing F-005 ad-hoc session
/// list/create/start/join entry points (FR-030). Archived circles are
/// read-only; a failed reload retains the last list with actions paused.
class CircleSessionsSection extends ConsumerStatefulWidget {
  const CircleSessionsSection({
    super.key,
    required this.circleId,
    required this.isManager,
    required this.isArchived,
  });

  final String circleId;

  /// Teacher/supervisor: may create, and may start scheduled sessions.
  final bool isManager;
  final bool isArchived;

  @override
  ConsumerState<CircleSessionsSection> createState() =>
      _CircleSessionsSectionState();
}

class _CircleSessionsSectionState extends ConsumerState<CircleSessionsSection> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        ref
            .read(circleSessionsControllerProvider(widget.circleId).notifier)
            .load();
      }
    });
  }

  static bool _isVisible(SessionModel session) =>
      session.status == 'scheduled' || session.status == 'active';

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(circleSessionsControllerProvider(widget.circleId));
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final sessions = state.sessions.where(_isVisible).toList(growable: false);
    final degraded =
        state.status == CircleSessionsStatus.error && sessions.isNotEmpty;
    final canMutate = !widget.isArchived && !degraded;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SectionHeader(
          title: rtl
              ? CircleSessionUiLabels.sectionTitleAr
              : CircleSessionUiLabels.sectionTitleEn,
          action: widget.isManager && canMutate
              ? OutlinedButton.icon(
                  key: const Key('circleSessionCreate'),
                  onPressed: state.isCreating ? null : _create,
                  icon: state.isCreating
                      ? const SizedBox(
                          width: 16,
                          height: 16,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.add),
                  label: Text(
                    rtl
                        ? CircleSessionUiLabels.createAr
                        : CircleSessionUiLabels.createEn,
                  ),
                )
              : null,
        ),
        if (widget.isArchived)
          Text(
            rtl
                ? CircleSessionUiLabels.archivedHintAr
                : CircleSessionUiLabels.archivedHintEn,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        if (degraded) ...[
          Text(
            state.failure == CircleSessionsFailure.network
                ? (rtl
                    ? CircleSessionUiLabels.offlineAr
                    : CircleSessionUiLabels.offlineEn)
                : (rtl
                    ? CircleSessionUiLabels.loadErrorAr
                    : CircleSessionUiLabels.loadErrorEn),
            style: Theme.of(context)
                .textTheme
                .bodyMedium
                ?.copyWith(color: Theme.of(context).colorScheme.error),
          ),
          const SizedBox(height: 8),
          _RetryButton(rtl: rtl, onRetry: _reload),
          const SizedBox(height: 8),
        ],
        if (state.status == CircleSessionsStatus.loading && sessions.isEmpty)
          const HalaqatyLoading(key: Key('circleSessionsLoading'))
        else if (state.status == CircleSessionsStatus.error && sessions.isEmpty)
          _SessionsLoadError(failure: state.failure, rtl: rtl, onRetry: _reload)
        else if (sessions.isEmpty)
          EmptyStateCard(
            key: const Key('circleSessionsEmpty'),
            title: rtl
                ? CircleSessionUiLabels.emptyTitleAr
                : CircleSessionUiLabels.emptyTitleEn,
            hint: widget.isManager
                ? (rtl
                    ? CircleSessionUiLabels.emptyManagerHintAr
                    : CircleSessionUiLabels.emptyManagerHintEn)
                : (rtl
                    ? CircleSessionUiLabels.emptyMemberHintAr
                    : CircleSessionUiLabels.emptyMemberHintEn),
          )
        else
          ...sessions.map((session) => _SessionTile(
                session: session,
                rtl: rtl,
                enabled: canMutate,
                canStart: widget.isManager && session.status == 'scheduled',
              )),
      ],
    );
  }

  Future<void> _reload() => ref
      .read(circleSessionsControllerProvider(widget.circleId).notifier)
      .load();

  Future<void> _create() async {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final created = await ref
        .read(circleSessionsControllerProvider(widget.circleId).notifier)
        .create();
    if (!mounted) return;
    if (created == null) {
      showHalaqatyError(
        context,
        rtl
            ? CircleSessionUiLabels.createFailedAr
            : CircleSessionUiLabels.createFailedEn,
      );
      return;
    }
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => SessionRoomScreen(
          sessionId: created.id,
          canStart: created.status == 'scheduled',
        ),
      ),
    );
  }
}

class _SessionTile extends StatelessWidget {
  const _SessionTile({
    required this.session,
    required this.rtl,
    required this.enabled,
    required this.canStart,
  });

  final SessionModel session;
  final bool rtl;
  final bool enabled;
  final bool canStart;

  @override
  Widget build(BuildContext context) {
    final active = session.status == 'active';
    return Card(
      child: ListTile(
        key: Key('circleSession-${session.id}'),
        enabled: enabled,
        leading: Icon(active ? Icons.volume_up : Icons.schedule),
        title: Text(
          active
              ? (rtl
                  ? CircleSessionUiLabels.activeAr
                  : CircleSessionUiLabels.activeEn)
              : (rtl
                  ? CircleSessionUiLabels.scheduledAr
                  : CircleSessionUiLabels.scheduledEn),
        ),
        subtitle: Text(
          rtl
              ? CircleSessionUiLabels.participantsAr(session.participantCount)
              : CircleSessionUiLabels.participantsEn(session.participantCount),
        ),
        trailing: const Icon(Icons.chevron_right),
        onTap: enabled
            ? () => Navigator.of(context).push(
                  MaterialPageRoute<void>(
                    builder: (_) => SessionRoomScreen(
                      sessionId: session.id,
                      canStart: canStart,
                    ),
                  ),
                )
            : null,
      ),
    );
  }
}

class _SessionsLoadError extends StatelessWidget {
  const _SessionsLoadError({
    required this.failure,
    required this.rtl,
    required this.onRetry,
  });

  final CircleSessionsFailure? failure;
  final bool rtl;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final permission = failure == CircleSessionsFailure.permission;
    return Card(
      key: const Key('circleSessionsError'),
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(
              permission ? Icons.lock_outline : Icons.cloud_off,
              color: Theme.of(context).colorScheme.error,
            ),
            const SizedBox(height: 12),
            Text(
              permission
                  ? (rtl
                      ? CircleSessionUiLabels.permissionAr
                      : CircleSessionUiLabels.permissionEn)
                  : (rtl
                      ? CircleSessionUiLabels.loadErrorAr
                      : CircleSessionUiLabels.loadErrorEn),
              textAlign: TextAlign.center,
            ),
            if (!permission) ...[
              const SizedBox(height: 12),
              _RetryButton(rtl: rtl, onRetry: onRetry),
            ],
          ],
        ),
      ),
    );
  }
}

class _RetryButton extends StatelessWidget {
  const _RetryButton({required this.rtl, required this.onRetry});

  final bool rtl;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return OutlinedButton.icon(
      key: const Key('circleSessionsRetry'),
      onPressed: onRetry,
      icon: const Icon(Icons.refresh),
      label: Text(
        rtl ? CircleSessionUiLabels.retryAr : CircleSessionUiLabels.retryEn,
      ),
    );
  }
}
