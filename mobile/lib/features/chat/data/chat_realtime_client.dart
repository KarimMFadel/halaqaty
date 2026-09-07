import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_protocol_constants.dart';

/// A chat event from the shared authenticated realtime transport.
sealed class ChatRealtimeEvent {
  const ChatRealtimeEvent({required this.eventId});

  final String eventId;
}

/// A durable `chat.message` projection for the subscribed circle.
class ChatMessageEvent extends ChatRealtimeEvent {
  const ChatMessageEvent({required super.eventId, required this.message});

  final ChatMessage message;
}

/// Any other event type (or an unreadable `chat.message` frame). Callers
/// reconcile against authoritative REST history instead of guessing.
class ChatUnknownEvent extends ChatRealtimeEvent {
  const ChatUnknownEvent({required super.eventId, required this.type});

  final String type;
}

/// Decodes chat frames for one circle topic, deduplicating at-least-once
/// delivery by `event_id` (message-id deduplication is owned by the
/// controller, which also sees REST history).
class ChatRealtimeEventDecoder {
  ChatRealtimeEventDecoder(this._circleId);

  final String _circleId;
  final Set<String> _seenEventIds = {};

  ChatRealtimeEvent? decode(String raw) {
    final Object? decoded;
    try {
      decoded = jsonDecode(raw);
    } on FormatException {
      return null;
    }
    if (decoded is! Map<String, dynamic>) return null;
    final type = decoded[ChatJsonKeys.type];
    final eventId = decoded[ChatJsonKeys.eventId];
    if (type is! String || eventId is! String || eventId.isEmpty) {
      return null;
    }
    if (!_seenEventIds.add(eventId)) return null;

    if (type != ChatRealtimeTypes.message) {
      return ChatUnknownEvent(eventId: eventId, type: type);
    }
    final payload = decoded[ChatJsonKeys.payload];
    if (payload is! Map<String, dynamic>) {
      return ChatUnknownEvent(eventId: eventId, type: type);
    }
    if (payload[ChatJsonKeys.circleId] != _circleId) return null;
    try {
      return ChatMessageEvent(
        eventId: eventId,
        message: ChatMessage.fromJson(payload),
      );
    } on FormatException {
      return ChatUnknownEvent(eventId: eventId, type: type);
    } on TypeError {
      return ChatUnknownEvent(eventId: eventId, type: type);
    }
  }
}

/// Chat boundary over the EXISTING realtime plumbing: the same ticket
/// endpoint, WebSocket protocol, subscribe/ping messages, and URL builder as
/// the sessions feature. This adds no second transport.
abstract interface class ChatRealtimeClient {
  /// Opens an authenticated connection and emits chat events for
  /// [circleId]'s `circle.{circleId}` topic only.
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
      {required String token, required String backendSessionId});

  Future<void> dispose();
}

class WebSocketChatRealtimeClient implements ChatRealtimeClient {
  WebSocketChatRealtimeClient(this._dio,
      {Duration heartbeatInterval = const Duration(seconds: 30)})
      : _heartbeatInterval = heartbeatInterval;

  final Dio _dio;
  final Duration _heartbeatInterval;
  final Set<_ChatSocketConnection> _connections = {};

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
      {required String token, required String backendSessionId}) {
    // Each call gets its own connection state: overlapping circles can
    // never clobber each other's socket, heartbeat, or teardown.
    final connection = _ChatSocketConnection(_connections.remove);
    _connections.add(connection);
    final controller =
        StreamController<ChatRealtimeEvent>(onCancel: connection.close);
    connection.sink = controller;
    unawaited(_open(circleId, token, backendSessionId, connection));
    return controller.stream;
  }

  Future<void> _open(String circleId, String token, String backendSessionId,
      _ChatSocketConnection connection) async {
    try {
      final ticket = await _fetchTicket(token, backendSessionId);
      final url = realtimeWebSocketUrl(_dio.options.baseUrl)
          .replace(queryParameters: {'token': ticket});
      final socket = await WebSocket.connect(url.toString());
      if (connection.isClosed) {
        // The subscriber left while connecting: never leak the socket.
        await socket.close();
        return;
      }
      connection.socket = socket;
      final decoder = ChatRealtimeEventDecoder(circleId);
      socket.listen((data) {
        if (data is! String || connection.sink.isClosed) return;
        final event = decoder.decode(data);
        if (event != null) connection.sink.add(event);
      },
          // ponytail: transport failures close the stream instead of
          // surfacing; reconnect/backoff arrives with US2 (T039/T040).
          onError: (Object _) {
        unawaited(connection.close());
      }, onDone: () {
        unawaited(connection.close());
      }, cancelOnError: true);
      socket.add(jsonEncode(realtimeSubscribeMessage('circle.$circleId')));
      connection.heartbeat = Timer.periodic(_heartbeatInterval, (_) {
        socket.add(jsonEncode(realtimePingMessage));
      });
    } catch (_) {
      // Ticket or socket setup failure: close the stream; the controller
      // keeps authoritative REST history. Reconnect is US2 (T039/T040).
      await connection.close();
    }
  }

  Future<String> _fetchTicket(String token, String backendSessionId) async {
    final response = await _dio.post<Map<String, dynamic>>(
      SessionApiPaths.realtimeTickets,
      options: Options(headers: sessionRequestHeaders(token, backendSessionId)),
    );
    final ticket = response.data?['token'];
    if (ticket is! String || ticket.isEmpty) {
      throw StateError('Realtime ticket response missing token');
    }
    return ticket;
  }

  @override
  Future<void> dispose() async {
    // Copy: close() removes connections from the set while iterating.
    for (final connection in _connections.toList()) {
      await connection.close();
    }
  }
}

/// One circle connection: its own socket, heartbeat, and stream sink, with
/// idempotent teardown no matter which side ends it (subscriber cancel,
/// transport error/done, ticket failure, or client dispose). Closed
/// connections remove themselves from the owning client.
class _ChatSocketConnection {
  _ChatSocketConnection(void Function(_ChatSocketConnection) onRemoved)
      : _onRemoved = onRemoved;

  final void Function(_ChatSocketConnection) _onRemoved;
  late final StreamController<ChatRealtimeEvent> sink;
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

final chatRealtimeClientProvider = Provider<ChatRealtimeClient>(
    (ref) => WebSocketChatRealtimeClient(ref.watch(dioProvider)));
