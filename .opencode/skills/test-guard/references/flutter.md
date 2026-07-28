# Flutter / Dart Testing Reference

Patterns and conventions for testing Flutter mobile code in Halaqaty.

## Test types and directories

| Type | Directory | Command | When to use |
|------|-----------|---------|-------------|
| Unit tests | `mobile/test/` | `flutter test` | Business logic, use-cases, repositories (with mocks), data classes |
| Widget tests | `mobile/test/` | `flutter test` | UI components, screens, widget behavior |
| Integration tests | `mobile/integration_test/` | `flutter test integration_test/` | End-to-end flows against a real/staging backend |

## Test file conventions

- Mirror the source structure: `lib/features/circle/domain/circle_service.dart` → `test/features/circle/domain/circle_service_test.dart`
- Every test file imports `package:flutter_test/flutter_test.dart`
- Use `group()` to organize tests by subject; use `test()` for individual cases
- Use `setUp()` and `tearDown()` for shared state within a `group()`

```dart
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

void main() {
  group('CircleService', () {
    late MockCircleRepository mockRepo;
    late CircleService sut;

    setUp(() {
      mockRepo = MockCircleRepository();
      sut = CircleService(repository: mockRepo);
    });

    test('createCircle: duplicate name throws DuplicateCircleException', () async {
      when(() => mockRepo.create(any())).thenThrow(DuplicateCircleException());

      expect(
        () => sut.createCircle(const CreateCircleRequest(name: 'Existing')),
        throwsA(isA<DuplicateCircleException>()),
      );
    });
  });
}
```

## Mocking with mocktail

Use `package:mocktail` (preferred over `mockito` for null-safe Dart — no code generation required).

```dart
// 1. Declare the mock class
class MockCircleRepository extends Mock implements CircleRepository {}
class MockFirebaseAuth extends Mock implements FirebaseAuth {}

// 2. Register fallback values for custom types (required by mocktail)
setUpAll(() {
  registerFallbackValue(const CreateCircleRequest(name: ''));
  registerFallbackValue(FakeCircle()); // a minimal fake implementation
});

// 3. Stub and verify
when(() => mockRepo.findById(any())).thenAnswer(
  (_) async => Circle(id: testId, name: 'Test Circle'),
);

verify(() => mockRepo.findById(testId)).called(1);
```

**Key rule**: mock only at system boundaries (Firebase Auth SDK, network client, MinIO client). Never mock a `freezed` data class, a domain model, or a local pure-function utility.

## Data classes: always construct real instances

```dart
// WRONG — mocking a freezed data class hides field errors
final mockCircle = MockCircle();
when(() => mockCircle.name).thenReturn('Test');

// CORRECT — construct a real instance
final circle = Circle(
  id: uuid.v4(),
  name: 'Test Circle',
  teacherId: testTeacherId,
  createdAt: DateTime.now(),
);
```

## Widget testing with WidgetTester

```dart
testWidgets('CircleCard: archived circle shows archived badge', (tester) async {
  final circle = Circle(
    id: uuid.v4(),
    name: 'Old Circle',
    status: CircleStatus.archived,
    teacherId: testTeacherId,
    createdAt: DateTime.now(),
  );

  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: CircleCard(circle: circle),
      ),
    ),
  );

  expect(find.text('Archived'), findsOneWidget);
  expect(find.byKey(const Key('circle_card_archive_badge')), findsOneWidget);
});
```

**`pump` vs `pumpAndSettle`**:
- `pump()` — processes one frame; use when you control timing
- `pumpAndSettle()` — runs frames until no more are scheduled; use for animations and async gaps
- `pump(Duration)` — advance the clock by a specific amount; use for timers and delayed state changes

## Testing Riverpod providers

If the project uses Riverpod for state management:

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

testWidgets('CircleListScreen: shows empty state when no circles', (tester) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        circleRepositoryProvider.overrideWithValue(MockCircleRepository()),
      ],
      child: const MaterialApp(home: CircleListScreen()),
    ),
  );

  await tester.pumpAndSettle();
  expect(find.text('No circles yet'), findsOneWidget);
});
```

For unit-testing notifiers/providers directly:

```dart
test('circleListNotifier: loads circles on init', () async {
  final container = ProviderContainer(
    overrides: [
      circleRepositoryProvider.overrideWithValue(mockRepo),
    ],
  );
  addTearDown(container.dispose);

  when(() => mockRepo.listAll()).thenAnswer((_) async => [testCircle]);

  final state = await container.read(circleListProvider.future);
  expect(state, [testCircle]);
});
```

## Testing BLoC / Cubit

If the project uses `flutter_bloc`:

```dart
import 'package:bloc_test/bloc_test.dart';

