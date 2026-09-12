import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_access_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_picker.dart';
import 'package:halaqaty_mobile/features/chat/application/media_attachment_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/voice_note_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_media_widgets.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:integration_test/integration_test.dart';

const _poll = Duration(milliseconds: 100);

/// T052 acceptance flow (F-004 US3): record/preview/send a compliant voice
/// note and play it back over a renewed link, send compliant JPEG and PDF
/// attachments, renew aged links, and reject every MIME, size, duration,
/// context, authorization, and upload-rate violation — with every rejection
/// surfacing as localized user-facing copy, never raw internal errors.
///
/// Fixtures are tiny synthesized samples embedded as base64 (a one-second
/// silent MP3 the server can ffprobe, a 64×64 ffmpeg JPEG, a 64×64 PNG, and
/// a qpdf-clean minimal PDF); malformed/oversized variants are derived in
/// code so the committed file stays small and binary-free.
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
    'T052: chat media happy paths, renewable links, and every '
    'server-enforced boundary (RTL)',
    (tester) async {
      final env = Platform.environment;
      final apiBaseUrl =
          env['T052_API_BASE_URL'] ?? 'http://host.docker.internal:8080/api/v1';
      final member = _UserCredentials.fromEnv(env, 'MEMBER');
      final outsider = _UserCredentials.fromEnv(env, 'OUTSIDER');

      if (member == null || outsider == null) {
        markTestSkipped(
          'T052_* env vars missing; real-backend integration needs '
          'pre-provisioned Firebase tokens and backend sessions.',
        );
        return;
      }
      // bySemanticsLabel taps (the RTL accessibility surface) need the
      // semantics tree, matching the chat media widget tests.
      final semantics = tester.ensureSemantics();

      final dio = Dio(BaseOptions(baseUrl: apiBaseUrl));
      addTearDown(dio.close);
      final circles = CircleApiClient(dio);
      final chat = ChatApiClient(dio);
      final media = ChatMediaApiClient(dio);

      // ── Fixtures (valid samples + derived rejection variants) ───────────
      final fixtureDir = await Directory.systemTemp.createTemp('t052-media-');
      addTearDown(() => fixtureDir.delete(recursive: true));
      String fixture(String name, List<int> bytes) {
        final file = File('${fixtureDir.path}/$name');
        file.writeAsBytesSync(bytes, flush: true);
        return file.path;
      }

      final voiceBytes = base64Decode(_voiceMp3Base64);
      final jpegBytes = base64Decode(_imageJpegBase64);
      final pngBytes = base64Decode(_imagePngBase64);
      final pdfBytes = base64Decode(_filePdfBase64);
      final rawVoiceBytes = base64Decode(_voiceRawMp3Base64);

      final voiceFile = fixture('t052-note.mp3', voiceBytes);
      final jpegFile = fixture('t052-image.jpg', jpegBytes);
      final pngFile = fixture('t052-image.png', pngBytes);
      final pdfFile = fixture('t052-file.pdf', pdfBytes);

      // >300 s of real decodable audio: raw MPEG frames without an ID3/Xing
      // container header, tiled 151 × 2.064 s ≈ 311.7 s. Only the server can
      // reject this, because the declared duration stays legal at 300 s.
      // (Tiling the headered sample instead would make ffprobe trust the
      // first Xing tag and under-report the true duration.)
      final longVoice = Uint8List(rawVoiceBytes.length * 151);
      for (var i = 0; i < 151; i++) {
        longVoice.setAll(i * rawVoiceBytes.length, rawVoiceBytes);
      }
      final longVoiceFile = fixture('t052-long.mp3', longVoice);

      // >5 MB but under the 21 MB route cap, still a parseable JPEG: the
      // app's local pre-check would refuse it, so the raw seam below carries
      // it to the server to prove the server-side 413 boundary.
      final oversizedJpeg = Uint8List(jpegBytes.length + 5 * 1024 * 1024 + 1024)
        ..setAll(0, jpegBytes);
      final oversizedJpegFile =
          fixture('t052-oversized.raw.jpg', oversizedJpeg);

      // ── World setup via REST (member owns a fresh circle) ────────────────
      final circle = await circles.createCircle(
        firebaseIdToken: member.token,
        sessionId: member.sessionId,
        request: const CreateCircleRequest(
          name: 'T052-chat-media-flow',
          language: 'ar',
          maxCapacity: 10,
        ),
      );
      addTearDown(() async {
        try {
          await circles.archiveCircle(
            firebaseIdToken: member.token,
            sessionId: member.sessionId,
            circleId: circle.id,
          );
        } on DioException {
          // Best-effort cleanup.
        }
      });

      // ── 1. Local pre-upload limits reject before any network work ────────
      await _expectChatMediaLimit(
        () => media.uploadVoice(
          token: member.token,
          sessionId: member.sessionId,
          filePath: voiceFile,
          durationSeconds: ChatLimits.maxVoiceDurationSeconds + 1,
          circleId: circle.id,
        ),
        ChatMediaLimitKind.voiceTooLong,
        'a >300 s declared duration must be refused locally',
      );
      await _expectChatMediaLimit(
        () => media.uploadVoice(
          token: member.token,
          sessionId: member.sessionId,
          filePath:
              fixture('t052-huge.mp3', Uint8List(ChatLimits.maxVoiceBytes + 1)),
          durationSeconds: 5,
          circleId: circle.id,
        ),
        ChatMediaLimitKind.voiceTooLarge,
        'a >20 MB voice file must be refused locally',
      );
      await _expectChatMediaLimit(
        () => media.uploadImage(
          token: member.token,
          sessionId: member.sessionId,
          filePath:
              fixture('t052-huge.jpg', Uint8List(ChatLimits.maxImageBytes + 1)),
          circleId: circle.id,
        ),
        ChatMediaLimitKind.imageTooLarge,
        'a >5 MB image must be refused locally',
      );

      // ── 2. Server-enforced context and duration violations (422/403) ────
      // Rejected uploads stage no object and consume no upload budget, so
      // these run before the happy paths keep the quota math deterministic.
      await _expectChatError(
        () => _rawUpload(
          dio,
          member,
          ChatMediaApiPaths.uploadsVoice,
          voiceFile,
          circleId: circle.id,
          dmPeerId: outsider.userId,
          durationSeconds: ChatLimits.maxVoiceDurationSeconds,
        ),
        {422},
        'uploading with both circle and DM targets must be a 422 context '
        'validation',
      );
      await _expectChatError(
        () => media.uploadVoice(
          token: member.token,
          sessionId: member.sessionId,
          filePath: longVoiceFile,
          durationSeconds: ChatLimits.maxVoiceDurationSeconds,
          circleId: circle.id,
        ),
        {422},
        'voice data whose probed duration exceeds 5 minutes must be a 422',
      );
      await _expectChatError(
        () => media.uploadVoice(
          token: outsider.token,
          sessionId: outsider.sessionId,
          filePath: voiceFile,
          durationSeconds: 5,
          circleId: circle.id,
        ),
        {403, 404},
        'a non-member uploading into the circle must be denied without '
        'enumeration',
      );

      // ── 3. Voice record → preview → send over the real backend (RTL) ────
      final sentMessages = <ChatMessage>[];
      final localPreview = _LocalFilePreviewPlayer(voiceBytes);
      final voice = VoiceNoteController(
        upload: (filePath, durationSeconds) => media.uploadVoice(
          token: member.token,
          sessionId: member.sessionId,
          filePath: filePath,
          durationSeconds: durationSeconds,
          circleId: circle.id,
        ),
        recorder: _ScriptedRecorder(voiceBytes),
        player: localPreview,
      );
      final picker = _AttachmentPicker();
      final attachment = MediaAttachmentController(
        picker: picker,
        uploadImage: (filePath, onProgress) => media.uploadImage(
          token: member.token,
          sessionId: member.sessionId,
          filePath: filePath,
          circleId: circle.id,
          onProgress: onProgress,
        ),
        uploadFile: (filePath, onProgress) => media.uploadFile(
          token: member.token,
          sessionId: member.sessionId,
          filePath: filePath,
          circleId: circle.id,
          onProgress: onProgress,
        ),
        attach: (type, uploadId, idempotencyKey) async {
          final message = await chat.sendMediaMessage(
            token: member.token,
            sessionId: member.sessionId,
            circleId: circle.id,
            type: type,
            uploadId: uploadId,
            idempotencyKey: idempotencyKey,
          );
          sentMessages.add(message);
          return message;
        },
      );
      final composerContainer = ProviderContainer(
        overrides: [
          voiceNoteControllerProvider(circle.id).overrideWith((_) => voice),
          mediaAttachmentControllerProvider(circle.id)
              .overrideWith((_) => attachment),
        ],
      );
      final voiceKeepAlive = composerContainer.listen(
        voiceNoteControllerProvider(circle.id),
        (_, __) {},
      );
      final attachmentKeepAlive = composerContainer.listen(
        mediaAttachmentControllerProvider(circle.id),
        (_, __) {},
      );
      addTearDown(() {
        voiceKeepAlive.close();
        attachmentKeepAlive.close();
        composerContainer.dispose();
      });
      await _pumpComposer(tester, circle.id, composerContainer);

      await tester.tap(find.bySemanticsLabel(ChatUiLabels.recordVoiceNote));
      await _waitFor(
          tester, () => voice.state.phase == VoiceNotePhase.recording);
      // A real ~2 s recording: the tick timer and amplitude stream run in
      // live time, so the waveform and duration states are genuinely driven.
      await Future<void>.delayed(const Duration(milliseconds: 2200));
      await tester.pump();
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.stopRecording));
      await _waitFor(
          tester, () => voice.state.phase == VoiceNotePhase.recorded);
      expect(voice.state.amplitudes, isNotEmpty);

      await tester.tap(find.bySemanticsLabel(ChatUiLabels.previewVoiceNote));
      await _waitFor(tester, () => localPreview.previewedFiles == 1);
      await _waitFor(
          tester, () => voice.state.phase == VoiceNotePhase.recorded);

      await tester.tap(find.bySemanticsLabel(ChatUiLabels.sendRecording));
      await _waitFor(tester, () => sentMessages.isNotEmpty);
      final voiceMessage = sentMessages.single;
      expect(voiceMessage.type, ChatMessageType.voice);
      expect(voiceMessage.mediaUrl, isNotNull);

      // ── 4. Received voice: renewal then playback of the served bytes ────
      final httpPlayer = _HttpFetchPlayer(voiceBytes);
      final voiceAccess = _accessController(
        renew: () => media.renewMediaUrl(
          token: member.token,
          sessionId: member.sessionId,
          messageId: voiceMessage.id,
        ),
        player: httpPlayer,
      );
      await _pumpMediaBody(tester, voiceMessage, voiceAccess);

      await tester.tap(find.bySemanticsLabel(ChatUiLabels.playVoiceMessage));
      await _waitFor(tester, () => httpPlayer.playedUrls.isNotEmpty);
      final renewed = voiceAccess.state.access!;
      expect(httpPlayer.playedUrls.single, renewed.url);
      expect(Uri.parse(renewed.url).hasQuery, isTrue);
      final now = DateTime.now();
      expect(
        renewed.expiresAt.isAfter(now.add(const Duration(days: 6))),
        isTrue,
        reason: 'renewal returns a fresh seven-day presigned URL',
      );
      expect(
        renewed.expiresAt.isBefore(now.add(const Duration(days: 8))),
        isTrue,
        reason: 'renewal returns a fresh seven-day presigned URL',
      );
      expect(httpPlayer.playedUrls.single, renewed.url);
      await _waitFor(tester,
          () => voiceAccess.state.phase != ChatMediaAccessPhase.loading);

      // ── 5. Image happy path: pick → preview → send → render, then renew
      // an aged link and confirm the renewed URL still serves the bytes ────
      await _pumpComposer(tester, circle.id, composerContainer);
      picker.nextImage = pngFile;
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attach));
      await tester.pumpAndSettle();
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attachImage));
      await tester.pumpAndSettle();
      await _waitFor(tester,
          () => attachment.state.phase == MediaAttachmentPhase.previewing);
      expect(attachment.state.kind, MediaAttachmentKind.image);

      await tester.tap(find.bySemanticsLabel(ChatUiLabels.send));
      await _waitFor(tester, () => sentMessages.length == 2);
      final imageMessage = sentMessages[1];
      expect(imageMessage.type, ChatMessageType.image);
      // media_url is nullable on the canonical Message response; the
      // renewable-link controller obtains the authorized URL below.

      // Aged projection: the presigned window has lapsed, so the body offers
      // renewal; the renewed URL must serve the exact uploaded PNG bytes.
      final agedImage = ChatMessage(
        id: imageMessage.id,
        senderId: imageMessage.senderId,
        circleId: imageMessage.circleId,
        content: imageMessage.content,
        type: imageMessage.type,
        sentAt: imageMessage.sentAt,
        deliveryStatus: imageMessage.deliveryStatus,
        mediaUrl: imageMessage.mediaUrl,
        mediaUrlExpiresAt: DateTime.now().subtract(const Duration(days: 8)),
      );
      final pngProbe = _HttpFetchPlayer(pngBytes);
      final agedAccess = _accessController(
        renew: () => media.renewMediaUrl(
          token: member.token,
          sessionId: member.sessionId,
          messageId: imageMessage.id,
        ),
        player: pngProbe,
      );
      await _pumpMediaBody(tester, agedImage, agedAccess);
      expect(find.text(ChatUiLabels.linkExpired), findsOneWidget);

      await tester.tap(find.bySemanticsLabel(ChatUiLabels.renewLink));
      await _waitFor(
          tester, () => agedAccess.state.phase == ChatMediaAccessPhase.ready);
      await tester.pumpAndSettle();
      expect(find.text(ChatUiLabels.linkExpired), findsNothing);
      expect(find.byIcon(Icons.broken_image), findsNothing);
      expect(agedAccess.state.access, isNotNull);
      // The renewed private link itself serves the uploaded content.
      final served = await _httpGetBytes(agedAccess.state.access!.url);
      expect(served, pngBytes);

      // ── 6. PDF happy path: pick → send → download ────────────────────────
      await _pumpComposer(tester, circle.id, composerContainer);
      picker.nextPdf = pdfFile;
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attach));
      await tester.pumpAndSettle();
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attachPdf));
      await tester.pumpAndSettle();
      await _waitFor(tester,
          () => attachment.state.phase == MediaAttachmentPhase.previewing);
      expect(attachment.state.kind, MediaAttachmentKind.pdf);

      await tester.tap(find.bySemanticsLabel(ChatUiLabels.send));
      await _waitFor(tester, () => sentMessages.length == 3);
      final pdfMessage = sentMessages[2];
      expect(pdfMessage.type, ChatMessageType.file);
      // file_name on the Message response is backend-pending (final inventory).

      final pdfAccess = _accessController(
        renew: () => media.renewMediaUrl(
          token: member.token,
          sessionId: member.sessionId,
          messageId: pdfMessage.id,
        ),
        player: _HttpFetchPlayer(pdfBytes),
      );
      await _pumpMediaBody(tester, pdfMessage, pdfAccess);
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.downloadPdf));
      await _waitFor(tester,
          () => pdfAccess.state.phase == ChatMediaAccessPhase.downloaded);
      expect(find.text(ChatUiLabels.downloaded), findsOneWidget);
      final downloaded = await _newestDownloadedPdf();
      expect(downloaded, isNotNull, reason: 'download writes a temp PDF');
      expect(downloaded!, pdfBytes);

      // ── 7. Authoritative history carries the three durable messages ────
      final history = await chat.listMessages(
        token: member.token,
        sessionId: member.sessionId,
        circleId: circle.id,
        limit: 50,
      );
      final byId = {
        for (final message in history.messages) message.id: message
      };
      expect(byId[voiceMessage.id], isNotNull);
      expect(byId[imageMessage.id], isNotNull);
      expect(byId[pdfMessage.id], isNotNull);

      // ── 8. Unauthorized access: non-members can neither read nor renew ──
      await _expectChatError(
        () => chat.listMessages(
          token: outsider.token,
          sessionId: outsider.sessionId,
          circleId: circle.id,
        ),
        {403, 404},
        'a non-member listing circle messages must be denied without '
        'enumeration',
      );
      await _expectChatError(
        () => media.renewMediaUrl(
          token: outsider.token,
          sessionId: outsider.sessionId,
          messageId: voiceMessage.id,
        ),
        {403, 404},
        'a non-member renewing another circle\'s media link must be denied',
      );
      // And the denial surface is localized, not a raw error dump.
      final outsiderAccess = _accessController(
        renew: () => media.renewMediaUrl(
          token: outsider.token,
          sessionId: outsider.sessionId,
          messageId: voiceMessage.id,
        ),
        player: _HttpFetchPlayer(voiceBytes),
      );
      await _pumpMediaBody(tester, voiceMessage, outsiderAccess);
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.playVoiceMessage));
      await _waitFor(tester,
          () => outsiderAccess.state.phase == ChatMediaAccessPhase.denied);
      expect(find.text(ChatUiLabels.mediaAccessDenied), findsOneWidget);
      expect(outsiderAccess.state.access, isNull);
      expect(find.textContaining('ERR_'), findsNothing);

      // ── 9. Upload-rate limit: 10 staged uploads per rolling hour ─────────
      // Three happy-path uploads are staged; pad with seven more staged
      // images, then the next attempt must be a 429 that surfaces as
      // localized, retryable copy in the composer (never a raw error).
      for (var i = 0; i < 7; i++) {
        await media.uploadImage(
          token: member.token,
          sessionId: member.sessionId,
          filePath: jpegFile,
          circleId: circle.id,
        );
      }
      await _expectChatError(
        () => media.uploadImage(
          token: member.token,
          sessionId: member.sessionId,
          filePath: jpegFile,
          circleId: circle.id,
        ),
        {429},
        'the 11th successfully-staged-eligible upload in the rolling hour '
        'must be rate limited',
      );

      await _pumpComposer(tester, circle.id, composerContainer);
      picker.nextImage = jpegFile;
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attach));
      await tester.pumpAndSettle();
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attachImage));
      await tester.pumpAndSettle();
      await _waitFor(tester,
          () => attachment.state.phase == MediaAttachmentPhase.previewing);
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.send));
      await _waitFor(tester,
          () => attachment.state.error == MediaAttachmentError.rateLimited);
      await tester.pump();
      expect(find.text(ChatUiLabels.uploadRateLimited), findsOneWidget);
      expect(find.bySemanticsLabel(ChatUiLabels.retry), findsOneWidget);
      expect(find.textContaining('ERR_'), findsNothing);

      // ── 10. Wrong MIME is rejected by the server as 415 (localized UI) ──
      final gifFile = fixture(
        't052-icon.gif',
        asciiBytes('GIF89a') + Uint8List(120),
      );
      picker.nextImage = gifFile;
      attachment.cancel();
      await tester.pump();
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attach));
      await tester.pumpAndSettle();
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attachImage));
      await tester.pumpAndSettle();
      await _waitFor(tester,
          () => attachment.state.phase == MediaAttachmentPhase.previewing);
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.send));
      await _waitFor(tester,
          () => attachment.state.error == MediaAttachmentError.unsupportedType);
      await tester.pump();
      expect(find.text(ChatUiLabels.uploadUnsupportedType), findsOneWidget);
      expect(find.bySemanticsLabel(ChatUiLabels.retry), findsNothing);
      expect(find.textContaining('ERR_'), findsNothing);

      // ── 11. Server-side 413 for an oversized image (raw seam) ───────────
      // The app's local 5 MB pre-check (section 1) normally prevents this
      // upload from starting; the raw seam mirrors the client's multipart
      // call without the pre-check so the server's own limit is exercised.
      final rawComposer = MediaAttachmentController(
        picker: picker,
        uploadImage: (filePath, onProgress) => _rawUpload(
          dio,
          member,
          ChatMediaApiPaths.uploadsImage,
          filePath,
          circleId: circle.id,
        ),
        uploadFile: (filePath, onProgress) => _rawUpload(
          dio,
          member,
          ChatMediaApiPaths.uploadsFile,
          filePath,
          circleId: circle.id,
        ),
        attach: (type, uploadId, idempotencyKey) => chat.sendMediaMessage(
          token: member.token,
          sessionId: member.sessionId,
          circleId: circle.id,
          type: type,
          uploadId: uploadId,
          idempotencyKey: idempotencyKey,
        ),
      );
      final rawVoice = VoiceNoteController(
        upload: (filePath, durationSeconds) => media.uploadVoice(
          token: member.token,
          sessionId: member.sessionId,
          filePath: filePath,
          durationSeconds: durationSeconds,
          circleId: circle.id,
        ),
        recorder: _ScriptedRecorder(voiceBytes),
        player: _LocalFilePreviewPlayer(voiceBytes),
      );
      final rawContainer = ProviderContainer(
        overrides: [
          voiceNoteControllerProvider(circle.id).overrideWith((_) => rawVoice),
          mediaAttachmentControllerProvider(circle.id)
              .overrideWith((_) => rawComposer),
        ],
      );
      addTearDown(rawContainer.dispose);
      await _pumpComposer(tester, circle.id, rawContainer);
      picker.nextImage = oversizedJpegFile;
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attach));
      await tester.pumpAndSettle();
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.attachImage));
      await tester.pumpAndSettle();
      await _waitFor(tester,
          () => rawComposer.state.phase == MediaAttachmentPhase.previewing);
      await tester.tap(find.bySemanticsLabel(ChatUiLabels.send));
      await _waitFor(tester,
          () => rawComposer.state.error == MediaAttachmentError.imageTooLarge);
      await tester.pump();
      expect(find.text(ChatUiLabels.uploadTooLargeImage), findsOneWidget);
      expect(find.bySemanticsLabel(ChatUiLabels.retry), findsNothing);
      expect(find.textContaining('ERR_'), findsNothing);

      // ── 12. Contract: malformed supported-media data must be 422 ────────
      final malformedVoiceFile = fixture(
        't052-broken.mp3',
        Uint8List.fromList(voiceBytes.take(4).toList() + Uint8List(300)),
      );
      final malformedJpegFile = fixture(
        't052-broken.jpg',
        Uint8List.fromList(
            [0xFF, 0xD8, 0xFF, 0xE0] + List<int>.filled(300, 0x00)),
      );
      final malformedPdfFile = fixture(
        't052-broken.pdf',
        asciiBytes('%PDF-1.4\n') + Uint8List(300),
      );
      await _expectChatError(
        () => media.uploadVoice(
          token: member.token,
          sessionId: member.sessionId,
          filePath: malformedVoiceFile,
          durationSeconds: 5,
          circleId: circle.id,
        ),
        {422},
        'malformed voice must be 422',
      );
      await _expectChatError(
        () => media.uploadImage(
          token: member.token,
          sessionId: member.sessionId,
          filePath: malformedJpegFile,
          circleId: circle.id,
        ),
        {422},
        'malformed JPEG must be 422',
      );
      await _expectChatError(
        () => media.uploadFile(
          token: member.token,
          sessionId: member.sessionId,
          filePath: malformedPdfFile,
          circleId: circle.id,
        ),
        {422},
        'malformed PDF must be 422',
      );

      // ── 13. Contract: Message responses carry the media projection ──────
      expect(voiceMessage.mediaUrl, isNotNull);
      expect(voiceMessage.voiceDurationSeconds, 2); // ceil(1.056 s)
      final projectionNow = DateTime.now();
      expect(
        voiceMessage.mediaUrlExpiresAt!
            .isAfter(projectionNow.add(const Duration(days: 6))),
        isTrue,
      );
      expect(
        voiceMessage.mediaUrlExpiresAt!
            .isBefore(projectionNow.add(const Duration(days: 8))),
        isTrue,
      );
      expect(imageMessage.mediaUrl, isNotNull);
      expect(pdfMessage.fileName, endsWith('.pdf'));
      expect(byId[voiceMessage.id]?.mediaUrl, isNotNull);
      expect(byId[imageMessage.id]?.mediaUrl, isNotNull);
      expect(byId[pdfMessage.id]?.mediaUrl, isNotNull);
      semantics.dispose();
    },
    timeout: const Timeout(Duration(minutes: 15)),
  );
}

