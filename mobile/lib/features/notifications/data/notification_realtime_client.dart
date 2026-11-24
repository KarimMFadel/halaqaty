import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/notifications/domain/notification_models.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_protocol_constants.dart';

/// Note: this module may import session realtime plumbing (URL/ticket/ping
/// helpers) but never `livekit_client`; media remains ADR-015-bound.

/// App-level authenticated transport for the user-targeted
/// `notification.updated` event (`docs/contracts/ws_events.md`).
///
/// Uses the shared realtime ticket endpoint and WebSocket — no second
/// transport. Reconnects internally (delivery may be missed/duplicated, so
/// every re-established connection emits `NotificationRealtimeEvent
/// .reconnected()` so the controller reconciles with authoritative REST).
/// No topic subscription is sent: server-side `SendToUsers` reaches every
/// authenticated connection of the user regardless of subscriptions.
abstract interface class NotificationRealtimeClient {
  /// Opens the user-scoped connection and never completes with an error;
  /// reconnection is internal. Cancel the returned stream subscription to
  /// stop the transport.
  Stream<NotificationRealtimeEvent> events(NotificationCredentials credentials);

  Future<void> dispose();
}

class WebSocketNotificationRealtimeClient
    implements NotificationRealtimeClient {
  WebSocketNotificationRealtimeClient(this._dio,
      {Duration heartbeatInterval = const Duration(seconds: 30)})
      : _heartbeatInterval = heartbeatInterval;

  final Dio _dio;
  final Duration _heartbeatInterval;
  final Set<_NotificationConnection> _connections = {};

  @override
  Stream<NotificationRealtimeEvent> events(
      NotificationCredentials credentials) {
    final connection = _NotificationConnection(_connections.remove);
    _connections.add(connection);
    final controller =
        StreamController<NotificationRealtimeEvent>(onCancel: connection.close);
    connection.sink = controller;
    unawaited(_open(credentials, connection));
    return controller.stream;
  }

  Future<void> _open(
    NotificationCredentials credentials,
    _NotificationConnection connection,
  ) async {
    var delay = const Duration(seconds: 1);
    var reconnects = 0;
    while (!connection.isClosed) {
      try {
        final ticket = await _fetchTicket(credentials);
        final url = realtimeWebSocketUrl(_dio.options.baseUrl)
            .replace(queryParameters: {'token': ticket});
        final socket = await WebSocket.connect(url.toString());
        if (connection.isClosed) {
          await socket.close();
          return;
        }
        connection.socket = socket;
        final isReconnect = reconnects++ > 0;
        if (isReconnect) {
          connection.sink.add(const NotificationRealtimeEvent.reconnected());
        }
        // Close detection uses the socket listener, not `await socket.done`:
        // the listener's onDone fires reliably on server-initiated closes,
        // while socket.done can stall after a close handshake it did not
        // start (observed on Windows).
        final closed = Completer<void>();
        socket.listen((data) {
          if (data is! String || connection.sink.isClosed) return;
          final event = decodeNotificationEvent(data);
          if (event != null) connection.sink.add(event);
        }, onError: (_) {
          if (!closed.isCompleted) closed.complete();
        }, onDone: () {
          if (!closed.isCompleted) closed.complete();
        });
        connection.heartbeat = Timer.periodic(_heartbeatInterval, (_) {
          if (!connection.isClosed) {
            socket.add(jsonEncode(realtimePingMessage));
          }
        });
        delay = const Duration(seconds: 1);
        await closed.future;
        connection.heartbeat?.cancel();
        connection.heartbeat = null;
        connection.socket = null;
      } on DioException catch (error) {
        // A rejected ticket means the session is revoked/unauthorized:
        // reconnecting would retry forever against a terminal state. End the
        // stream; REST paths surface the session loss through 401 handling.
        final status = error.response?.statusCode;
        if (status == 401 || status == 403) {
          await connection.close();
          return;
        }
      } catch (_) {
        // ponytail: bounded backoff loop; terminal session loss is caught as
        // 401/403 above so every other failure remains a transient retry.
      }
      if (connection.isClosed) return;
      await Future<void>.delayed(delay);
      if (delay.inSeconds < 4) delay *= 2;
    }
  }

  Future<String> _fetchTicket(NotificationCredentials credentials) async {
    final response = await _dio.post<Map<String, dynamic>>(
      SessionApiPaths.realtimeTickets,
      options: Options(
          headers: sessionRequestHeaders(
        credentials.firebaseIdToken,
        credentials.sessionId,
      )),
    );
    final ticket = response.data?['token'];
    if (ticket is! String || ticket.isEmpty) {
      throw StateError('Realtime ticket response missing token');
    }
    return ticket;
  }

  @override
  Future<void> dispose() async {
    for (final connection in _connections.toList()) {
      await connection.close();
    }
  }
}

/// Decodes one frame of the `notification.updated` contract envelope.
///
/// Unknown event types and malformed frames return null (not errors): the
/// transport carries other users' events and the contract's state hint must
/// never break passive listening.
const _notificationUpdatedEventType = 'notification.updated';

NotificationRealtimeEvent? decodeNotificationEvent(String raw) {
  final Object? decoded;
  try {
    decoded = jsonDecode(raw);
  } on FormatException {
    return null;
  }
  if (decoded is! Map<String, dynamic>) return null;
  if (decoded['type'] != _notificationUpdatedEventType) return null;
  final payload = decoded['payload'];
  if (payload is! Map<String, dynamic>) return null;
  final count = payload['unread_count'];
  final unreadCount = switch (count) {
    int value when value >= 0 => value,
    double value when value >= 0 => value.toInt(),
    _ => null,
  };
  if (unreadCount == null) return null;
  final notificationId = payload['notification_id'];
  return NotificationRealtimeEvent(
    unreadCount: unreadCount,
    notificationId: notificationId is String ? notificationId : null,
  );
}

/// One user connection: its own socket, heartbeat, and stream sink, with
/// idempotent teardown from any side (subscriber cancel, transport done,
/// ticket failure, or client dispose). Mirrors the chat connection shape.
class _NotificationConnection {
  _NotificationConnection(void Function(_NotificationConnection) onRemoved)
      : _onRemoved = onRemoved;

  final void Function(_NotificationConnection) _onRemoved;
  late final StreamController<NotificationRealtimeEvent> sink;
  WebSocket? socket;
  Timer? heartbeat;
  bool _closed = false;

  bool get isClosed => _closed;

  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    _onRemoved(this);
    heartbeat?.cancel();
    heartbeat = null;
    await sink.close();
    final socket = this.socket;
    this.socket = null;
    await socket?.close();
  }
}

final notificationRealtimeClientProvider = Provider<NotificationRealtimeClient>(
    (ref) => WebSocketNotificationRealtimeClient(ref.watch(dioProvider)));
