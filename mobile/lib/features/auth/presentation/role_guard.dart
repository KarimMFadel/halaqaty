import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/admin/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';

bool isCircleManagerRole(CircleRole role) {
  return role == CircleRole.teacher || role == CircleRole.supervisor;
}

class RoleGuard extends ConsumerWidget {
  const RoleGuard({
    required this.allowedRoles,
    required this.currentRole,
    required this.child,
    super.key,
    this.unauthenticatedChild,
    this.unauthorizedChild,
  });

  final Set<CircleRole> allowedRoles;
  final CircleRole? currentRole;
  final Widget child;
  final Widget? unauthenticatedChild;
  final Widget? unauthorizedChild;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final authState = ref.watch(authControllerProvider);
    if (!authState.isAuthenticated) {
      return unauthenticatedChild ?? const SizedBox.shrink();
    }
    final role = currentRole;
    if (role == null || !allowedRoles.contains(role)) {
      return unauthorizedChild ?? const SizedBox.shrink();
    }
    return child;
  }
}
