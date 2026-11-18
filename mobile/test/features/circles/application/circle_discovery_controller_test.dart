import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

void main() {
  late CircleDiscoveryController controller;

  setUp(() {
    controller = CircleDiscoveryController(
      apiClient: CircleApiClient(Dio()),
      loadFirebaseIdToken: () async => 'firebase-token',
      readAuthState: () => const AuthState(sessionId: 'session-id'),
      logout: () async {},
    );
  });

  test('normalizeInvite: accepts a legacy 4-symbol invite code', () {
    expect(controller.normalizeInvite('HLQ-7X2K'), 'HLQ-7X2K');
  });

  test('normalizeInvite: accepts a new 12-symbol invite code', () {
    expect(
      controller.normalizeInvite('HLQ-7X2KABCDEFGH'),
      'HLQ-7X2KABCDEFGH',
    );
  });

  test('normalizeInvite: accepts a join link with a 12-symbol code', () {
    expect(
      controller.normalizeInvite('https://halaqaty.app/join/HLQ-7X2KABCDEFGH'),
      'HLQ-7X2KABCDEFGH',
    );
  });

  test('normalizeInvite: rejects 5, 11, and 13 symbol codes', () {
    expect(controller.normalizeInvite('HLQ-7X2KA'), isNull);
    expect(controller.normalizeInvite('HLQ-7X2KABCDEFG'), isNull);
    expect(controller.normalizeInvite('HLQ-7X2KABCDEFGHX'), isNull);
  });

  test('normalizeInvite: rejects symbols outside the invite alphabet', () {
    expect(controller.normalizeInvite('HLQ-7X2I'), isNull); // letter I
    expect(controller.normalizeInvite('HLQ-7X2O'), isNull); // letter O
    expect(controller.normalizeInvite('HLQ-7X20'), isNull); // digit 0
    expect(controller.normalizeInvite('HLQ-7X21'), isNull); // digit 1
  });
}