blocTest<CircleCubit, CircleState>(
  'emits [loading, loaded] when fetchCircles succeeds',
  build: () {
    when(() => mockRepo.listAll()).thenAnswer((_) async => [testCircle]);
    return CircleCubit(repository: mockRepo);
  },
  act: (cubit) => cubit.fetchCircles(),
  expect: () => [
    const CircleState.loading(),
    CircleState.loaded(circles: [testCircle]),
  ],
);

blocTest<CircleCubit, CircleState>(
  'emits [loading, error] when fetchCircles throws',
  build: () {
    when(() => mockRepo.listAll()).thenThrow(NetworkException());
    return CircleCubit(repository: mockRepo);
  },
  act: (cubit) => cubit.fetchCircles(),
  expect: () => [
    const CircleState.loading(),
    const CircleState.error(message: 'Network error'),
  ],
);
```

## Testing domain invariants (always justify these)

The following Halaqaty domain rules must always be tested:

```dart
group('QueueEntry validation', () {
  test('position must be positive', () {
    expect(
      () => QueueEntry(position: 0, studentId: testId),
      throwsA(isA<InvalidQueuePositionException>()),
    );
  });

  test('position 1 is valid', () {
    expect(
      () => QueueEntry(position: 1, studentId: testId),
      returnsNormally,
    );
  });
});

group('AyahRange validation', () {
  test('start ayah cannot exceed end ayah', () {
    expect(
      () => AyahRange(surahNumber: 2, startAyah: 10, endAyah: 5),
      throwsA(isA<InvalidAyahRangeException>()),
    );
  });

  test('ayah numbers are 1-based', () {
    expect(
      () => AyahRange(surahNumber: 1, startAyah: 0, endAyah: 7),
      throwsA(isA<InvalidAyahRangeException>()),
    );
  });
});
```

## Integration tests

Integration tests live in `mobile/integration_test/` and run against a real or staging backend. Use `package:integration_test` and `package:patrol` (if available).

```dart
// integration_test/circle_flow_test.dart
import 'package:integration_test/integration_test.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('Teacher can create a circle and student can join', (tester) async {
    app.main();
    await tester.pumpAndSettle();

    // sign in as teacher
    await tester.tap(find.byKey(const Key('google_sign_in_button')));
    await tester.pumpAndSettle();

    // create circle
    await tester.tap(find.byKey(const Key('create_circle_fab')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('circle_name_field')), 'Test Circle');
    await tester.tap(find.byKey(const Key('submit_button')));
    await tester.pumpAndSettle();

    expect(find.text('Test Circle'), findsOneWidget);
  });
}
```

## Test naming conventions

| Pattern | Example |
|---------|---------|
| `group` for subject | `group('CircleService', ...)` |
| `test` for scenario | `test('createCircle: empty name throws ValidationException', ...)` |
| `testWidgets` for UI | `testWidgets('CircleCard: shows member count badge', ...)` |
| `blocTest` description | `'emits [loading, loaded] when fetchCircles succeeds'` |

Format: `subject: scenario description` — read like a requirement sentence.

## Test helper conventions

- Put shared builders and fakes in `test/helpers/` or `test/fakes/`
- Name builder functions `makeCircle()`, `makeUser()`, `makeQueueEntry()` etc.
- Use `FakeX` classes (lightweight manual fakes) when mocktail is overkill for simple stubs
- Prefer `const` constructors for test data when possible to avoid object allocation noise

```dart
// test/helpers/test_factories.dart
Circle makeCircle({
  String name = 'Test Circle',
  CircleStatus status = CircleStatus.active,
  String? teacherId,
}) {
  return Circle(
    id: const Uuid().v4(),
    name: name,
    status: status,
    teacherId: teacherId ?? testTeacherId,
    createdAt: DateTime(2026, 1, 1),
  );
}
```

## Running tests

Use the project Makefiles. Run from the repo root or from `mobile/` directly:

```bash
# All unit + widget tests — from repo root or mobile/
make test
# or directly from mobile/: flutter test test

# Static analysis (must be clean before PR — zero issues)
make lint         # from root (runs flutter analyze + golangci-lint + spectral + gitleaks)
# or from mobile/: make analyze
# or directly: flutter analyze

# Install/update dependencies (after pubspec.yaml changes)
make deps         # from mobile/
# or directly: flutter pub get

# Build APK for internal testing
make build-apk    # from mobile/
# or directly: flutter build apk --debug

# With coverage report
flutter test test --coverage
genhtml coverage/lcov.info -o coverage/html

# Specific file
flutter test test/features/circle/domain/circle_service_test.dart

# Integration tests (requires connected device or emulator)
flutter test integration_test/
```