Uint8List asciiBytes(String text) => Uint8List.fromList(utf8.encode(text));

/// Mirrors ChatMediaApiClient's multipart upload minus the client-side size
/// pre-check, so the server's own limit can be exercised end-to-end.
Future<ChatUploadResult> _rawUpload(
  Dio dio,
  _UserCredentials user,
  String path,
  String filePath, {
  String? circleId,
  String? dmPeerId,
  int? durationSeconds,
}) async {
  try {
    final response = await dio.post<Map<String, dynamic>>(
      path,
      data: FormData.fromMap({
        ChatJsonKeys.file: await MultipartFile.fromFile(filePath),
        if (circleId != null) ChatJsonKeys.circleId: circleId,
        if (dmPeerId != null) ChatJsonKeys.dmPeerId: dmPeerId,
        if (durationSeconds != null)
          ChatJsonKeys.durationSeconds: durationSeconds,
      }),
      options:
          Options(headers: sessionRequestHeaders(user.token, user.sessionId)),
    );
    return ChatUploadResult.fromJson(response.data!);
  } on DioException catch (error) {
    throw mapChatApiException(error);
  }
}

ChatMediaAccessController _accessController({
  required Future<ChatMediaAccess> Function() renew,
  required PreviewPlayer player,
}) =>
    ChatMediaAccessController(
      renew: renew,
      player: player,
      download: (url) async {
        final saved = await downloadChatPdf(url);
        return saved;
      },
    );

