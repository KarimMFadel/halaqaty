abstract final class SessionApiPaths {
  static const circles = '/circles';
  static const sessions = '/sessions';
  static const realtimeTickets = '/realtime/tickets';
}

abstract final class SessionJsonKeys {
  static const data = 'data';
  static const id = 'id';
  static const circleId = 'circle_id';
  static const status = 'status';
  static const mediaMode = 'media_mode';
  static const participantCount = 'participant_count';
  static const isLocked = 'is_locked';
  static const endReason = 'end_reason';
  static const actualStart = 'actual_start';
  static const actualEnd = 'actual_end';
  static const endpoint = 'endpoint';
  static const credential = 'credential';
  static const expiresAt = 'expires_at';
  static const session = 'session';
  static const mediaConnection = 'media_connection';
  static const isModerator = 'is_moderator';
  static const userId = 'user_id';
  static const displayName = 'display_name';
  static const role = 'role';
  static const isCurrentlyPresent = 'is_currently_present';
  static const handRaisedAt = 'hand_raised_at';
  static const type = 'type';
  static const payload = 'payload';
  static const timestamp = 'timestamp';
  static const eventId = 'event_id';
  static const sessionId = 'session_id';
  static const version = 'version';
  static const queueEntryId = 'queue_entry_id';
  static const entries = 'entries';
  static const policy = 'policy';
  static const participantId = 'participant_id';
  static const participantName = 'participant_name';
  static const locked = 'locked';
}

abstract final class SessionRealtimeTypes {
  static const snapshot = 'session.snapshot';
  static const participantJoined = 'session.participant_joined';
  static const participantLeft = 'session.participant_left';
  static const participantRemoved = 'session.participant_removed';
  static const handRaised = 'session.hand_raised';
  static const handLowered = 'session.hand_lowered';
  static const lockChanged = 'session.lock_changed';
  static const ended = 'session.ended';
  static const queueState = 'queue.state';
  static const queueEntryUpdated = 'queue.entry_updated';
  static const queueYourTurn = 'queue.your_turn';
  static const queueNextSoon = 'queue.next_soon';
  static const queueReordered = 'queue.reordered';
  static const queueRoundStarted = 'queue.round_started';
  static const queueGradeSubmitted = 'queue.grade_submitted';
  static const queueAdvanced = 'queue.advanced';
  static const queueRoundFinalized = 'queue.round_finalized';
  static const queuePolicyChanged = 'queue.policy_changed';
  static const queueOptOutRequested = 'queue.opt_out_requested';
  static const ping = 'ping';
}

abstract final class SessionRealtimeActions {
  static const subscribe = 'subscribe';
  static const raiseHand = 'cmd.raise_hand';
  static const lowerHand = 'cmd.lower_hand';
}

abstract final class SessionHeaders {
  static const authorization = 'Authorization';
  static const sessionId = 'X-Halaqaty-Session-ID';
}
