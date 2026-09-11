import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_picker.dart';
import 'package:halaqaty_mobile/features/chat/application/media_attachment_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';

void main() {
  test('lost attach response retries the staged upload with identical payload',
      () async {
    var uploads = 0;
    final payloads = <String>[];
    final controller = MediaAttachmentController(
      picker: _Picker(),
      uploadImage: (path, progress) async {
        uploads++;
        return const ChatUploadResult(
            objectKey: 'object', url: 'https://media.test', uploadId: 'staged');
      },
      uploadFile: (_, __) async => throw UnimplementedError(),
      attach: (type, uploadId, key) async {
        payloads.add('$uploadId:$key');
        if (payloads.length == 1) {
          throw const ChatApiException(
              statusCode: null,
              code: 'ERR_REQUEST_FAILED',
              message: 'lost response');
        }
        return ChatMessage(
            id: 'message',
            senderId: 'user',
            circleId: 'circle',
            content: '',
            type: type,
            sentAt: DateTime.utc(2026),
            deliveryStatus: ChatDeliveryStatus.sent);
      },
    );
    addTearDown(controller.dispose);
    await controller.pickImage();
    expect(await controller.confirmSend(), isFalse);
    expect(await controller.retry(), isTrue);
    expect(uploads, 1);
    expect(payloads, [payloads.first, payloads.first]);
    expect(controller.state.phase, MediaAttachmentPhase.idle);
  });

  test('native picker failure becomes recoverable safe state', () async {
    final controller = MediaAttachmentController(
        picker: _Picker(fail: true),
        uploadImage: (_, __) async => throw UnimplementedError(),
        uploadFile: (_, __) async => throw UnimplementedError(),
        attach: (_, __, ___) async => throw UnimplementedError());
    addTearDown(controller.dispose);
    await controller.pickPdf();
    expect(controller.state.phase, MediaAttachmentPhase.failed);
    controller.cancel();
    expect(controller.state.phase, MediaAttachmentPhase.idle);
  });
}

class _Picker implements ChatAttachmentPicker {
  _Picker({this.fail = false});
  final bool fail;
  @override
  Future<String?> pickImage() async => '/tmp/image.png';
  @override
  Future<String?> pickPdf() async {
    if (fail) throw PlatformException(code: 'native_failure');
    return null;
  }
}