Future<void> _pumpComposer(
  WidgetTester tester,
  String circleId,
  ProviderContainer container,
) =>
    tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: Scaffold(
              body: ChatMediaComposerBar(circleId: circleId),
            ),
          ),
        ),
      ),
    );

Future<void> _pumpMediaBody(
  WidgetTester tester,
  ChatMessage message,
  ChatMediaAccessController access,
) =>
    tester.pumpWidget(
      ProviderScope(
        key: UniqueKey(),
        overrides: [
          chatMediaAccessControllerProvider(message.id)
              .overrideWith((_) => access),
        ],
        child: MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: Scaffold(
              body: ChatMediaMessageBody(message: message),
            ),
          ),
        ),
      ),
    );

Future<ChatApiException> _expectChatError(
  Future<Object?> Function() action,
  Set<int> expectedStatuses,
  String because,
) async {
  try {
    await action();
  } on ChatApiException catch (error) {
    expect(
      expectedStatuses.contains(error.statusCode),
      isTrue,
      reason: '$because (got ${error.statusCode} ${error.code})',
    );
    return error;
  }
  fail('$because — the request unexpectedly succeeded');
}

Future<void> _expectChatMediaLimit(
  Future<Object?> Function() action,
  ChatMediaLimitKind kind,
  String because,
) async {
  try {
    await action();
  } on ChatMediaLimitException catch (error) {
    expect(error.kind, kind, reason: because);
    return;
  }
  fail('$because — the upload was not refused locally');
}

