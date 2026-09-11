import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_access_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_picker.dart';
import 'package:halaqaty_mobile/features/chat/application/media_attachment_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/voice_note_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_media_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';

const _circleId = 'circle-1';
const _messageId = 'msg-1';

ChatUploadResult _uploadResult([String uploadId = 'upload-1']) =>
    ChatUploadResult(
      objectKey: 'chat/voice/abc.m4a',
      url: 'https://media.example.com/chat/voice/abc.m4a?sig=1',
      uploadId: uploadId,
      urlExpiresAt: DateTime.parse('2026-09-18T12:00:00Z'),
    );

ChatMessage _mediaMessage(
  ChatMessageType type, {
  String? mediaUrl,
  DateTime? mediaUrlExpiresAt,
  String? fileName,
  int? voiceDurationSeconds,
}) =>
    ChatMessage(
      id: _messageId,
      senderId: 'other-1',
      circleId: _circleId,
      content: '',
      type: type,
      sentAt: DateTime.utc(2026, 9, 3, 12),
      deliveryStatus: ChatDeliveryStatus.delivered,
      senderName: 'مريم',
      mediaUrl: mediaUrl,
      mediaUrlExpiresAt: mediaUrlExpiresAt,
      fileName: fileName,
      voiceDurationSeconds: voiceDurationSeconds,
    );

/// Bilingual label accessor mirroring the repo's `rtl ? ar : en` pattern so
/// every test asserts the exact user-facing copy in both locales.
class _Labels {
  const _Labels(this.rtl);

  final bool rtl;

  String get attach => rtl ? ChatUiLabels.attach : ChatUiLabels.attachEn;
  String get attachImage =>
      rtl ? ChatUiLabels.attachImage : ChatUiLabels.attachImageEn;
  String get attachPdf =>
      rtl ? ChatUiLabels.attachPdf : ChatUiLabels.attachPdfEn;
  String get recordVoiceNote => rtl
      ? ChatUiLabels.recordVoiceNote
      : ChatUiLabels.recordVoiceNoteEn;
  String get stopRecording =>
      rtl ? ChatUiLabels.stopRecording : ChatUiLabels.stopRecordingEn;
  String get sendRecording =>
      rtl ? ChatUiLabels.sendRecording : ChatUiLabels.sendRecordingEn;
  String get recordingDuration => rtl
      ? ChatUiLabels.recordingDuration
      : ChatUiLabels.recordingDurationEn;
  String get waveform =>
      rtl ? ChatUiLabels.waveform : ChatUiLabels.waveformEn;
  String get preview =>
      rtl ? ChatUiLabels.previewVoiceNote : ChatUiLabels.previewVoiceNoteEn;
  String get discard =>
      rtl ? ChatUiLabels.discardVoiceNote : ChatUiLabels.discardVoiceNoteEn;
  String get micPermissionDenied => rtl
      ? ChatUiLabels.micPermissionDenied
      : ChatUiLabels.micPermissionDeniedEn;
  String get openSettings =>
      rtl ? ChatUiLabels.openSettings : ChatUiLabels.openSettingsEn;
  String get voiceSendFailed =>
      rtl ? ChatUiLabels.voiceSendFailed : ChatUiLabels.voiceSendFailedEn;
  String get voicePreviewFailed => rtl
      ? ChatUiLabels.voicePreviewFailed
      : ChatUiLabels.voicePreviewFailedEn;
  String get sendingVoice =>
      rtl ? ChatUiLabels.sendingVoice : ChatUiLabels.sendingVoiceEn;
  String get cancel => rtl ? ChatUiLabels.cancel : ChatUiLabels.cancelEn;
  String get send => rtl ? ChatUiLabels.send : ChatUiLabels.sendEn;
  String get retry => rtl ? ChatUiLabels.retry : ChatUiLabels.retryEn;
  String get uploading =>
      rtl ? ChatUiLabels.uploading : ChatUiLabels.uploadingEn;
  String get attaching =>
      rtl ? ChatUiLabels.attaching : ChatUiLabels.attachingEn;
  String get uploadTooLargeImage => rtl
      ? ChatUiLabels.uploadTooLargeImage
      : ChatUiLabels.uploadTooLargeImageEn;
  String get uploadUnsupportedType => rtl
      ? ChatUiLabels.uploadUnsupportedType
      : ChatUiLabels.uploadUnsupportedTypeEn;
  String get uploadInvalid =>
      rtl ? ChatUiLabels.uploadInvalid : ChatUiLabels.uploadInvalidEn;
  String get uploadRateLimited => rtl
      ? ChatUiLabels.uploadRateLimited
      : ChatUiLabels.uploadRateLimitedEn;
  String get uploadNetworkError => rtl
      ? ChatUiLabels.uploadNetworkError
      : ChatUiLabels.uploadNetworkErrorEn;
  String get attachFailedMedia => rtl
      ? ChatUiLabels.attachFailedMedia
      : ChatUiLabels.attachFailedMediaEn;
  String get playVoiceMessage => rtl
      ? ChatUiLabels.playVoiceMessage
      : ChatUiLabels.playVoiceMessageEn;
  String get loadingMedia =>
      rtl ? ChatUiLabels.loadingMedia : ChatUiLabels.loadingMediaEn;
  String get mediaAccessDenied => rtl
      ? ChatUiLabels.mediaAccessDenied
      : ChatUiLabels.mediaAccessDeniedEn;
  String get mediaAccessFailed => rtl
      ? ChatUiLabels.mediaAccessFailed
      : ChatUiLabels.mediaAccessFailedEn;
  String get linkExpired =>
      rtl ? ChatUiLabels.linkExpired : ChatUiLabels.linkExpiredEn;
  String get renewLink =>
      rtl ? ChatUiLabels.renewLink : ChatUiLabels.renewLinkEn;
  String get downloadPdf =>
      rtl ? ChatUiLabels.downloadPdf : ChatUiLabels.downloadPdfEn;
  String get downloaded =>
      rtl ? ChatUiLabels.downloaded : ChatUiLabels.downloadedEn;
  String get pdfFallbackName =>
      rtl ? ChatUiLabels.pdfFallbackName : ChatUiLabels.pdfFallbackNameEn;
  String get imageAlt => rtl ? ChatUiLabels.imageAlt : ChatUiLabels.imageAltEn;
}

