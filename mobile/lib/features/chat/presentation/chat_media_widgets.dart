import 'dart:async';
import 'dart:io';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_media_access_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/media_attachment_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/voice_note_controller.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';

String _label(BuildContext context, String ar, String en) =>
    Directionality.of(context) == TextDirection.rtl ? ar : en;
String _duration(Duration duration) =>
    '${duration.inMinutes}:${(duration.inSeconds % 60).toString().padLeft(2, '0')}';
Widget _action(String label, VoidCallback? onPressed) => Semantics(
    button: true,
    label: label,
    child: SizedBox(
        height: 48,
        child: TextButton(
            onPressed: onPressed,
            child: ExcludeSemantics(child: Text(label)))));
Widget _status(String label) => Semantics(liveRegion: true, child: Text(label));

/// The recording and attachment controls belong to the circle's provider scope.
class ChatMediaComposerBar extends ConsumerStatefulWidget {
  const ChatMediaComposerBar({super.key, required this.circleId});
  final String circleId;
  @override
  ConsumerState<ChatMediaComposerBar> createState() =>
      _ChatMediaComposerBarState();
}

class _ChatMediaComposerBarState extends ConsumerState<ChatMediaComposerBar>
    with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed) {
      unawaited(ref
          .read(voiceNoteControllerProvider(widget.circleId).notifier)
          .interrupt());
    }
  }

  @override
  Widget build(BuildContext context) {
    final voice = ref.watch(voiceNoteControllerProvider(widget.circleId));
    final attachment =
        ref.watch(mediaAttachmentControllerProvider(widget.circleId));
    final recorder =
        ref.read(voiceNoteControllerProvider(widget.circleId).notifier);
    final media =
        ref.read(mediaAttachmentControllerProvider(widget.circleId).notifier);
    String l(String ar, String en) => _label(context, ar, en);
    Future<void> sendVoice() async {
      final upload = await recorder.send();
      if (upload != null && mounted) await media.attachVoice(upload);
    }

    if (attachment.phase != MediaAttachmentPhase.idle) {
      return _AttachmentPanel(state: attachment, controller: media);
    }
    final recording = voice.phase == VoiceNotePhase.recording;
    final hasNote = [
      VoiceNotePhase.recorded,
      VoiceNotePhase.previewing,
      VoiceNotePhase.failed
    ].contains(voice.phase);
    return Column(mainAxisSize: MainAxisSize.min, children: [
      if (recording || hasNote) ...[
        Semantics(
            container: true,
            label: l(ChatUiLabels.recordingDuration,
                ChatUiLabels.recordingDurationEn),
            child: ExcludeSemantics(child: Text(_duration(voice.duration)))),
        Semantics(
            container: true,
            label: l(ChatUiLabels.waveform, ChatUiLabels.waveformEn),
            child: SizedBox(
                height: 40,
                width: 240,
                child: CustomPaint(
                    painter: _Waveform(voice.amplitudes,
                        Theme.of(context).colorScheme.primary)))),
      ],
      if (voice.phase == VoiceNotePhase.permissionDenied) ...[
        _status(l(ChatUiLabels.micPermissionDenied,
            ChatUiLabels.micPermissionDeniedEn)),
        Text(l(ChatUiLabels.openSettings, ChatUiLabels.openSettingsEn)),
      ],
      if (voice.failure != null)
        _status(switch (voice.failure!) {
          VoiceNoteFailure.send =>
            l(ChatUiLabels.voiceSendFailed, ChatUiLabels.voiceSendFailedEn),
          VoiceNoteFailure.preview => l(ChatUiLabels.voicePreviewFailed,
              ChatUiLabels.voicePreviewFailedEn),
          VoiceNoteFailure.recorder => l(ChatUiLabels.recordingInterrupted,
              ChatUiLabels.recordingInterruptedEn),
        }),
      if (voice.phase == VoiceNotePhase.sending)
        _status(l(ChatUiLabels.sendingVoice, ChatUiLabels.sendingVoiceEn)),
      Wrap(alignment: WrapAlignment.center, children: [
        if (recording)
          _action(l(ChatUiLabels.stopRecording, ChatUiLabels.stopRecordingEn),
              () => unawaited(recorder.stopRecording())),
        if (hasNote) ...[
          _action(
              l(ChatUiLabels.previewVoiceNote, ChatUiLabels.previewVoiceNoteEn),
              voice.phase == VoiceNotePhase.recorded
                  ? () => unawaited(recorder.startPreview())
                  : null),
          _action(
              l(ChatUiLabels.discardVoiceNote, ChatUiLabels.discardVoiceNoteEn),
              () => unawaited(recorder.discard())),
          if (voice.failure == VoiceNoteFailure.send &&
              voice.failureIsRetryable)
            _action(l(ChatUiLabels.retry, ChatUiLabels.retryEn),
                () => unawaited(sendVoice()))
          else
            _action(
                l(ChatUiLabels.sendRecording, ChatUiLabels.sendRecordingEn),
                voice.phase == VoiceNotePhase.recorded
                    ? () => unawaited(sendVoice())
                    : null),
        ],
        if (!recording &&
            !hasNote &&
            voice.phase != VoiceNotePhase.sending) ...[
          _action(
              l(ChatUiLabels.recordVoiceNote, ChatUiLabels.recordVoiceNoteEn),
              voice.phase == VoiceNotePhase.requestingPermission
                  ? null
                  : () => unawaited(recorder.startRecording())),
          _action(
              l(ChatUiLabels.attach, ChatUiLabels.attachEn),
              () => showModalBottomSheet<void>(
                  context: context,
                  builder: (context) => SafeArea(
                          child:
                              Column(mainAxisSize: MainAxisSize.min, children: [
                        _action(
                            l(ChatUiLabels.attachImage,
                                ChatUiLabels.attachImageEn), () {
                          Navigator.pop(context);
                          unawaited(media.pickImage());
                        }),
                        _action(
                            l(ChatUiLabels.attachPdf, ChatUiLabels.attachPdfEn),
                            () {
                          Navigator.pop(context);
                          unawaited(media.pickPdf());
                        }),
                      ])))),
        ],
      ]),
    ]);
  }
}