Future<List<int>> _httpGetBytes(String url) async {
  final client = HttpClient();
  try {
    final response = await (await client.getUrl(Uri.parse(url))).close();
    expect(response.statusCode, 200,
        reason: 'presigned URL must serve content');
    return response
        .fold<List<int>>(<int>[], (all, chunk) => all..addAll(chunk));
  } finally {
    client.close(force: true);
  }
}

Future<List<int>?> _newestDownloadedPdf() async {
  final files = Directory.systemTemp
      .listSync()
      .whereType<File>()
      .where((file) => file.path.contains('halaqaty-chat-'))
      .toList()
    ..sort((a, b) => b.lastModifiedSync().compareTo(a.lastModifiedSync()));
  return files.isEmpty ? null : files.first.readAsBytesSync();
}

Future<void> _waitFor(
  WidgetTester tester,
  bool Function() condition, {
  Duration timeout = const Duration(seconds: 15),
}) async {
  final deadline = DateTime.now().add(timeout);
  while (!condition() && DateTime.now().isBefore(deadline)) {
    await Future<void>.delayed(_poll);
    await tester.pump();
  }
  // Flush one more frame: async state transitions can land between the
  // previous pump's build and the condition check.
  await tester.pump();
  expect(condition(), isTrue, reason: 'Timed out waiting for the media flow');
}

