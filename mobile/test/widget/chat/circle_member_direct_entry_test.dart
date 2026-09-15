import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_members_screen.dart';

void main() {
  test('direct entry only allows eligible counterpart roles', () {
    expect(
      canOpenDirectChat(
        currentUserId: 'me',
        currentRole: CircleRole.teacher,
        peerUserId: 'student',
        peerRole: CircleRole.student,
        isArchived: false,
      ),
      isTrue,
    );
    expect(
      canOpenDirectChat(
        currentUserId: 'me',
        currentRole: CircleRole.teacher,
        peerUserId: 'supervisor',
        peerRole: CircleRole.supervisor,
        isArchived: false,
      ),
      isFalse,
    );
  });

  test('direct entry rejects self and archived circles', () {
    expect(
      canOpenDirectChat(
        currentUserId: 'me',
        currentRole: CircleRole.student,
        peerUserId: 'me',
        peerRole: CircleRole.teacher,
        isArchived: false,
      ),
      isFalse,
    );
    expect(
      canOpenDirectChat(
        currentUserId: 'me',
        currentRole: CircleRole.student,
        peerUserId: 'teacher',
        peerRole: CircleRole.teacher,
        isArchived: true,
      ),
      isFalse,
    );
  });
}
