import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_protocol_constants.dart';

/// Provider-neutral realtime session events per `docs/contracts/ws_events.md`.
///
/// Only session-scoped facts are modeled (US2); queue/chat events and
/// `session.participant_muted` (no US2 client state) are dropped by the parser.
abstract class RealtimeSessionEvent {
  const RealtimeSessionEvent({required this.sessionId});
  final String sessionId;
}

/// Authoritative presence/hand snapshot; replaces any derived state.
class SessionSnapshotEvent extends RealtimeSessionEvent {
  const SessionSnapshotEvent(
      {required super.sessionId,
      required this.isLocked,
      required this.participants});
  final bool isLocked;
  final List<SessionParticipant> participants;
}

class ParticipantJoinedEvent extends RealtimeSessionEvent {
  const ParticipantJoinedEvent(
      {required super.sessionId,
      required this.userId,
      required this.displayName,
      required this.role});
  final String userId;
  final String displayName;
  final CircleRole role;
}

class ParticipantLeftEvent extends RealtimeSessionEvent {
  const ParticipantLeftEvent({required super.sessionId, required this.userId});
  final String userId;
}

class ParticipantRemovedEvent extends RealtimeSessionEvent {
  const ParticipantRemovedEvent(
      {required super.sessionId, required this.userId});
  final String userId;
}

class HandRaisedEvent extends RealtimeSessionEvent {
  const HandRaisedEvent(
      {required super.sessionId,
      required this.participantId,
      required this.participantName,
      this.at});
  final String participantId;
  final String participantName;
  final DateTime? at;
}

class HandLoweredEvent extends RealtimeSessionEvent {
  const HandLoweredEvent(
      {required super.sessionId,
      required this.participantId,
      required this.participantName,
      this.at});
  final String participantId;
  final String participantName;
  final DateTime? at;
}

class LockChangedEvent extends RealtimeSessionEvent {
  const LockChangedEvent({required super.sessionId, required this.locked});
  final bool locked;
}

class SessionEndedEvent extends RealtimeSessionEvent {
  const SessionEndedEvent({required super.sessionId, this.endReason});
  final String? endReason;
}

/// A queue projection delivered over the at-least-once session topic.
sealed class QueueRealtimeEvent extends RealtimeSessionEvent {
  const QueueRealtimeEvent({
    required super.sessionId,
    required this.eventId,
    this.version,
  });

  final String eventId;
  final int? version;
}

/// A full, visibility-filtered queue snapshot.
class QueueStateEvent extends QueueRealtimeEvent {
  QueueStateEvent({
    required super.sessionId,
    required super.eventId,
    required this.queue,
  }) : super(version: queue.version);

  final QueueState queue;
}

/// Signals that a queue projection was skipped and REST reconciliation is due.
class QueueVersionGapEvent extends QueueRealtimeEvent {
  const QueueVersionGapEvent({
    required super.sessionId,
    required super.eventId,
    required this.previousVersion,
    required this.receivedVersion,
  }) : super(version: receivedVersion);

  final int previousVersion;
  final int receivedVersion;
}

/// A policy change requires a fresh visibility-filtered queue projection.
class QueuePolicyChangedEvent extends QueueRealtimeEvent {
  const QueuePolicyChangedEvent({
    required super.sessionId,
    required super.eventId,
    required this.policyVersion,
  }) : super(version: policyVersion);

  final int policyVersion;
}

/// A recognized incremental queue projection.
class QueueChangeEvent extends QueueRealtimeEvent {
  const QueueChangeEvent({
    required super.sessionId,
    required super.eventId,
    required super.version,
    required this.type,
  });

  final String type;
}

/// Decodes queue frames and tracks delivery per websocket session.
class QueueRealtimeEventDecoder {
  QueueRealtimeEventDecoder(this._liveSessionId);

  final String _liveSessionId;
  final Set<String> _seenEventIds = {};
  int? _roundVersion;

