import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

enum CircleJoinFailure {
  invalidInvite,
  alreadyMember,
  full,
  archived,
  membershipLimit,
  privateCircle,
  sessionExpired,
  network,
  unknown,
}

class CircleDiscoveryState {
  const CircleDiscoveryState({
    this.myCircles = const [],
    this.publicCircles = const [],
    this.nextCursor,
    this.isLoading = false,
    this.joiningCircleId,
    this.failure,
  });

  final List<CircleSummary> myCircles;
  final List<CircleSummary> publicCircles;
  final String? nextCursor;
  final bool isLoading;
  final String? joiningCircleId;
  final CircleJoinFailure? failure;

  CircleDiscoveryState copyWith({
    List<CircleSummary>? myCircles,
    List<CircleSummary>? publicCircles,
    String? nextCursor,
    bool? isLoading,
    String? joiningCircleId,
    CircleJoinFailure? failure,
    bool clearJoining = false,
    bool clearFailure = false,
    bool clearNextCursor = false,
  }) {
    return CircleDiscoveryState(
      myCircles: myCircles ?? this.myCircles,
      publicCircles: publicCircles ?? this.publicCircles,
      nextCursor: clearNextCursor ? null : (nextCursor ?? this.nextCursor),
      isLoading: isLoading ?? this.isLoading,
      joiningCircleId:
          clearJoining ? null : (joiningCircleId ?? this.joiningCircleId),
      failure: clearFailure ? null : (failure ?? this.failure),
    );
  }
}

class CircleDiscoveryController extends StateNotifier<CircleDiscoveryState> {
  CircleDiscoveryController({
    required CircleApiClient apiClient,
    required Future<String?> Function() loadFirebaseIdToken,
    required AuthState Function() readAuthState,
    required Future<void> Function() logout,
  })  : _apiClient = apiClient,
        _loadFirebaseIdToken = loadFirebaseIdToken,
        _readAuthState = readAuthState,
        _logout = logout,
        super(const CircleDiscoveryState());

  static final _inviteCode = RegExp(r'^HLQ-[A-HJ-NP-Z2-9]{4}$');

  final CircleApiClient _apiClient;
  final Future<String?> Function() _loadFirebaseIdToken;
  final AuthState Function() _readAuthState;
  final Future<void> Function() _logout;

  Future<void> loadMyCircles() async {
    state = state.copyWith(isLoading: true, clearFailure: true);
    try {
      final credentials = await _credentials();
      if (credentials == null) return;
      final circles = await _apiClient.listCircles(
        firebaseIdToken: credentials.$1,
        sessionId: credentials.$2,
      );
      state = state.copyWith(
        myCircles: circles,
        isLoading: false,
        clearFailure: true,
      );
    } on FirebaseAuthException {
      _fail(CircleJoinFailure.sessionExpired);
    } on DioException catch (error) {
      _fail(await _failureFrom(error));
    }
  }

  Future<void> discover({String? query, String? cursor}) async {
    state = state.copyWith(isLoading: true, clearFailure: true);
    try {
      final credentials = await _credentials();
      if (credentials == null) return;
      final page = await _apiClient.discoverCircles(
        firebaseIdToken: credentials.$1,
        sessionId: credentials.$2,
        query: query?.trim(),
        cursor: cursor,
      );
      state = state.copyWith(
        publicCircles: page.circles,
        nextCursor: page.nextCursor,
        clearNextCursor: page.nextCursor == null,
        isLoading: false,
        clearFailure: true,
      );
    } on FirebaseAuthException {
      _fail(CircleJoinFailure.sessionExpired);
    } on DioException catch (error) {
      _fail(await _failureFrom(error));
    }
  }

  Future<bool> joinPublic(CircleSummary circle) async {
    state = state.copyWith(
      joiningCircleId: circle.id,
      clearFailure: true,
    );
    try {
      final credentials = await _credentials();
      if (credentials == null) return false;
      final joined = await _apiClient.joinPublicCircle(
        firebaseIdToken: credentials.$1,
        sessionId: credentials.$2,
        circleId: circle.id,
      );
      state = state.copyWith(
        myCircles: _addCircle(state.myCircles, joined.toSummary()),
        publicCircles: state.publicCircles
            .where((item) => item.id != circle.id)
            .toList(growable: false),
        clearJoining: true,
        clearFailure: true,
      );
      return true;
    } on FirebaseAuthException {
      _fail(CircleJoinFailure.sessionExpired);
    } on DioException catch (error) {
      _fail(await _failureFrom(error));
    }
    return false;
  }

