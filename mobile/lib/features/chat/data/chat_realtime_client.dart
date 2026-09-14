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

/// A targeted sender notification that one recipient read a message.
class ChatMessageReadEvent extends ChatRealtimeEvent {
  const ChatMessageReadEvent(
      {required super.eventId,
      required this.messageId,
      required this.readerId,
      required this.readAt});
  final String messageId;
  final String readerId;
  final DateTime readAt;
}

/// An ephemeral typing projection. Consumers must expire it locally.
class ChatTypingEvent extends ChatRealtimeEvent {
  const ChatTypingEvent(
      {required super.eventId,
      required this.userId,
      this.circleId,
      this.dmPeerId,
      required this.isTyping,
      required this.expiresAt});
  final String userId;
  final String? circleId;
  final String? dmPeerId;
  final bool isTyping;
  final DateTime expiresAt;
}

/// Any other event type (or an unreadable `chat.message` frame). Callers
/// reconcile against authoritative REST history instead of guessing.
class ChatUnknownEvent extends ChatRealtimeEvent {
  const ChatUnknownEvent({required super.eventId, required this.type});

  final String type;
}

/// Emitted after a socket is re-established so the controller can refresh
/// authoritative history and cover events missed while disconnected.
class ChatReconnectedEvent extends ChatRealtimeEvent {
  const ChatReconnectedEvent({required super.eventId});
}

/// Decodes chat frames for one circle topic, deduplicating at-least-once
/// delivery by `event_id` (message-id deduplication is owned by the
/// controller, which also sees REST history).
class ChatRealtimeEventDecoder {
  ChatRealtimeEventDecoder(this._circleId);

  final String? _circleId;
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

    if (type == ChatRealtimeTypes.messageRead) {
      final payload = decoded[ChatJsonKeys.payload];
      if (payload is! Map<String, dynamic>) {
        return ChatUnknownEvent(eventId: eventId, type: type);
      }
      try {
        return ChatMessageReadEvent(
          eventId: eventId,
          messageId: payload[ChatJsonKeys.messageId] as String,
          readerId: payload[ChatJsonKeys.readerId] as String,
          readAt: DateTime.parse(payload[ChatJsonKeys.readAt] as String),
        );
      } on FormatException {
        return ChatUnknownEvent(eventId: eventId, type: type);
      } on TypeError {
        return ChatUnknownEvent(eventId: eventId, type: type);
      }
    }
    if (type == ChatRealtimeTypes.typing) {
      final payload = decoded[ChatJsonKeys.payload];
      if (payload is! Map<String, dynamic>) {
        return ChatUnknownEvent(eventId: eventId, type: type);
      }
      try {
        final circleId = payload[ChatJsonKeys.circleId] as String?;
        final dmPeerId = payload[ChatJsonKeys.dmPeerId] as String?;
        if ((circleId == null) == (dmPeerId == null)) {
          return ChatUnknownEvent(eventId: eventId, type: type);
        }
        return ChatTypingEvent(
          eventId: eventId,
          userId: payload[ChatJsonKeys.userId] as String,
          circleId: circleId,
          dmPeerId: dmPeerId,
          isTyping: payload[ChatJsonKeys.isTyping] as bool,
          expiresAt: DateTime.parse(payload[ChatJsonKeys.expiresAt] as String),
        );
      } on FormatException {
        return ChatUnknownEvent(eventId: eventId, type: type);
      } on TypeError {
        return ChatUnknownEvent(eventId: eventId, type: type);
      }
    }
    if (type != ChatRealtimeTypes.message) {
      return ChatUnknownEvent(eventId: eventId, type: type);
    }
    final payload = decoded[ChatJsonKeys.payload];
    if (payload is! Map<String, dynamic>) {
      return ChatUnknownEvent(eventId: eventId, type: type);
    }
    if (_circleId != null && payload[ChatJsonKeys.circleId] != _circleId) {
      return null;
    }
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

/// Realtime additions used only by active chat controllers. Keeping these
/// narrow lets existing circle-only consumers remain on [ChatRealtimeClient].
abstract interface class ChatRealtimePresenceClient {
  Stream<ChatRealtimeEvent> directChatEvents(String peerId,
      {required String token, required String backendSessionId});

  Future<void> sendTyping(
      {String? circleId, String? dmPeerId, required bool isTyping});
}

class WebSocketChatRealtimeClient
    implements ChatRealtimeClient, ChatRealtimePresenceClient {
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
    final connection =
        _ChatSocketConnection(_connections.remove, circleId: circleId);
    _connections.add(connection);
    final controller =
        StreamController<ChatRealtimeEvent>(onCancel: connection.close);
    connection.sink = controller;
    unawaited(_open(circleId, token, backendSessionId, connection));
    return controller.stream;
  }