  RealtimeSessionEvent? decode(String raw) {
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map<String, dynamic>) return null;
      final type = decoded[SessionJsonKeys.type];
      final payload = decoded[SessionJsonKeys.payload];
      final eventId = decoded[SessionJsonKeys.eventId];
      if (type is! String ||
          !_queueTypes.contains(type) ||
          payload is! Map<String, dynamic> ||
          eventId is! String ||
          eventId.isEmpty ||
          payload[SessionJsonKeys.sessionId] != _liveSessionId ||
          !_seenEventIds.add(eventId)) {
        return null;
      }

      if (type == SessionRealtimeTypes.queueState) {
        final queue = QueueState.fromJson(_queueStatePayload(payload));
        _roundVersion = queue.version;
        return QueueStateEvent(
          sessionId: _liveSessionId,
          eventId: eventId,
          queue: queue,
        );
      }

      if (type == SessionRealtimeTypes.queuePolicyChanged) {
        final policy = payload[SessionJsonKeys.policy];
        final version = policy is Map<String, dynamic>
            ? policy[SessionJsonKeys.version]
            : null;
        if (version is! int) return null;
        return QueuePolicyChangedEvent(
          sessionId: _liveSessionId,
          eventId: eventId,
          policyVersion: version,
        );
      }

      final version = payload[SessionJsonKeys.version];
      if (version is! int ||
          (_roundVersion != null && version <= _roundVersion!)) {
        return null;
      }
      final previousVersion = _roundVersion;
      _roundVersion = version;
      if (previousVersion != null && version > previousVersion + 1) {
        return QueueVersionGapEvent(
          sessionId: _liveSessionId,
          eventId: eventId,
          previousVersion: previousVersion,
          receivedVersion: version,
        );
      }
      return QueueChangeEvent(
        sessionId: _liveSessionId,
        eventId: eventId,
        version: version,
        type: type,
      );
    } on FormatException {
      return null;
    } on TypeError {
      return null;
    }
  }

  static const _queueTypes = {
    SessionRealtimeTypes.queueState,
    SessionRealtimeTypes.queueEntryUpdated,
    SessionRealtimeTypes.queueYourTurn,
    SessionRealtimeTypes.queueNextSoon,
    SessionRealtimeTypes.queueReordered,
    SessionRealtimeTypes.queueRoundStarted,
    SessionRealtimeTypes.queueGradeSubmitted,
    SessionRealtimeTypes.queueAdvanced,
    SessionRealtimeTypes.queueRoundFinalized,
    SessionRealtimeTypes.queuePolicyChanged,
    SessionRealtimeTypes.queueOptOutRequested,
  };

  static Map<String, dynamic> _queueStatePayload(
    Map<String, dynamic> payload,
  ) {
    final entries = payload[SessionJsonKeys.entries];
    if (entries is! List) return payload;
    return {
      ...payload,
      SessionJsonKeys.entries: entries.map((entry) {
        if (entry is! Map<String, dynamic>) return entry;
        return {
          ...entry,
          SessionJsonKeys.id: entry[SessionJsonKeys.queueEntryId],
        };
      }).toList(growable: false),
    };
  }
}