class _AttachmentPanel extends StatelessWidget {
  const _AttachmentPanel({required this.state, required this.controller});
  final MediaAttachmentState state;
  final MediaAttachmentController controller;
  @override
  Widget build(BuildContext context) {
    String l(String ar, String en) => _label(context, ar, en);
    final busy = state.phase == MediaAttachmentPhase.uploading ||
        state.phase == MediaAttachmentPhase.sending;
    return Column(mainAxisSize: MainAxisSize.min, children: [
      if (state.kind == MediaAttachmentKind.image && state.filePath != null)
        Image.file(File(state.filePath!),
            height: 100, errorBuilder: (_, __, ___) => const Icon(Icons.image)),
      if (state.kind != null && state.fileName != null) Text(state.fileName!),
      if (busy) ...[
        _status(state.phase == MediaAttachmentPhase.uploading
            ? l(ChatUiLabels.uploading, ChatUiLabels.uploadingEn)
            : l(ChatUiLabels.attaching, ChatUiLabels.attachingEn)),
        if (state.phase == MediaAttachmentPhase.uploading)
          Text('${state.progressPercent}%'),
      ],
      if (state.error != null)
        _status(switch (state.error!) {
          MediaAttachmentError.voiceTooLong => l(
              ChatUiLabels.recordingLimitReached,
              ChatUiLabels.recordingLimitReachedEn),
          MediaAttachmentError.voiceTooLarge => l(
              ChatUiLabels.uploadTooLargeVoice,
              ChatUiLabels.uploadTooLargeVoiceEn),
          MediaAttachmentError.imageTooLarge => l(
              ChatUiLabels.uploadTooLargeImage,
              ChatUiLabels.uploadTooLargeImageEn),
          MediaAttachmentError.fileTooLarge => l(
              ChatUiLabels.uploadTooLargeFile,
              ChatUiLabels.uploadTooLargeFileEn),
          MediaAttachmentError.unsupportedType => l(
              ChatUiLabels.uploadUnsupportedType,
              ChatUiLabels.uploadUnsupportedTypeEn),
          MediaAttachmentError.invalid =>
            l(ChatUiLabels.uploadInvalid, ChatUiLabels.uploadInvalidEn),
          MediaAttachmentError.rateLimited =>
            l(ChatUiLabels.uploadRateLimited, ChatUiLabels.uploadRateLimitedEn),
          MediaAttachmentError.network => l(ChatUiLabels.uploadNetworkError,
              ChatUiLabels.uploadNetworkErrorEn),
          MediaAttachmentError.attachFailed =>
            l(ChatUiLabels.attachFailedMedia, ChatUiLabels.attachFailedMediaEn),
        }),
      if (!busy)
        Wrap(children: [
          _action(
              l(ChatUiLabels.cancel, ChatUiLabels.cancelEn), controller.cancel),
          if (state.phase == MediaAttachmentPhase.previewing)
            _action(l(ChatUiLabels.send, ChatUiLabels.sendEn),
                () => unawaited(controller.confirmSend())),
          if (state.errorIsRetryable)
            _action(
                l(ChatUiLabels.retry, ChatUiLabels.retryEn),
                () => unawaited(state.kind == null
                    ? controller.retryVoiceAttach()
                    : controller.retry())),
        ]),
    ]);
  }
}