void main() {
  group('ChatMediaComposerBar voice recording', () {
    testWidgets(
        'recording shows live duration, an accessible waveform, and stop '
        '(RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(true);

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump();
        await tester.pump(const Duration(seconds: 2));

        // Duration advances and is announced, not just drawn.
        expect(find.text('0:02'), findsOneWidget);
        expect(find.bySemanticsLabel(labels.recordingDuration), findsOneWidget);
        // The waveform is exposed to assistive tech with a label.
        expect(find.bySemanticsLabel(labels.waveform), findsOneWidget);
        expect(find.bySemanticsLabel(labels.stopRecording), findsOneWidget);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'stop freezes the note and offers preview, discard, and send '
        '(RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(true);

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump(const Duration(seconds: 3));
        await tester.tap(find.bySemanticsLabel(labels.stopRecording));
        await tester.pumpAndSettle();

        expect(find.text('0:03'), findsOneWidget);
        expect(find.bySemanticsLabel(labels.preview), findsOneWidget);
        expect(find.bySemanticsLabel(labels.discard), findsOneWidget);
        expect(find.bySemanticsLabel(labels.sendRecording), findsOneWidget);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'preview plays the recording and a playback failure shows safe '
        'localized copy (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(true);

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump();
        await tester.tap(find.bySemanticsLabel(labels.stopRecording));
        await tester.pumpAndSettle();

        // Preview failure: safe copy in a live region; the note is kept.
        harness.player.playError = StateError('decoder secret failure');
        await tester.tap(find.bySemanticsLabel(labels.preview));
        await tester.pumpAndSettle();

        expect(find.text(labels.voicePreviewFailed), findsOneWidget);
        expect(
          tester
              .getSemantics(find.text(labels.voicePreviewFailed))
              .flagsCollection
              .isLiveRegion,
          isTrue,
        );
        expect(find.textContaining('decoder'), findsNothing);
        expect(find.bySemanticsLabel(labels.sendRecording), findsOneWidget);

        // The kept note is still previewable once the failure clears.
        harness.player.playError = null;
        await tester.tap(find.bySemanticsLabel(labels.preview));
        await tester.pump();
        expect(harness.player.playedPath, harness.recorder.startedPath);
        harness.player.playback.complete();
        await tester.pumpAndSettle();
        expect(find.bySemanticsLabel(labels.sendRecording), findsOneWidget);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets('discard resets the composer to idle (RTL + LTR)',
        (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(true);

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump();
        await tester.tap(find.bySemanticsLabel(labels.stopRecording));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(labels.discard));
        await tester.pumpAndSettle();

        expect(find.bySemanticsLabel(labels.recordVoiceNote), findsOneWidget);
        expect(find.bySemanticsLabel(labels.sendRecording), findsNothing);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'denied microphone permission surfaces guidance, never a dead end '
        '(RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(false);

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pumpAndSettle();

        expect(find.text(labels.micPermissionDenied), findsOneWidget);
        expect(find.text(labels.openSettings), findsOneWidget);
        // The mic stays retryable once the user grants permission.
        harness.recorder.permission = Completer<bool>()..complete(true);
        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump();
        expect(find.bySemanticsLabel(labels.stopRecording), findsOneWidget);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'sending uploads with the recorded duration and attaches the staged '
        'upload (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(true);

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump(const Duration(seconds: 7));
        File(harness.recorder.startedPath!)
            .writeAsBytesSync(List<int>.filled(8, 1));
        await tester.tap(find.bySemanticsLabel(labels.stopRecording));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(labels.sendRecording));
        await tester.pumpAndSettle();

        expect(harness.voiceUploadCalls, hasLength(1));
        expect(harness.voiceUploadCalls.single.$2, 7);
        expect(harness.attaches.single.type, ChatMessageType.voice);
        expect(harness.attaches.single.uploadId, 'upload-1');
        // Success clears both panels back to idle.
        expect(find.bySemanticsLabel(labels.recordVoiceNote), findsOneWidget);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'a rejected upload keeps the note and shows safe retryable copy '
        '(RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(true);
        harness.voiceUploadError = const ChatApiException(
          statusCode: null,
          code: 'ERR_REQUEST_FAILED',
          message: 'socket secret',
        );

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump();
        File(harness.recorder.startedPath!)
            .writeAsBytesSync(List<int>.filled(8, 1));
        await tester.tap(find.bySemanticsLabel(labels.stopRecording));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(labels.sendRecording));
        await tester.pumpAndSettle();

        expect(find.text(labels.voiceSendFailed), findsOneWidget);
        expect(find.textContaining('socket secret'), findsNothing);

        // Retry with recovered connectivity succeeds.
        harness.voiceUploadError = null;
        await tester.tap(find.bySemanticsLabel(labels.retry));
        await tester.pumpAndSettle();
        expect(harness.attaches.single.type, ChatMessageType.voice);
        expect(find.bySemanticsLabel(labels.recordVoiceNote), findsOneWidget);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'an attach rejection shows safe copy and retry reuses the same '
        'idempotency key (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(true);
        harness.attachError = const ChatApiException(
          statusCode: 409,
          code: 'ERR_UPLOAD_CONSUMED',
          message: 'Upload already attached.',
        );

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump();
        File(harness.recorder.startedPath!)
            .writeAsBytesSync(List<int>.filled(8, 1));
        await tester.tap(find.bySemanticsLabel(labels.stopRecording));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(labels.sendRecording));
        await tester.pumpAndSettle();

        expect(find.text(labels.attachFailedMedia), findsOneWidget);
        expect(find.textContaining('ERR_UPLOAD_CONSUMED'), findsNothing);

        // Same-key retry converges on the idempotent attach.
        harness.attachError = null;
        await tester.tap(find.bySemanticsLabel(labels.retry));
        await tester.pumpAndSettle();
        expect(harness.attaches, hasLength(2));
        expect(harness.attaches.first.key, harness.attaches.last.key);
        harness.voice.dispose();
      }
      semantics.dispose();
    });
  });

  group('ChatMediaComposerBar attachments', () {
    testWidgets(
        'picked image shows a preview before any upload starts; cancel '
        'discards it (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        final image = await _tempFile('photo.png');
        harness.picker.image = image.path;

        await tester.tap(find.bySemanticsLabel(labels.attach));
        await tester.pump(const Duration(milliseconds: 500));
        await tester.tap(find.bySemanticsLabel(labels.attachImage));
        await tester.pump(const Duration(milliseconds: 500));

        expect(find.byType(Image), findsOneWidget);
        expect(find.text('photo.png'), findsOneWidget);
        expect(find.bySemanticsLabel(labels.cancel), findsOneWidget);
        expect(harness.imageUploadCalls, isEmpty);

        await tester.tap(find.bySemanticsLabel(labels.cancel));
        await tester.pump(const Duration(milliseconds: 500));
        expect(find.byType(Image), findsNothing);
        expect(harness.imageUploadCalls, isEmpty);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'confirming an image uploads with text percentage progress — never '
        'color alone — then attaches (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        final image = await _tempFile('photo.png');
        harness.picker.image = image.path;
        final uploadGate = Completer<void>();
        harness.imageUploadGate = uploadGate;

        await tester.tap(find.bySemanticsLabel(labels.attach));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(labels.attachImage));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(labels.send));
        await tester.pump();

        // Progress is readable as text (percentage), not hue alone.
        expect(find.text(labels.uploading), findsOneWidget);
        expect(find.text('45%'), findsOneWidget);
        uploadGate.complete();
        await tester.pumpAndSettle();

        expect(harness.imageUploadCalls.single, image.path);
        expect(harness.attaches.single.type, ChatMessageType.image);
        expect(harness.attaches.single.uploadId, 'upload-1');
        expect(find.byType(Image), findsNothing);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'the PDF action picks a document, previews its name, and sends it '
        'as a file message (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        final pdf = await _tempFile('exercise.pdf');
        harness.picker.pdf = pdf.path;

        await tester.tap(find.bySemanticsLabel(labels.attach));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(labels.attachPdf));
        await tester.pumpAndSettle();

        expect(find.text('exercise.pdf'), findsOneWidget);
        expect(find.bySemanticsLabel(labels.send), findsOneWidget);
        expect(harness.fileUploadCalls, isEmpty);

        await tester.tap(find.bySemanticsLabel(labels.send));
        await tester.pumpAndSettle();

        expect(harness.fileUploadCalls.single, pdf.path);
        expect(harness.attaches.single.type, ChatMessageType.file);
        expect(find.text('exercise.pdf'), findsNothing);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'a cancelled native pick leaves the composer idle (RTL + LTR)',
        (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);

        await tester.tap(find.bySemanticsLabel(labels.attach));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(labels.attachImage));
        await tester.pumpAndSettle();

        expect(find.bySemanticsLabel(labels.recordVoiceNote), findsOneWidget);
        expect(find.bySemanticsLabel(labels.cancel), findsNothing);
        harness.voice.dispose();
      }
      semantics.dispose();
    });

    testWidgets('server-enforced upload rejections map to safe localized copy',
        (tester) async {
      final semantics = tester.ensureSemantics();
      final cases = <({Object error, String label, bool retryable})>[
        (
          error: ChatApiException(
              statusCode: 413, code: 'ERR_UPLOAD_TOO_LARGE', message: 'big'),
          label: _Labels(false).uploadTooLargeImage,
          retryable: false,
        ),
        (
          error: ChatApiException(
              statusCode: 415, code: 'ERR_UPLOAD_MEDIA_TYPE', message: 'no'),
          label: _Labels(false).uploadUnsupportedType,
          retryable: false,
        ),
        (
          error: ChatApiException(
              statusCode: 422, code: 'ERR_UPLOAD_INVALID', message: 'bad'),
          label: _Labels(false).uploadInvalid,
          retryable: false,
        ),
        (
          error: ChatApiException(
              statusCode: 429, code: 'ERR_RATE_LIMITED', message: 'slow'),
          label: _Labels(false).uploadRateLimited,
          retryable: true,
        ),
        (
          error: ChatApiException(
              statusCode: null, code: 'ERR_REQUEST_FAILED', message: 'x'),
          label: _Labels(false).uploadNetworkError,
          retryable: true,
        ),
        (
          error: ChatMediaLimitException(ChatMediaLimitKind.imageTooLarge),
          label: _Labels(false).uploadTooLargeImage,
          retryable: false,
        ),
      ];

      for (final testCase in cases) {
        final harness =
            await _pumpComposerBar(tester, direction: TextDirection.ltr);
        final image = await _tempFile('photo.png');
        harness.picker.image = image.path;
        harness.imageUploadError = testCase.error;

        await tester.tap(find.bySemanticsLabel(_Labels(false).attach));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(_Labels(false).attachImage));
        await tester.pumpAndSettle();
        await tester.tap(find.bySemanticsLabel(_Labels(false).send));
        await tester.pumpAndSettle();

        expect(find.text(testCase.label), findsOneWidget,
            reason: 'copy for ${testCase.error}');
        expect(find.textContaining('ERR_'), findsNothing);
        final hasRetry =
            find.bySemanticsLabel(_Labels(false).retry).evaluate().isNotEmpty;
        expect(hasRetry, testCase.retryable,
            reason: 'retry offer for ${testCase.error}');
        if (testCase.retryable) {
          harness.imageUploadError = null;
          await tester.tap(find.bySemanticsLabel(_Labels(false).retry));
          await tester.pumpAndSettle();
          expect(harness.imageUploadCalls, hasLength(2));
        }
        harness.voice.dispose();
        await tester.pumpWidget(const SizedBox.shrink());
      }
      semantics.dispose();
    });

    testWidgets('interactive controls meet 48dp targets (RTL + LTR)',
        (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final harness = await _pumpComposerBar(tester, direction: direction);
        harness.recorder.permission.complete(true);

        for (final label in [
          labels.attach,
          labels.recordVoiceNote,
        ]) {
          final target = tester.getSize(find.bySemanticsLabel(label));
          expect(target.width, greaterThanOrEqualTo(48));
          expect(target.height, greaterThanOrEqualTo(48));
        }

        await tester.tap(find.bySemanticsLabel(labels.recordVoiceNote));
        await tester.pump();
        for (final label in [labels.stopRecording]) {
          final target = tester.getSize(find.bySemanticsLabel(label));
          expect(target.width, greaterThanOrEqualTo(48));
          expect(target.height, greaterThanOrEqualTo(48));
        }
        harness.voice.dispose();
      }
      semantics.dispose();
    });
  });

  group('ChatMediaMessageBody', () {
    testWidgets(
        'voice bubble announces duration, flags an expired link, and plays '
        'through a renewed URL (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final access = _FakeAccess();
        await _pumpMessageBody(
          tester,
          direction: direction,
          message: _mediaMessage(
            ChatMessageType.voice,
            mediaUrl: 'https://media.example.com/v/old?sig=0',
            mediaUrlExpiresAt: DateTime.utc(2026, 9, 1),
            fileName: 'note.m4a',
            voiceDurationSeconds: 42,
          ),
          access: access,
        );

        expect(find.text('0:42'), findsOneWidget);
        expect(find.text(labels.linkExpired), findsOneWidget);

        await tester.tap(find.bySemanticsLabel(labels.playVoiceMessage));
        await tester.pump();
        expect(access.renewCalls, 1);
        expect(access.player.urlPlayed, 'https://media.example.com/v/new?sig=1');
        expect(find.text(labels.loadingMedia), findsNothing);

        access.player.urlPlayback.complete();
        await tester.pumpAndSettle();
        expect(find.bySemanticsLabel(labels.playVoiceMessage), findsOneWidget);
        access.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'a denied renewal (403) and a failed renewal render distinct safe '
        'copy, never raw internals (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);

        final denied = _FakeAccess()
          ..renewError = const ChatApiException(
            statusCode: 403,
            code: 'ERR_FORBIDDEN',
            message: 'membership secret details',
          );
        await _pumpMessageBody(
          tester,
          direction: direction,
          message:
              _mediaMessage(ChatMessageType.voice, voiceDurationSeconds: 5),
          access: denied,
        );
        await tester.tap(find.bySemanticsLabel(labels.playVoiceMessage));
        await tester.pumpAndSettle();
        expect(find.text(labels.mediaAccessDenied), findsOneWidget);
        expect(find.textContaining('secret'), findsNothing);
        denied.dispose();
        await tester.pumpWidget(const SizedBox.shrink());

        final failed = _FakeAccess()
          ..renewError = const ChatApiException(
            statusCode: null,
            code: 'ERR_REQUEST_FAILED',
            message: 'offline',
          );
        await _pumpMessageBody(
          tester,
          direction: direction,
          message:
              _mediaMessage(ChatMessageType.voice, voiceDurationSeconds: 5),
          access: failed,
        );
        await tester.tap(find.bySemanticsLabel(labels.playVoiceMessage));
        await tester.pumpAndSettle();
        expect(find.text(labels.mediaAccessFailed), findsOneWidget);
        failed.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'image bubble renders the projected URL with a labeled fallback and '
        'renew action when loading fails (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final access = _FakeAccess();
        await _pumpMessageBody(
          tester,
          direction: direction,
          message: _mediaMessage(
            ChatMessageType.image,
            mediaUrl: 'https://media.example.com/i/mushaf.png',
            mediaUrlExpiresAt: DateTime.utc(2026, 9, 18),
          ),
          access: access,
        );

        expect(find.bySemanticsLabel(labels.imageAlt), findsOneWidget);

        // The renew affordance routes through the access controller.
        await tester.tap(find.bySemanticsLabel(labels.renewLink),
            warnIfMissed: false);
        await tester.pumpAndSettle();
        expect(access.renewCalls, 1);
        access.dispose();
      }
      semantics.dispose();
    });

    testWidgets(
        'PDF bubble shows the safe filename, downloads through a renewed '
        'link, and confirms non-color-only (RTL + LTR)', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final direction in TextDirection.values) {
        final labels = _Labels(direction == TextDirection.rtl);
        final access = _FakeAccess();
        await _pumpMessageBody(
          tester,
          direction: direction,
          message: _mediaMessage(
            ChatMessageType.file,
            mediaUrl: 'https://media.example.com/f/old.pdf',
            fileName: 'exercise.pdf',
          ),
          access: access,
        );

        expect(find.text('exercise.pdf'), findsOneWidget);
        expect(find.bySemanticsLabel(labels.downloadPdf), findsOneWidget);

        await tester.tap(find.bySemanticsLabel(labels.downloadPdf));
        await tester.pumpAndSettle();

        expect(access.renewCalls, 1);
        expect(access.downloadedUrls,
            ['https://media.example.com/f/new?sig=1']);
        expect(find.text(labels.downloaded), findsOneWidget);
        access.dispose();
      }
      semantics.dispose();
    });

    testWidgets('PDF filename falls back safely when the server omits it',
        (tester) async {
      final semantics = tester.ensureSemantics();
      final labels = _Labels(false);
      final access = _FakeAccess();
      await _pumpMessageBody(
        tester,
        direction: TextDirection.ltr,
        message: _mediaMessage(ChatMessageType.file,
            mediaUrl: 'https://media.example.com/f/x.pdf'),
        access: access,
      );

      expect(find.text(labels.pdfFallbackName), findsOneWidget);
      access.dispose();
      semantics.dispose();
    });
  });
}