class _UserCredentials {
  const _UserCredentials({
    required this.token,
    required this.sessionId,
    required this.userId,
  });

  final String token;
  final String sessionId;
  final String userId;

  static _UserCredentials? fromEnv(Map<String, String> env, String role) {
    final token = env['T052_${role}_TOKEN'];
    final sessionId = env['T052_${role}_SESSION'];
    final userId = env['T052_${role}_USER_ID'];
    if (token == null || sessionId == null || userId == null) return null;
    return _UserCredentials(
      token: token,
      sessionId: sessionId,
      userId: userId,
    );
  }
}

/// `VoiceRecorder` seam: "records" the compliant fixture to the target path
/// and streams two amplitude samples so the waveform is genuinely drawn.
class _ScriptedRecorder implements VoiceRecorder {
  _ScriptedRecorder(this._audioBytes);

  final List<int> _audioBytes;
  final _amplitudes = StreamController<double>.broadcast();
  String? _path;

  @override
  Future<bool> hasPermission() async => true;

  @override
  Future<void> start(String filePath) async {
    _path = filePath;
  }

  @override
  Future<String?> stop() async {
    final path = _path;
    _path = null;
    if (path == null) return null;
    File(path).writeAsBytesSync(_audioBytes, flush: true);
    return path;
  }

