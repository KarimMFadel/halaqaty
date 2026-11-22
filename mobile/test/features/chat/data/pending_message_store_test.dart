import 'dart:async';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/data/pending_message_store.dart';

class _MemoryStorage extends FlutterSecureStorage {
  final values = <String, String>{};
  Completer<void>? writeGate;
  Completer<void>? writeStarted;

  @override
  Future<String?> read({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    LinuxOptions? lOptions,
    WindowsOptions? wOptions,
  }) async =>
      values[key];

  @override
  Future<void> write({
    required String key,
    required String? value,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    LinuxOptions? lOptions,
    WindowsOptions? wOptions,
  }) async {
    if (key.endsWith('.draft') && writeGate != null) {
      if (!writeStarted!.isCompleted) writeStarted!.complete();
      await writeGate!.future;
    }
    if (value == null) {
      values.remove(key);
    } else {
      values[key] = value;
    }
  }

  @override
  Future<void> delete({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    LinuxOptions? lOptions,
    WindowsOptions? wOptions,
  }) async =>
      values.remove(key);
}

void main() {
  test('legacy unscoped drafts are never restored', () async {
    final storage = _MemoryStorage();
    storage.values['halaqaty.chat.pending.index'] = 'legacy';
    storage.values['halaqaty.chat.pending.legacy'] =
        '{"idempotency_key":"legacy","circle_id":"circle","content":"private"}';
    expect(
      await PendingMessageStore(storage, userId: 'user-b').loadAll(),
      isEmpty,
    );
  });

  test(
    'clear waits for another instance and rejects its later writes',
    () async {
      final storage = _MemoryStorage()
        ..writeGate = Completer<void>()
        ..writeStarted = Completer<void>();
      final oldStore = PendingMessageStore(storage, userId: 'user-a');
      const draft = PendingMessageEnvelope(
        idempotencyKey: 'draft',
        circleId: 'circle',
        content: 'private',
      );
      final saving = oldStore.save(draft);
      await storage.writeStarted!.future;
      final clearing = PendingMessageStore(storage, userId: 'user-a').clear();
      storage.writeGate!.complete();
      await saving;
      await clearing;
      await oldStore.save(draft);
      expect(
        await PendingMessageStore(storage, userId: 'user-a').loadAll(),
        isEmpty,
      );
    },
  );

  test('persists one stable envelope and reloads it after restart', () async {
    final storage = _MemoryStorage();
    final envelope = PendingMessageEnvelope(
      idempotencyKey: 'stable-key',
      circleId: 'circle',
      content: 'hello',
      attachmentPaths: const ['/tmp/a.m4a'],
    );

    await PendingMessageStore(storage, userId: 'user-a').save(envelope);
    final loaded = await PendingMessageStore(
      storage,
      userId: 'user-a',
    ).loadAll();

    expect(loaded.single.idempotencyKey, 'stable-key');
    expect(loaded.single.attachmentPaths, ['/tmp/a.m4a']);
  });

  test('edits and discards an envelope without changing its key', () async {
    final storage = _MemoryStorage();
    final store = PendingMessageStore(storage, userId: 'user-a');
    await store.save(
      const PendingMessageEnvelope(
        idempotencyKey: 'same',
        circleId: 'circle',
        content: 'old',
      ),
    );

    await store.edit('same', 'new');
    expect((await store.loadAll()).single.content, 'new');
    await store.discard('same');
    expect(await store.loadAll(), isEmpty);
  });

  test('drafts are isolated per account on the same device', () async {
    final storage = _MemoryStorage();
    await PendingMessageStore(storage, userId: 'user-a').save(
      const PendingMessageEnvelope(
        idempotencyKey: 'draft-a',
        circleId: 'circle',
        content: 'from A',
      ),
    );

    expect(
      await PendingMessageStore(storage, userId: 'user-b').loadAll(),
      isEmpty,
    );
    final own = await PendingMessageStore(storage, userId: 'user-a').loadAll();
    expect(own.single.content, 'from A');
  });

  test('clear removes only the signing-out account drafts', () async {
    final storage = _MemoryStorage();
    await PendingMessageStore(storage, userId: 'user-a').save(
      const PendingMessageEnvelope(
        idempotencyKey: 'draft-a',
        circleId: 'circle',
        content: 'from A',
      ),
    );
    await PendingMessageStore(storage, userId: 'user-b').save(
      const PendingMessageEnvelope(
        idempotencyKey: 'draft-b',
        circleId: 'circle',
        content: 'from B',
      ),
    );

    await PendingMessageStore(storage, userId: 'user-a').clear();

    expect(
      await PendingMessageStore(storage, userId: 'user-a').loadAll(),
      isEmpty,
    );
    expect(
      (await PendingMessageStore(
        storage,
        userId: 'user-b',
      ).loadAll())
          .single
          .content,
      'from B',
    );
  });
}
