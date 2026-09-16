import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/data/livekit_media_session.dart';

final mediaSessionProvider =
    Provider<MediaSession>((ref) => LiveKitMediaSession());