  @override
  Stream<double> amplitudeStream() {
    // Emit after the controller's listener attaches (microtask), like the
    // real recorder's recurring amplitude events.
    scheduleMicrotask(() {
      _amplitudes.add(-30);
      _amplitudes.add(-12);
    });
    return _amplitudes.stream;
  }

  @override
  Future<void> dispose() => _amplitudes.close();
}

/// `PreviewPlayer` seam for the composer's foreground preview: proves the
/// previewed local file is exactly the recorded fixture.
class _LocalFilePreviewPlayer implements PreviewPlayer {
  _LocalFilePreviewPlayer(this._expectedBytes);

  final List<int> _expectedBytes;
  int previewedFiles = 0;

  @override
  Future<void> playToCompletion(String filePath) async {
    expect(File(filePath).readAsBytesSync(), _expectedBytes,
        reason: 'preview must play the recorded file');
    previewedFiles++;
  }

  @override
  Future<void> playUrlToCompletion(String url) async {
    throw StateError('Local preview player must not receive URLs');
  }

  @override
  Stream<Duration> get positionStream => const Stream.empty();

  @override
  Future<void> dispose() async {}
}

/// `PreviewPlayer` seam for received-media playback: fetches the renewed
/// presigned URL over real HTTP and asserts it serves the uploaded bytes —
/// the headless Linux target has no just_audio platform implementation, so
/// this verifies the same contract (URL reachable, content intact) at the
/// point where the plugin would render it.
class _HttpFetchPlayer implements PreviewPlayer {
  _HttpFetchPlayer(this._expectedBytes);