class _Waveform extends CustomPainter {
  _Waveform(this.samples, this.color);
  final List<double> samples;
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 2;
    for (var i = 0; i < samples.length; i++) {
      final height =
          math.max(2.0, ((samples[i] + 60) / 60).clamp(0, 1) * size.height);
      final x = i * size.width / math.max(samples.length, 1);
      canvas.drawLine(Offset(x, (size.height - height) / 2),
          Offset(x, (size.height + height) / 2), paint);
    }
  }

  @override
  bool shouldRepaint(_Waveform oldDelegate) =>
      oldDelegate.samples != samples || oldDelegate.color != color;
}

/// A received attachment uses renewable, access-controlled playback/download.
class ChatMediaMessageBody extends ConsumerStatefulWidget {
  const ChatMediaMessageBody({super.key, required this.message});
  final ChatMessage message;
  @override
  ConsumerState<ChatMediaMessageBody> createState() =>
      _ChatMediaMessageBodyState();
}

class _ChatMediaMessageBodyState extends ConsumerState<ChatMediaMessageBody>
    with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed) {
      unawaited(ref
          .read(chatMediaAccessControllerProvider(widget.message.id).notifier)
          .stop());
    }
  }

  @override
  Widget build(BuildContext context) {
    final message = widget.message;
    final state = ref.watch(chatMediaAccessControllerProvider(message.id));
    final controller =
        ref.read(chatMediaAccessControllerProvider(message.id).notifier);
    String l(String ar, String en) => _label(context, ar, en);
    if (state.phase == ChatMediaAccessPhase.denied) {
      return _status(
          l(ChatUiLabels.mediaAccessDenied, ChatUiLabels.mediaAccessDeniedEn));
    }
    final busy = state.phase == ChatMediaAccessPhase.loading ||
        state.phase == ChatMediaAccessPhase.playing;
    final url = state.access?.url ?? message.mediaUrl;
    final expiry = state.access?.expiresAt ?? message.mediaUrlExpiresAt;
    return Column(mainAxisSize: MainAxisSize.min, children: [
      if (expiry != null && expiry.isBefore(DateTime.now()))
        Text(l(ChatUiLabels.linkExpired, ChatUiLabels.linkExpiredEn)),
      if (state.phase == ChatMediaAccessPhase.loading)
        _status(l(ChatUiLabels.loadingMedia, ChatUiLabels.loadingMediaEn)),
      if (state.phase == ChatMediaAccessPhase.failed)
        _status(l(
            ChatUiLabels.mediaAccessFailed, ChatUiLabels.mediaAccessFailedEn)),
      if (message.type == ChatMessageType.voice) ...[
        Text(_duration(Duration(seconds: message.voiceDurationSeconds ?? 0))),
        if (state.phase == ChatMediaAccessPhase.playing)
          _status(l(ChatUiLabels.voicePlaying, ChatUiLabels.voicePlayingEn)),
        _action(
            l(ChatUiLabels.playVoiceMessage, ChatUiLabels.playVoiceMessageEn),
            busy ? null : () => unawaited(controller.play())),
      ],
      if (message.type == ChatMessageType.image) ...[
        Semantics(
            label: l(ChatUiLabels.imageAlt, ChatUiLabels.imageAltEn),
            child: url == null
                ? const Icon(Icons.image)
                : Image.network(url,
                    height: 160,
                    errorBuilder: (_, __, ___) =>
                        const Icon(Icons.broken_image))),
        _action(l(ChatUiLabels.renewLink, ChatUiLabels.renewLinkEn),
            busy ? null : () => unawaited(controller.renew())),
      ],
      if (message.type == ChatMessageType.file) ...[
        Text(message.fileName ??
            l(ChatUiLabels.pdfFallbackName, ChatUiLabels.pdfFallbackNameEn)),
        _action(l(ChatUiLabels.downloadPdf, ChatUiLabels.downloadPdfEn),
            busy ? null : () => unawaited(controller.download())),
        if (state.phase == ChatMediaAccessPhase.downloaded)
          _status(l(ChatUiLabels.downloaded, ChatUiLabels.downloadedEn)),
      ],
    ]);
  }
}