Future<File> _tempFile(String name) async {
  final dir = await Directory.systemTemp.createTemp('chat_media_widgets');
  final file = File('${dir.path}/$name');
  file.writeAsBytesSync(List<int>.filled(16, 1));
  addTearDown(() => dir.delete(recursive: true));
  return file;
}

/// One pump of the composer bar with every seam faked: the real controllers
/// run, only the platform boundaries are pure Dart.
Future<_ComposerHarness> _pumpComposerBar(
  WidgetTester tester, {
  required TextDirection direction,
}) async {
  final harness = _ComposerHarness();
  await tester.pumpWidget(const SizedBox.shrink());
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        voiceNoteControllerProvider(_circleId)
            .overrideWith((_) => harness.voice),
        mediaAttachmentControllerProvider(_circleId)
            .overrideWith((_) => harness.attachment),
      ],
      child: MaterialApp(
        home: Directionality(
          textDirection: direction,
          child: const Scaffold(
            body: Align(
              alignment: AlignmentDirectional.bottomCenter,
              child: ChatMediaComposerBar(circleId: _circleId),
            ),
          ),
        ),
      ),
    ),
  );
  await tester.pump();
  return harness;
}

Future<void> _pumpMessageBody(
  WidgetTester tester, {
  required TextDirection direction,
  required ChatMessage message,
  required _FakeAccess access,
}) async {
  final controller = access.buildController();
  await tester.pumpWidget(const SizedBox.shrink());
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        chatMediaAccessControllerProvider(_messageId)
            .overrideWith((_) => controller),
      ],
      child: MaterialApp(
        home: Directionality(
          textDirection: direction,
          child: Scaffold(
            body: Center(child: ChatMediaMessageBody(message: message)),
          ),
        ),
      ),
    ),
  );
  // Let a projected Image.network fail into its fallback deterministically.
  await tester.pump();
}