  final List<int> _expectedBytes;
  final List<String> playedUrls = [];

  @override
  Future<void> playToCompletion(String filePath) async {
    throw StateError('URL player must not receive local files');
  }

  @override
  Future<void> playUrlToCompletion(String url) async {
    playedUrls.add(url);
    final bytes = await _httpGetBytes(url);
    expect(bytes, _expectedBytes,
        reason: 'the renewed URL must serve the uploaded bytes');
  }

  @override
  Stream<Duration> get positionStream => const Stream.empty();

  @override
  Future<void> dispose() async {}
}

/// `ChatAttachmentPicker` seam standing in for the native image/PDF pickers,
/// which cannot run on the headless integration target.
class _AttachmentPicker implements ChatAttachmentPicker {
  String? nextImage;
  String? nextPdf;

  @override
  Future<String?> pickImage() async => nextImage;

  @override
  Future<String?> pickPdf() async => nextPdf;
}

// One second of genuine silence (ffmpeg anullsrc → libmp3lame, 32 kbps mono,
// ffprobe duration 1.056 s): small enough to commit, real enough for the
// server's ffprobe-based validation.
const _voiceMp3Base64 =
    'SUQzBAAAAAAACgAAAAAAAAAAAAD/84TAAAAAAAAAAAAASW5mbwAAAA8AAAAsAAARQAAQEBYWGxshISEmJiwsMjI3Nzc9PUJCSEhNTU1TU1lZXl5eZGRpaW9vdHR0enqAgIWFi4uLkJCWlpubm6GhpqasrLKysre3vb3CwsjIyM3N09PZ2dne3uTk6env7+/09Pr6//8AAAAATGF2ZiBsYW1lAAAAAAAAAAAAAAAAJAMAAAAAAAAAEUC6GYjQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD/80TEAAAAA0gAAAAATEFNRTMuMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TEUwAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TEpgAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAMTAwVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVMQU1FMy7/80TErAAAA0gAAAAAVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVX/80TErAAAA0gAAAAAVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVU=';