/// Parses one WS text frame into a session event for [liveSessionId].
///
/// Returns `null` for non-session events, events of other sessions, and
/// malformed frames (at-least-once delivery tolerates dropped unknown frames).
RealtimeSessionEvent? parseRealtimeSessionEvent(
    String raw, String liveSessionId) {
  final Object? decoded;
  try {
    decoded = jsonDecode(raw);
  } on FormatException {
    return null;
  }
  if (decoded is! Map<String, dynamic>) return null;
  final type = decoded[SessionJsonKeys.type];
  final payload = decoded[SessionJsonKeys.payload];
  if (type is! String || payload is! Map<String, dynamic>) return null;
  final at =
      DateTime.tryParse(decoded[SessionJsonKeys.timestamp] as String? ?? '');

  final String? eventSessionId = _eventSessionId(type, payload);
  if (eventSessionId == null || eventSessionId != liveSessionId) return null;

  switch (type) {
    case SessionRealtimeTypes.snapshot:
      final session = payload[SessionJsonKeys.session];
      if (session is! Map<String, dynamic>) return null;
      return SessionSnapshotEvent(
        sessionId: eventSessionId,
        isLocked: session[SessionJsonKeys.isLocked] as bool? ?? false,
        participants: _participants(payload['participants']),
      );
    case SessionRealtimeTypes.participantJoined:
      return ParticipantJoinedEvent(
        sessionId: eventSessionId,
        userId: payload[SessionJsonKeys.userId] as String,
        displayName: payload[SessionJsonKeys.displayName] as String,
        role: _role(payload[SessionJsonKeys.role]),
      );
    case SessionRealtimeTypes.participantLeft:
      return ParticipantLeftEvent(
          sessionId: eventSessionId,
          userId: payload[SessionJsonKeys.userId] as String);
    case SessionRealtimeTypes.participantRemoved:
      return ParticipantRemovedEvent(
          sessionId: eventSessionId,
          userId: payload[SessionJsonKeys.userId] as String);
    case SessionRealtimeTypes.handRaised:
      return HandRaisedEvent(
        sessionId: eventSessionId,
        participantId: payload[SessionJsonKeys.participantId] as String,
        participantName: payload[SessionJsonKeys.participantName] as String,
        at: at,
      );
    case SessionRealtimeTypes.handLowered:
      return HandLoweredEvent(
        sessionId: eventSessionId,
        participantId: payload[SessionJsonKeys.participantId] as String,
        participantName: payload[SessionJsonKeys.participantName] as String,
        at: at,
      );
    case SessionRealtimeTypes.lockChanged:
      return LockChangedEvent(
          sessionId: eventSessionId,
          locked: payload[SessionJsonKeys.locked] as bool? ?? false);
    case SessionRealtimeTypes.ended:
      return SessionEndedEvent(
          sessionId: eventSessionId,
          endReason: payload[SessionJsonKeys.endReason] as String?);
    default:
      return null;
  }
}

String? _eventSessionId(String type, Map<String, dynamic> payload) {
  if (type == SessionRealtimeTypes.snapshot) {
    final session = payload[SessionJsonKeys.session];
    return session is Map<String, dynamic>
        ? session[SessionJsonKeys.id] as String?
        : null;
  }
  return payload[SessionJsonKeys.sessionId] as String?;
}

List<SessionParticipant> _participants(Object? raw) =>
    (raw as List<dynamic>? ?? const [])
        .whereType<Map<String, dynamic>>()
        .map(SessionParticipant.fromJson)
        .toList(growable: false);

CircleRole _role(Object? raw) => raw is String
    ? CircleRole.values.asNameMap()[raw] ?? CircleRole.student
    : CircleRole.student;

/// Builds the WS endpoint (`GET /api/v1/ws`) from the REST API base URL.
Uri realtimeWebSocketUrl(String apiBaseUrl) {
  final base = Uri.parse(apiBaseUrl);
  final scheme = base.isScheme('https') ? 'wss' : 'ws';
  final path = base.path.endsWith('/')
      ? '${base.path}ws'
      : base.path.isEmpty || base.path == '/'
          ? '/ws'
          : '${base.path}/ws';
  return base.replace(scheme: scheme, path: path);
}

Map<String, String> realtimeSubscribeMessage(String topic) =>
    {'action': SessionRealtimeActions.subscribe, 'topic': topic};

const Map<String, String> realtimePingMessage = {
  SessionJsonKeys.type: SessionRealtimeTypes.ping
};

/// Realtime transport boundary for the session room. The stream never errors:
/// transport failures close the stream; reconnection is owned by US3 (T039+).
abstract interface class RealtimeSessionClient {
  /// Opens an authenticated connection and emits session events for
  /// [liveSessionId] only, after fetching a short-lived realtime ticket.
  Stream<RealtimeSessionEvent> sessionEvents(String liveSessionId,
      {required String token, required String backendSessionId});

  /// Sends `cmd.raise_hand` for [liveSessionId] over the open connection.
  Future<void> raiseHand(String liveSessionId);