  Future<bool> joinInvite(String input) async {
    final inviteCode = normalizeInvite(input);
    if (inviteCode == null) {
      _fail(CircleJoinFailure.invalidInvite);
      return false;
    }
    state = state.copyWith(joiningCircleId: inviteCode, clearFailure: true);
    try {
      final credentials = await _credentials();
      if (credentials == null) return false;
      final joined = await _apiClient.joinCircleByInvite(
        firebaseIdToken: credentials.$1,
        sessionId: credentials.$2,
        inviteCode: inviteCode,
      );
      state = state.copyWith(
        myCircles: _addCircle(state.myCircles, joined.toSummary()),
        clearJoining: true,
        clearFailure: true,
      );
      return true;
    } on FirebaseAuthException {
      _fail(CircleJoinFailure.sessionExpired);
    } on DioException catch (error) {
      _fail(await _failureFrom(error));
    }
    return false;
  }

  String? normalizeInvite(String input) {
    final value = input.trim();
    final code = value.toUpperCase();
    if (_inviteCode.hasMatch(code)) return code;
    final uri = Uri.tryParse(value);
    if (uri == null ||
        uri.scheme.toLowerCase() != 'https' ||
        uri.host.toLowerCase() != 'halaqaty.app' ||
        uri.pathSegments.length != 2 ||
        uri.pathSegments.first.toLowerCase() != 'join') {
      return null;
    }
    final linkCode = uri.pathSegments.last.toUpperCase();
    return _inviteCode.hasMatch(linkCode) ? linkCode : null;
  }

  Future<(String, String)?> _credentials() async {
    final sessionId = _readAuthState().sessionId;
    final token = await _loadFirebaseIdToken();
    if (token == null ||
        token.isEmpty ||
        sessionId == null ||
        sessionId.isEmpty) {
      _fail(CircleJoinFailure.sessionExpired);
      return null;
    }
    return (token, sessionId);
  }

  Future<CircleJoinFailure> _failureFrom(DioException error) async {
    if (error.response?.statusCode == 401) {
      await _logout();
      return CircleJoinFailure.sessionExpired;
    }
    final responseBody = error.response?.data;
    final envelope =
        responseBody is Map<String, dynamic> ? responseBody['error'] : null;
    final message = envelope is Map ? envelope['message'] : null;
    return switch (message) {
      'user is already a circle member' => CircleJoinFailure.alreadyMember,
      'circle has reached its maximum capacity' => CircleJoinFailure.full,
      'circle is archived' => CircleJoinFailure.archived,
      'user has reached the maximum of 5 circles' =>
        CircleJoinFailure.membershipLimit,
      'circle is private' => CircleJoinFailure.privateCircle,
      _ when error.response?.statusCode == 404 =>
        CircleJoinFailure.invalidInvite,
      _ when error.type == DioExceptionType.connectionError ||
              error.type == DioExceptionType.connectionTimeout =>
        CircleJoinFailure.network,
      _ => CircleJoinFailure.unknown,
    };
  }

  List<CircleSummary> _addCircle(
    List<CircleSummary> circles,
    CircleSummary joined,
  ) =>
      [
        ...circles.where((circle) => circle.id != joined.id),
        joined,
      ];

  void _fail(CircleJoinFailure failure) {
    state = state.copyWith(
      isLoading: false,
      failure: failure,
      clearJoining: true,
    );
  }
}

final circleDiscoveryControllerProvider = StateNotifierProvider<
    CircleDiscoveryController, CircleDiscoveryState>((ref) {
  return CircleDiscoveryController(
    apiClient: ref.watch(circleApiClientProvider),
    loadFirebaseIdToken: () =>
        ref.read(firebaseAuthProvider).currentUser?.getIdToken() ??
        Future<String?>.value(),
    readAuthState: () => ref.read(authControllerProvider),
    logout: ref.read(authControllerProvider.notifier).logout,
  );
});