class _ComposerHarness {
  _ComposerHarness() {
    recorder = _FakeRecorder();
    player = _FakePlayer();
    voice = VoiceNoteController(
      recorder: recorder,
      player: player,
      upload: (filePath, durationSeconds) async {
        voiceUploadCalls.add((filePath, durationSeconds));
        final error = voiceUploadError;
        if (error != null) throw error;
        return _uploadResult();
      },
    );
    attachment = MediaAttachmentController(
      picker: picker,
      uploadImage: (filePath, onProgress) async {
        imageUploadCalls.add(filePath);
        onProgress(45, 100);
        final gate = imageUploadGate;
        if (gate != null) await gate.future;
        final error = imageUploadError;
        if (error != null) throw error;
        return _uploadResult();
      },
      uploadFile: (filePath, onProgress) async {
        fileUploadCalls.add(filePath);
        final error = fileUploadError;
        if (error != null) throw error;
        return _uploadResult();
      },
      attach: (type, uploadId, idempotencyKey) async {
        attaches.add(
          (type: type, uploadId: uploadId, key: idempotencyKey),
        );
        final error = attachError;
        if (error != null) throw error;
        return ChatMessage(
          id: 'server-1',
          senderId: 'me-1',
          circleId: _circleId,
          content: '',
          type: type,
          sentAt: DateTime.utc(2026, 9, 3, 12),
          deliveryStatus: ChatDeliveryStatus.delivered,
        );
      },
    );
  }