  @override
  Stream<ChatRealtimeEvent> directChatEvents(String peerId,
      {required String token, required String backendSessionId}) {
    final connection =
        _ChatSocketConnection(_connections.remove, dmPeerId: peerId);
    _connections.add(connection);
    final controller =
        StreamController<ChatRealtimeEvent>(onCancel: connection.close);
    connection.sink = controller;
    unawaited(_open(null, token, backendSessionId, connection));
    return controller.stream;
  }

  @override
  Future<void> sendTyping(
      {String? circleId, String? dmPeerId, required bool isTyping}) async {
    if ((circleId == null) == (dmPeerId == null)) return;
    _ChatSocketConnection? connection;
    for (final candidate in _connections) {
      if (candidate.circleId == circleId && candidate.dmPeerId == dmPeerId) {
        connection = candidate;
        break;
      }
    }
    final socket = connection?.socket;
    if (socket == null || connection!.isClosed) return;
    socket.add(jsonEncode({
      ChatJsonKeys.type: 'cmd.chat.typing',
      'request_id': newChatIdempotencyKey(),
      ChatJsonKeys.payload: {
        ChatJsonKeys.circleId: circleId,
        ChatJsonKeys.dmPeerId: dmPeerId,
        ChatJsonKeys.isTyping: isTyping,
      },
    }));
  }

  Future<void> _open(String? circleId, String token, String backendSessionId,
      _ChatSocketConnection connection) async {
    final decoder = ChatRealtimeEventDecoder(circleId);
    var delay = const Duration(seconds: 1);
    var reconnects = 0;
    while (!connection.isClosed) {
      try {
        final ticket = await _fetchTicket(token, backendSessionId);
        final url = realtimeWebSocketUrl(_dio.options.baseUrl)
            .replace(queryParameters: {'token': ticket});
        final socket = await WebSocket.connect(url.toString());
        if (connection.isClosed) {
          await socket.close();
          return;
        }
        connection.socket = socket;
        final isReconnect = reconnects++ > 0;
        var subscriptionAcknowledged = circleId == null;
        socket.listen((data) {
          if (data is! String || connection.sink.isClosed) return;
          if (!subscriptionAcknowledged &&
              _isSubscriptionAcknowledgement(data, circleId!)) {
            subscriptionAcknowledged = true;
            if (isReconnect) {
              connection.sink.add(ChatReconnectedEvent(
                  eventId:
                      'reconnect-${DateTime.now().microsecondsSinceEpoch}'));
            }
            return;
          }
          final event = decoder.decode(data);
          if (event != null) connection.sink.add(event);
        }, onError: (_) {}, onDone: () {});
        if (circleId != null) {
          socket.add(jsonEncode(realtimeSubscribeMessage('circle.$circleId')));
        }
        connection.heartbeat = Timer.periodic(_heartbeatInterval, (_) {
          if (!connection.isClosed) socket.add(jsonEncode(realtimePingMessage));
        });
        delay = const Duration(seconds: 1);
        await socket.done;
        connection.heartbeat?.cancel();
        connection.heartbeat = null;
        connection.socket = null;
      } on DioException catch (error) {
        // A rejected ticket means the session is revoked or unauthorized:
        // reconnecting would retry forever against a terminal state, so the
        // stream ends and REST reconciliation surfaces the access loss
        // (FR-010 suppression; no unbounded battery-draining loop).
        final status = error.response?.statusCode;
        if (status == 401 || status == 403) {
          await connection.close();
          return;
        }
      } catch (_) {
        // Reconnect is deliberately bounded; authoritative REST refresh is
        // owned by the controller after unknown or resumed delivery.
      }
      if (connection.isClosed) return;
      await Future<void>.delayed(delay);
      if (delay.inSeconds < 4) delay *= 2;
    }
  }

  static bool _isSubscriptionAcknowledgement(String raw, String circleId) {
    try {
      final value = jsonDecode(raw);
      return value is Map<String, dynamic> &&
          value[ChatJsonKeys.type] == 'subscribed' &&
          value['topic'] == 'circle.$circleId';
    } on FormatException {
      return false;
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
  _ChatSocketConnection(void Function(_ChatSocketConnection) onRemoved,
      {this.circleId, this.dmPeerId})
      : _onRemoved = onRemoved;

  final void Function(_ChatSocketConnection) _onRemoved;
  final String? circleId;
  final String? dmPeerId;
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
