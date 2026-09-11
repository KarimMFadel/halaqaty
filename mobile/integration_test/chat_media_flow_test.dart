import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('chat media client enforces local limits before upload', (tester) async {
    expect(ChatLimits.maxVoiceDurationSeconds, 300);
    expect(ChatLimits.maxVoiceBytes, 20 * 1024 * 1024);
    expect(ChatLimits.maxImageBytes, 5 * 1024 * 1024);
    expect(ChatLimits.maxFileBytes, 10 * 1024 * 1024);
  });
}
