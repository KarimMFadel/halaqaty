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
}