  late final VoiceNoteController voice;
  late final MediaAttachmentController attachment;
  late final _FakeRecorder recorder;
  late final _FakePlayer player;
  final picker = _FakePicker();

  final voiceUploadCalls = <(String, int)>[];
  Object? voiceUploadError;

  final imageUploadCalls = <String>[];
  Object? imageUploadError;
  Completer<void>? imageUploadGate;

  final fileUploadCalls = <String>[];
  Object? fileUploadError;

  final attaches = <({ChatMessageType type, String uploadId, String key})>[];
  Object? attachError;
}

class _FakePicker implements ChatAttachmentPicker {
  String? image;
  String? pdf;

  @override
  Future<String?> pickImage() async => image;

  @override
  Future<String?> pickPdf() async => pdf;
}

class _FakeRecorder implements VoiceRecorder {
  Completer<bool> permission = Completer<bool>();
  String? startedPath;
  final amplitudes = StreamController<double>.broadcast();

  @override
  Future<bool> hasPermission() async => permission.future;

  @override
  Future<void> start(String filePath) async => startedPath = filePath;

  @override
  Future<String?> stop() async => startedPath;

  @override
  Stream<double> amplitudeStream() => amplitudes.stream;

  @override
  Future<void> dispose() async => amplitudes.close();
}

