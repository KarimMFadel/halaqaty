import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';

class ProfileState {
  const ProfileState({
    this.profile,
    this.isLoading = false,
    this.isSaving = false,
    this.errorMessage,
    this.fieldErrors = const {},
  });

  final ProfileUser? profile;
  final bool isLoading;
  final bool isSaving;
  final String? errorMessage;
  final Map<String, String> fieldErrors;

  ProfileState copyWith({
    ProfileUser? profile,
    bool? isLoading,
    bool? isSaving,
    String? errorMessage,
    Map<String, String>? fieldErrors,
    bool clearErrorMessage = false,
    bool clearFieldErrors = false,
  }) {
    return ProfileState(
      profile: profile ?? this.profile,
      isLoading: isLoading ?? this.isLoading,
      isSaving: isSaving ?? this.isSaving,
      errorMessage: clearErrorMessage
          ? null
          : (errorMessage ?? this.errorMessage),
      fieldErrors:
          clearFieldErrors ? const {} : (fieldErrors ?? this.fieldErrors),
    );
  }
}

typedef ReadAuthState = AuthState Function();
typedef Logout = Future<void> Function();

class ProfileController extends StateNotifier<ProfileState> {
  ProfileController({
    required ProfileApiClient apiClient,
    required FirebaseAuth firebaseAuth,
    required ReadAuthState readAuthState,
    required Logout logout,
  })  : _apiClient = apiClient,
        _firebaseAuth = firebaseAuth,
        _readAuthState = readAuthState,
        _logout = logout,
        super(const ProfileState());

  final ProfileApiClient _apiClient;
  final FirebaseAuth _firebaseAuth;
  final ReadAuthState _readAuthState;
  final Logout _logout;

  Future<void> loadProfile() async {
    state = state.copyWith(
      isLoading: true,
      clearErrorMessage: true,
      clearFieldErrors: true,
    );

    try {
      final credentials = await _loadCredentials();
      if (credentials == null) {
        state = state.copyWith(
          isLoading: false,
          errorMessage: 'Session is missing. Please sign in again.',
        );
        return;
      }

      final profile = await _apiClient.getMe(
        firebaseIdToken: credentials.token,
        sessionId: credentials.sessionId,
      );
      state = state.copyWith(profile: profile, isLoading: false);
    } on DioException catch (e) {
      if (e.response?.statusCode == 401) {
        await _logout();
        state = state.copyWith(
          isLoading: false,
          errorMessage: 'Session expired. Please sign in again.',
        );
        return;
      }
      state = state.copyWith(
        isLoading: false,
        errorMessage: _extractErrorMessage(e) ?? e.message ?? 'Unknown error',
      );
    } catch (e) {
      state = state.copyWith(isLoading: false, errorMessage: e.toString());
    }
  }

  Future<bool> updateProfile({
    required UpdateProfileRequest request,
  }) async {
    state = state.copyWith(
      isSaving: true,
      clearErrorMessage: true,
      clearFieldErrors: true,
    );

    try {
      final credentials = await _loadCredentials();
      if (credentials == null) {
        state = state.copyWith(
          isSaving: false,
          errorMessage: 'Session is missing. Please sign in again.',
        );
        return false;
      }

      final profile = await _apiClient.updateMe(
        firebaseIdToken: credentials.token,
        sessionId: credentials.sessionId,
        request: request,
      );
      state = state.copyWith(
        profile: profile,
        isSaving: false,
        clearErrorMessage: true,
        clearFieldErrors: true,
      );
      return true;
    } on DioException catch (e) {
      if (e.response?.statusCode == 401) {
        await _logout();
        state = state.copyWith(
          isSaving: false,
          errorMessage: 'Session expired. Please sign in again.',
        );
        return false;
      }
      final fieldErrors = _extractFieldErrors(e);
      state = state.copyWith(
        isSaving: false,
        errorMessage: _extractErrorMessage(e) ?? e.message ?? 'Unknown error',
        fieldErrors: fieldErrors,
      );
      return false;
    } catch (e) {
      state = state.copyWith(isSaving: false, errorMessage: e.toString());
      return false;
    }
  }

  Future<_ProfileCredentials?> _loadCredentials() async {
    final firebaseUser = _firebaseAuth.currentUser;
    final sessionId = _readAuthState().sessionId;
    if (firebaseUser == null || sessionId == null) {
      return null;
    }
    final token = await firebaseUser.getIdToken();
    if (token == null || token.isEmpty) {
      return null;
    }
    return _ProfileCredentials(token: token, sessionId: sessionId);
  }

  String? _extractErrorMessage(DioException e) {
    final data = e.response?.data;
    if (data is! Map<String, dynamic>) {
      return null;
    }
    final error = data['error'];
    if (error is! Map<String, dynamic>) {
      return null;
    }
    return error['message'] as String?;
  }

  Map<String, String> _extractFieldErrors(DioException e) {
    final data = e.response?.data;
    if (data is! Map<String, dynamic>) {
      return const {};
    }
    final error = data['error'];
    if (error is! Map<String, dynamic>) {
      return const {};
    }
    final fields = error['fields'];
    if (fields is! Map) {
      return const {};
    }
    final mapped = <String, String>{};
    for (final entry in fields.entries) {
      final key = entry.key;
      final value = entry.value;
      if (key is String && value is String) {
        mapped[key] = value;
      }
    }
    return mapped;
  }
}

class _ProfileCredentials {
  const _ProfileCredentials({
    required this.token,
    required this.sessionId,
  });

  final String token;
  final String sessionId;
}

final profileControllerProvider =
    StateNotifierProvider<ProfileController, ProfileState>((ref) {
  return ProfileController(
    apiClient: ref.watch(profileApiClientProvider),
    firebaseAuth: ref.watch(firebaseAuthProvider),
    readAuthState: () => ref.read(authControllerProvider),
    logout: () => ref.read(authControllerProvider.notifier).logout(),
  );
});
