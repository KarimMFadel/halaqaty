/// Arabic-first labels for the F-004 group chat presentation, ready for
/// extraction once a localization framework is adopted. English copy is
/// inlined at call sites via the `rtl ? arabic : english` repo pattern.
class ChatUiLabels {
  const ChatUiLabels._();

  static const title = 'المحادثة';
  static const loading = 'جارٍ تحميل المحادثة...';
  static const empty = 'لا توجد رسائل بعد؛ ابدأ المحادثة';
  static const historyError = 'تعذر تحميل المحادثة';
  static const retry = 'إعادة المحاولة';
  static const loadOlder = 'تحميل الرسائل الأقدم';
  static const actionFailed = 'تعذر تنفيذ الإجراء';
  static const composerHint = 'اكتب رسالة';
  static const send = 'إرسال';
  static const tooLong = 'الرسالة طويلة جدًا؛ الحد الأقصى 4000 حرف';
  static const statusPending = 'قيد الإرسال';
  static const statusSent = 'أُرسلت';
  static const statusDelivered = 'تم التسليم';
  static const statusRead = 'تمت القراءة';
  static const memberFallback = 'عضو';

  // Voice-note surface (FR-019); consumed by the T050/T051 widgets.
  static const recording = 'جارٍ التسجيل';
  static const recordingLimitReached = 'تم بلوغ الحد الأقصى للتسجيل (٥ دقائق)';
  static const micPermissionDenied = 'يتطلب التسجيل إذن الميكروفون';
  static const openSettings = 'فتح الإعدادات';
  static const previewVoiceNote = 'استماع';
  static const discardVoiceNote = 'تجاهل التسجيل';
  static const voiceSendFailed = 'تعذر إرسال التسجيل';
  static const voicePreviewFailed = 'تعذر تشغيل المعاينة';
  static const recordingInterrupted = 'تمت مقاطعة التسجيل';

  // Chat media composer surface (FR-020/FR-021); T050/T051 widgets.
  static const attach = 'إرفاق';
  static const attachImage = 'إرفاق صورة';
  static const attachPdf = 'إرفاق ملف PDF';
  static const recordVoiceNote = 'تسجيل رسالة صوتية';
  static const stopRecording = 'إيقاف التسجيل';
  static const sendRecording = 'إرسال التسجيل';
  static const recordingDuration = 'مدة التسجيل';
  static const waveform = 'مستوى الصوت';
  static const recorderFailed = 'تعذر التسجيل';
  static const sendingVoice = 'جارٍ إرسال التسجيل...';
  static const cancel = 'إلغاء';
  static const uploading = 'جارٍ الرفع';
  static const attaching = 'جارٍ إرفاق الرسالة...';
  static const uploadTooLargeVoice = 'حجم التسجيل يتجاوز ٢٠ ميجابايت';
  static const uploadTooLargeImage = 'حجم الصورة يتجاوز ٥ ميجابايت';
  static const uploadTooLargeFile = 'حجم الملف يتجاوز ١٠ ميجابايت';
  static const uploadUnsupportedType = 'نوع الملف غير مدعوم';
  static const uploadInvalid = 'تفاصيل الملف غير صالحة';
  static const uploadRateLimited = 'محاولات كثيرة؛ حاول لاحقًا';
  static const uploadNetworkError = 'تعذر الاتصال؛ حاول مجددًا';
  static const attachFailedMedia = 'تعذر إرفاق الوسائط بالرسالة';

  // Received-media surface (FR-024 renewable links); T050/T051 widgets.
  static const playVoiceMessage = 'تشغيل الرسالة الصوتية';
  static const voicePlaying = 'قيد التشغيل';
  static const loadingMedia = 'جارٍ التحضير...';
  static const mediaAccessDenied = 'لا تملك صلاحية الوصول إلى هذه الوسائط';
  static const mediaAccessFailed = 'تعذر تحميل الوسائط';
  static const linkExpired = 'انتهت صلاحية الرابط';
  static const renewLink = 'تجديد الرابط';
  static const downloadPdf = 'تنزيل الملف';
  static const downloaded = 'تم التنزيل';
  static const pdfFallbackName = 'ملف';
  static const imageAlt = 'صورة';

  static const titleEn = 'Chat';
  static const loadingEn = 'Loading chat history...';
  static const emptyEn = 'No messages yet; start the conversation';
  static const historyErrorEn = 'Could not load chat history';
  static const retryEn = 'Retry';
  static const actionFailedEn = 'Action failed';
  static const composerHintEn = 'Write a message';
  static const sendEn = 'Send';
  static const tooLongEn = 'Message is too long; limit is 4000 characters';
  static const loadOlderEn = 'Load older messages';
  static const memberFallbackEn = 'Member';
  static const statusPendingEn = 'Sending';
  static const statusSentEn = 'Sent';
  static const statusDeliveredEn = 'Delivered';
  static const statusReadEn = 'Read';

  // Voice-note surface (FR-019); consumed by the T050/T051 widgets.
  static const recordingEn = 'Recording';
  static const recordingLimitReachedEn =
      'Recording limit reached (5 minutes)';
  static const micPermissionDeniedEn = 'Recording needs microphone permission';
  static const openSettingsEn = 'Open settings';
  static const previewVoiceNoteEn = 'Preview';
  static const discardVoiceNoteEn = 'Discard recording';
  static const voiceSendFailedEn = 'Could not send the recording';
  static const voicePreviewFailedEn = 'Could not play the preview';
  static const recordingInterruptedEn = 'Recording was interrupted';

  // Chat media composer surface (FR-020/FR-021); T050/T051 widgets.
  static const attachEn = 'Attach';
  static const attachImageEn = 'Attach photo';
  static const attachPdfEn = 'Attach PDF file';
  static const recordVoiceNoteEn = 'Record voice note';
  static const stopRecordingEn = 'Stop recording';
  static const sendRecordingEn = 'Send recording';
  static const recordingDurationEn = 'Recording duration';
  static const waveformEn = 'Sound level';
  static const recorderFailedEn = 'Recording failed';
  static const sendingVoiceEn = 'Sending recording...';
  static const cancelEn = 'Cancel';
  static const uploadingEn = 'Uploading';
  static const attachingEn = 'Attaching...';
  static const uploadTooLargeVoiceEn = 'Recording exceeds 20 MB';
  static const uploadTooLargeImageEn = 'Image exceeds 5 MB';
  static const uploadTooLargeFileEn = 'File exceeds 10 MB';
  static const uploadUnsupportedTypeEn = 'Unsupported file type';
  static const uploadInvalidEn = 'Invalid file details';
  static const uploadRateLimitedEn = 'Too many attempts; try again later';
  static const uploadNetworkErrorEn = 'Connection failed; try again';
  static const attachFailedMediaEn = 'Could not attach the media';

  // Received-media surface (FR-024 renewable links); T050/T051 widgets.
  static const playVoiceMessageEn = 'Play voice message';
  static const voicePlayingEn = 'Playing';
  static const loadingMediaEn = 'Preparing...';
  static const mediaAccessDeniedEn = 'You do not have access to this media';
  static const mediaAccessFailedEn = 'Could not load the media';
  static const linkExpiredEn = 'Link expired';
  static const renewLinkEn = 'Renew link';
  static const downloadPdfEn = 'Download file';
  static const downloadedEn = 'Downloaded';
  static const pdfFallbackNameEn = 'File';
  static const imageAltEn = 'Image';
}