// 64×64 ffmpeg-generated JPEG (2 152 bytes).
const _imageJpegBase64 =
    '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';

// 64×64 ffmpeg-generated PNG (200 bytes).
const _imagePngBase64 =
    'iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAIAAAAlC+aJAAAACXBIWXMAAAABAAAAAQBPJcTWAAAAeklEQVR4nO3PUQkAIBTAwJfOYGYyoCH8OITBAtxm7fN1wwUNaEEDWtCAFjSgBQ1oQQNa0IAWNKAFDWhBA1rQgBY0oAUNaEEDWtCAFjSgBQ1oQQNa0IAWNKAFDWhBA1rQgBY0oAUNaEEDWtCAFjSgBQ1oQQNa0IAWPHYB5E7BWgv4ozIAAAAASUVORK5CYII=';

// Minimal qpdf-verified PDF (310 bytes).
const _filePdfBase64 =
    'JVBERi0xLjQKMSAwIG9iago8PC9UeXBlL0NhdGFsb2cvUGFnZXMgMiAwIFI+PgplbmRvYmoKMiAwIG9iago8PC9UeXBlL1BhZ2VzL0tpZHNbMyAwIFJdL0NvdW50IDE+PgplbmRvYmoKMyAwIG9iago8PC9UeXBlL1BhZ2UvUGFyZW50IDIgMCBSL01lZGlhQm94WzAgMCAyMDAgMjAwXT4+CmVuZG9iagp4cmVmCjAgNAowMDAwMDAwMDAwIDY1NTM1IGYgCjAwMDAwMDAwMDkgMDAwMDAgbiAKMDAwMDAwMDA1NCAwMDAwMCBuIAowMDAwMDAwMTA1IDAwMDAwIG4gCnRyYWlsZXIKPDwvU2l6ZSA0L1Jvb3QgMSAwIFI+PgpzdGFydHhyZWYKMTcwCiUlRU9GCg==';

// Two seconds of raw MPEG frames (no ID3/Xing container, ffmpeg
// -write_xing 0 -id3v2_version 0, 8 256 bytes, ffprobe duration 2.064 s):
// tiling this sample scales the probed duration linearly, which the
// >300 s server boundary test relies on.
const _voiceRawMp3Base64 =
    '//NExAAAAANIAAAAAExBTUUzLjEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExFMAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKYAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVTEFNRTMu//NExKwAAANIAAAAADEwMFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVV//NExKwAAANIAAAAAFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVV//NExKwAAANIAAAAAFVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVV';
