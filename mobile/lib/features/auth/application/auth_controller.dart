import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../data/auth_api_client.dart';

const _sessionIdKey = 'halaqaty_session_id';

enum AuthStatus {
  unknown,
  authenticated,
  unauthenticated,
}

class AuthState {
  const AuthState({
    this.status = AuthStatus.unknown,
    this.sessionId,
    this.user,
    this.errorMessage,
    this.isLoading = false,
  });

  final AuthStatus status;
  final String? sessionId;
  final BackendUser? user;
  final String? errorMessage;
  final bool isLoading;

  bool get isAuthenticated => status == AuthStatus.authenticated;

  AuthState copyWith({
    AuthStatus? status,
    String? sessionId,
    BackendUser? user,
    String? errorMessage,
    bool? isLoading,
    bool clearError = false,
    bool clearSession = false,
  }) {
    return AuthState(
      status: status ?? this.status,
      sessionId: clearSession ? null : (sessionId ?? this.sessionId),
      user: clearSession ? null : (user ?? this.user),
      errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
      isLoading: isLoading ?? this.isLoading,
    );
  }
}

class AuthController extends StateNotifier<AuthState> {
  AuthController({
    required AuthApiClient apiClient,
    required FirebaseAuth firebaseAuth,
    required FlutterSecureStorage secureStorage,
  })  : _apiClient = apiClient,
        _firebaseAuth = firebaseAuth,
        _secureStorage = secureStorage,
        super(const AuthState()) {
    _init();
  }

  final AuthApiClient _apiClient;
  final FirebaseAuth _firebaseAuth;
  final FlutterSecureStorage _secureStorage;

  Future<void> _init() async {
    final storedSessionId = await _secureStorage.read(key: _sessionIdKey);
    final firebaseUser = _firebaseAuth.currentUser;

    if (storedSessionId != null && firebaseUser != null) {
      state = state.copyWith(
        status: AuthStatus.authenticated,
        sessionId: storedSessionId,
      );
      return;
    }
    state = state.copyWith(status: AuthStatus.unauthenticated);
  }

  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async {
    state = state.copyWith(isLoading: true, clearError: true);
    try {
      await _firebaseAuth.createUserWithEmailAndPassword(
        email: email,
        password: password,
      );

      final token = await _requireFirebaseIDToken();
      if (token == null) {
        await _clearLocalSession();
        state = state.copyWith(
          errorMessage: 'Unable to get Firebase ID token.',
          isLoading: false,
        );
        return;
      }

      final session = await _apiClient.register(
        firebaseIdToken: token,
        request: RegisterRequest(
          displayName: displayName,
          preferredLanguage: preferredLanguage,
        ),
      );

      await _secureStorage.write(key: _sessionIdKey, value: session.sessionId);
      state = state.copyWith(
        status: AuthStatus.authenticated,
        sessionId: session.sessionId,
        user: session.user,
        isLoading: false,
      );
    } on FirebaseAuthException catch (e) {
      state = state.copyWith(
        isLoading: false,
        errorMessage: e.message ?? 'Firebase sign-up failed.',
      );
    } on DioException catch (e) {
      final replaySession = _sessionFromConflictReplay(e);
      if (replaySession != null) {
        await _secureStorage.write(
          key: _sessionIdKey,
          value: replaySession.sessionId,
        );
        state = state.copyWith(
          status: AuthStatus.authenticated,
          sessionId: replaySession.sessionId,
          user: replaySession.user,
          isLoading: false,
        );
        return;
      }
      await _handleDioError(e);
    } catch (e) {
      state = state.copyWith(isLoading: false, errorMessage: e.toString());
    }
  }

  Future<void> signIn({
    required String email,
    required String password,
  }) async {
    state = state.copyWith(isLoading: true, clearError: true);
    try {
      await _firebaseAuth.signInWithEmailAndPassword(
        email: email,
        password: password,
      );

      final token = await _requireFirebaseIDToken();
      if (token == null) {
        await _clearLocalSession();
        state = state.copyWith(
          errorMessage: 'Unable to get Firebase ID token.',
          isLoading: false,
        );
        return;
      }

      final session = await _apiClient.createSession(firebaseIdToken: token);
      await _secureStorage.write(key: _sessionIdKey, value: session.sessionId);

      state = state.copyWith(
        status: AuthStatus.authenticated,
        sessionId: session.sessionId,
        user: session.user,
        isLoading: false,
      );
    } on FirebaseAuthException catch (e) {
      state = state.copyWith(
        isLoading: false,
        errorMessage: e.message ?? 'Firebase sign-in failed.',
      );
    } on DioException catch (e) {
      await _handleDioError(e);
    } catch (e) {
      state = state.copyWith(isLoading: false, errorMessage: e.toString());
    }
  }

  Future<void> logout() async {
    state = state.copyWith(isLoading: true, clearError: true);
    try {
      final currentSessionID = state.sessionId;
      final token = await _requireFirebaseIDToken();
      if (token != null && currentSessionID != null) {
        await _apiClient.logout(
          firebaseIdToken: token,
          sessionId: currentSessionID,
        );
      }
    } on DioException {
      // Best-effort backend logout: always clear local auth state.
    } finally {
      await _clearLocalSession();
    }
  }

  Future<String?> _requireFirebaseIDToken() async {
    final firebaseUser = _firebaseAuth.currentUser;
    if (firebaseUser == null) {
      return null;
    }
    return firebaseUser.getIdToken();
  }

  Future<void> _clearLocalSession() async {
    await _secureStorage.delete(key: _sessionIdKey);
    await _firebaseAuth.signOut();
    state = const AuthState(status: AuthStatus.unauthenticated);
  }

  Future<void> _handleDioError(DioException e) async {
    final response = e.response;
    final statusCode = response?.statusCode;
    final data = response?.data;

    String? code;
    String? message;
    if (data is Map<String, dynamic>) {
      final error = data['error'];
      if (error is Map<String, dynamic>) {
        code = error['code'] as String?;
        message = error['message'] as String?;
      }
    }

    if (statusCode == 401) {
      await _clearLocalSession();
      return;
    }

    state = state.copyWith(
      isLoading: false,
      errorMessage: message ?? e.message ?? 'Unknown error',
    );
  }

  BackendSessionResponse? _sessionFromConflictReplay(DioException e) {
    if (e.response?.statusCode != 409) {
      return null;
    }
    final data = e.response?.data;
    if (data is! Map<String, dynamic>) {
      return null;
    }
    if (!data.containsKey('session_id') ||
        !data.containsKey('expires_at') ||
        !data.containsKey('user')) {
      return null;
    }
    try {
      return BackendSessionResponse.fromJson(data);
    } catch (_) {
      return null;
    }
  }
}

final firebaseAuthProvider = Provider<FirebaseAuth>((ref) {
  return FirebaseAuth.instance;
});

final flutterSecureStorageProvider = Provider<FlutterSecureStorage>((ref) {
  return const FlutterSecureStorage(
    aOptions: AndroidOptions(encryptedSharedPreferences: true),
  );
});

final authControllerProvider =
    StateNotifierProvider<AuthController, AuthState>((ref) {
  return AuthController(
    apiClient: ref.watch(authApiClientProvider),
    firebaseAuth: ref.watch(firebaseAuthProvider),
    secureStorage: ref.watch(flutterSecureStorageProvider),
  );
});
