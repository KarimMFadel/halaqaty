import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/data/pending_message_store.dart';

class _MemoryStorage extends FlutterSecureStorage {
  final values = <String, String>{};

  @override
  Future<String?> read(
          {required String key,
          IOSOptions? iOptions,
          AndroidOptions? aOptions,
          WebOptions? webOptions,
          MacOsOptions? mOptions,
          LinuxOptions? lOptions,
          WindowsOptions? wOptions}) async =>
      values[key];

  @override
  Future<void> write(
      {required String key,
      required String? value,
      IOSOptions? iOptions,
      AndroidOptions? aOptions,
      WebOptions? webOptions,
      MacOsOptions? mOptions,
      LinuxOptions? lOptions,
      WindowsOptions? wOptions}) async {
    if (value == null) {
      values.remove(key);
    } else {
      values[key] = value;
    }
  }

  @override
  Future<void> delete(
          {required String key,
          IOSOptions? iOptions,
          AndroidOptions? aOptions,
          WebOptions? webOptions,
          MacOsOptions? mOptions,
          LinuxOptions? lOptions,
          WindowsOptions? wOptions}) async =>
      values.remove(key);
}

void main() {
  test('persists one stable envelope and reloads it after restart', () async {
    final storage = _MemoryStorage();
    final envelope = PendingMessageEnvelope(
        idempotencyKey: 'stable-key',
        circleId: 'circle',
        content: 'hello',
        attachmentPaths: const ['/tmp/a.m4a']);

    await PendingMessageStore(storage).save(envelope);
    final loaded = await PendingMessageStore(storage).loadAll();

    expect(loaded.single.idempotencyKey, 'stable-key');
    expect(loaded.single.attachmentPaths, ['/tmp/a.m4a']);
  });

  test('edits and discards an envelope without changing its key', () async {
    final storage = _MemoryStorage();
    final store = PendingMessageStore(storage);
    await store.save(const PendingMessageEnvelope(
        idempotencyKey: 'same', circleId: 'circle', content: 'old'));

    await store.edit('same', 'new');
    expect((await store.loadAll()).single.content, 'new');
    await store.discard('same');
    expect(await store.loadAll(), isEmpty);
  });
}
