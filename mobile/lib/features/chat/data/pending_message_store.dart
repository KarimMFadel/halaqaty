import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// A durable local envelope for a logical chat send.
class PendingMessageEnvelope {
  const PendingMessageEnvelope({
    required this.idempotencyKey,
    required this.circleId,
    required this.content,
    this.attachmentPaths = const [],
    this.updatedAt,
  });

  final String idempotencyKey;
  final String circleId;
  final String content;
  final List<String> attachmentPaths;
  final DateTime? updatedAt;

  Map<String, dynamic> toJson() => {
        'idempotency_key': idempotencyKey,
        'circle_id': circleId,
        'content': content,
        'attachment_paths': attachmentPaths,
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };

  factory PendingMessageEnvelope.fromJson(Map<String, dynamic> json) =>
      PendingMessageEnvelope(
        idempotencyKey: json['idempotency_key'] as String,
        circleId: json['circle_id'] as String,
        content: json['content'] as String? ?? '',
        attachmentPaths:
            (json['attachment_paths'] as List<dynamic>? ?? const [])
                .whereType<String>()
                .toList(growable: false),
        updatedAt: json['updated_at'] is String
            ? DateTime.tryParse(json['updated_at'] as String)
            : null,
      );
}

/// Encrypted-at-rest pending message storage, namespaced per account so a
/// draft written by one user is never restored or sent under another user
/// sharing the device. The index makes restart reload bounded and avoids
/// relying on platform key enumeration.
class PendingMessageStore {
  PendingMessageStore(this._storage, {required String userId})
      : _indexKey = 'halaqaty.chat.pending.$userId.index',
        _prefix = 'halaqaty.chat.pending.$userId.' {
    _queue = (_queues[_storage] ??= {}).putIfAbsent(_indexKey, _DraftQueue.new);
    _generation = _queue.generation;
  }

  static final _queues = Expando<Map<String, _DraftQueue>>();
  final String _indexKey;
  final String _prefix;
  final FlutterSecureStorage _storage;
  late final _DraftQueue _queue;
  late final int _generation;

  Future<List<PendingMessageEnvelope>> loadAll() async {
    final keys = (await _storage.read(
          key: _indexKey,
        ))
            ?.split('\n')
            .where((key) => key.isNotEmpty)
            .toList() ??
        <String>[];
    final result = <PendingMessageEnvelope>[];
    for (final id in keys) {
      final raw = await _storage.read(key: '$_prefix$id');
      if (raw == null) continue;
      try {
        result.add(
          PendingMessageEnvelope.fromJson(
            jsonDecode(raw) as Map<String, dynamic>,
          ),
        );
      } on Object {
        // Corrupt local entries are discarded during reconciliation.
        await _storage.delete(key: '$_prefix$id');
      }
    }
    return result;
  }

  Future<void> save(PendingMessageEnvelope envelope) =>
      _serialize(() => _save(envelope));

  Future<void> _save(PendingMessageEnvelope envelope) async {
    final current = await _ids();
    if (!current.contains(envelope.idempotencyKey)) {
      current.add(envelope.idempotencyKey);
    }
    await _storage.write(
      key: '$_prefix${envelope.idempotencyKey}',
      value: jsonEncode(envelope.toJson()),
    );
    await _writeIds(current);
  }

  Future<void> edit(String idempotencyKey, String content) =>
      _serialize(() => _edit(idempotencyKey, content));

  Future<void> _edit(String idempotencyKey, String content) async {
    final raw = await _storage.read(key: '$_prefix$idempotencyKey');
    if (raw == null) return;
    final current = PendingMessageEnvelope.fromJson(
      jsonDecode(raw) as Map<String, dynamic>,
    );
    await _save(
      PendingMessageEnvelope(
        idempotencyKey: current.idempotencyKey,
        circleId: current.circleId,
        content: content,
        attachmentPaths: current.attachmentPaths,
        updatedAt: DateTime.now().toUtc(),
      ),
    );
  }

  /// Removes every durable draft owned by this account, e.g. on logout.
  Future<void> clear() {
    _queue.generation++;
    return _serialize(_clear, allowStale: true);
  }

  Future<void> _clear() async {
    for (final id in await _ids()) {
      await _storage.delete(key: '$_prefix$id');
    }
    await _storage.delete(key: _indexKey);
  }

  Future<void> discard(String idempotencyKey) =>
      _serialize(() => _discard(idempotencyKey));

  Future<void> _discard(String idempotencyKey) async {
    final ids = await _ids()
      ..remove(idempotencyKey);
    await _storage.delete(key: '$_prefix$idempotencyKey');
    await _writeIds(ids);
  }

  Future<List<String>> _ids() async =>
      (await _storage.read(
        key: _indexKey,
      ))
          ?.split('\n')
          .where((key) => key.isNotEmpty)
          .toList() ??
      <String>[];

  Future<void> _writeIds(List<String> ids) =>
      _storage.write(key: _indexKey, value: ids.join('\n'));

  Future<void> _serialize(
    Future<void> Function() operation, {
    bool allowStale = false,
  }) {
    final result = _queue.mutation.then((_) async {
      if (allowStale || _generation == _queue.generation) await operation();
    });
    _queue.mutation = result.catchError((Object _) {});
    return result;
  }
}

class _DraftQueue {
  Future<void> mutation = Future<void>.value();
  int generation = 0;
}