class _FakePlayer implements PreviewPlayer {
  Object? playError;
  String? playedPath;
  final playback = Completer<void>();
  final urlPlayback = Completer<void>();
  String? urlPlayed;

  @override
  Future<void> playToCompletion(String filePath) async {
    playedPath = filePath;
    if (playError != null) throw playError!;
    await playback.future;
  }

  @override
  Future<void> playUrlToCompletion(String url) async {
    urlPlayed = url;
    await urlPlayback.future;
  }

  @override
  Stream<Duration> get positionStream => const Stream.empty();

  @override
  Future<void> dispose() async {}
}

/// Configurable seam bundle for the per-message access controller.
class _FakeAccess {
  final player = _UrlPlayer();
  int renewCalls = 0;
  Object? renewError;
  final downloadedUrls = <String>[];
  ChatMediaAccessController? _controller;

  ChatMediaAccessController buildController() => _controller ??=
      ChatMediaAccessController(
        renew: () async {
          renewCalls++;
          final error = renewError;
          if (error != null) throw error;
          return ChatMediaAccess(
            url: 'https://media.example.com/v/new?sig=1',
            expiresAt: DateTime.parse('2026-09-25T12:00:00Z'),
          );
        },
        player: player,
        download: (url) async {
          downloadedUrls.add(url);
          return '/tmp/downloaded-${downloadedUrls.length}.pdf';
        },
      );

  void dispose() => _controller?.dispose();
}

class _UrlPlayer implements PreviewPlayer {
  final urlPlayback = Completer<void>();
  String? urlPlayed;

  @override
  Future<void> playToCompletion(String filePath) async {
    throw UnimplementedError();
  }

  @override
  Future<void> playUrlToCompletion(String url) async {
    urlPlayed = url;
    await urlPlayback.future;
  }

  @override
  Stream<Duration> get positionStream => const Stream.empty();

  @override
  Future<void> dispose() async {}
}