  /// Sends `cmd.lower_hand` for [liveSessionId] over the open connection.
  Future<void> lowerHand(String liveSessionId);

  Future<void> dispose();
}

class WebSocketRealtimeSessionClient implements RealtimeSessionClient {
  WebSocketRealtimeSessionClient(this._dio,
      {Duration heartbeatInterval = const Duration(seconds: 30)})
      : _heartbeatInterval = heartbeatInterval;
  final Dio _dio;
  final Duration _heartbeatInterval;
  WebSocket? _socket;
  StreamController<RealtimeSessionEvent>? _events;
  Timer? _heartbeat;

  @override
  Stream<RealtimeSessionEvent> sessionEvents(String liveSessionId,
      {required String token, required String backendSessionId}) {
    final controller = StreamController<RealtimeSessionEvent>();
    _events = controller;
    unawaited(_open(liveSessionId, token, backendSessionId, controller));
    return controller.stream;
  }

  Future<void> _open(
      String liveSessionId,
      String token,
      String backendSessionId,
      StreamController<RealtimeSessionEvent> sink) async {
    try {
      final ticket = await _fetchTicket(token, backendSessionId);
      final url = realtimeWebSocketUrl(_dio.options.baseUrl)
          .replace(queryParameters: {'token': ticket});
      final socket = await WebSocket.connect(url.toString());
      _socket = socket;
      final queueDecoder = QueueRealtimeEventDecoder(liveSessionId);
      socket
          .add(jsonEncode(realtimeSubscribeMessage('session.$liveSessionId')));
      _heartbeat?.cancel();
      _heartbeat = Timer.periodic(_heartbeatInterval, (_) {
        if (_socket == socket) socket.add(jsonEncode(realtimePingMessage));
      });
      socket.listen((data) {
        if (data is! String || sink.isClosed) return;
        final event = parseRealtimeSessionEvent(data, liveSessionId) ??
            queueDecoder.decode(data);
        if (event != null) sink.add(event);
      },
          // ponytail: transport errors close the stream instead of surfacing;
          // reconnect/backoff arrives with US3 (T039+).
          onError: (Object _) {
        _heartbeat?.cancel();
        unawaited(sink.close());
      }, onDone: () {
        _heartbeat?.cancel();
        unawaited(sink.close());
      }, cancelOnError: true);
    } catch (_) {
      // Ticket or socket setup failure: close the stream; the room stays on
      // media + REST snapshots. Reconnect is US3 (T039+).
      await sink.close();
    }
  }

  Future<String> _fetchTicket(String token, String backendSessionId) async {
    final response = await _dio.post<Map<String, dynamic>>(
        SessionApiPaths.realtimeTickets,
        options:
            Options(headers: sessionRequestHeaders(token, backendSessionId)));
    final ticket = response.data?['token'];
    if (ticket is! String || ticket.isEmpty) {
      throw StateError('Realtime ticket response missing token');
    }
    return ticket;
  }

  @override
  Future<void> raiseHand(String liveSessionId) =>
      _sendCommand(SessionRealtimeActions.raiseHand, liveSessionId);

  @override
  Future<void> lowerHand(String liveSessionId) =>
      _sendCommand(SessionRealtimeActions.lowerHand, liveSessionId);

  Future<void> _sendCommand(String type, String liveSessionId) async {
    final socket = _socket;
    if (socket == null) {
      throw StateError('Realtime session is not connected');
    }
    socket.add(jsonEncode({
      SessionJsonKeys.type: type,
      SessionJsonKeys.payload: {SessionJsonKeys.sessionId: liveSessionId}
    }));
  }

  @override
  Future<void> dispose() async {
    _heartbeat?.cancel();
    _heartbeat = null;
    await _events?.close();
    await _socket?.close();
    _socket = null;
  }
}

final realtimeSessionClientProvider = Provider<RealtimeSessionClient>(
    (ref) => WebSocketRealtimeSessionClient(ref.watch(dioProvider)));
