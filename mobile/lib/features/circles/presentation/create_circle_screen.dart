import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/create_circle_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class CreateCircleScreen extends ConsumerStatefulWidget {
  const CreateCircleScreen({super.key, this.onCreated});

  final ValueChanged<CircleResponse>? onCreated;

  @override
  ConsumerState<CreateCircleScreen> createState() => _CreateCircleScreenState();
}

class _CreateCircleScreenState extends ConsumerState<CreateCircleScreen> {
  final _formKey = GlobalKey<FormState>();
  final _name = TextEditingController();
  final _description = TextEditingController();
  final _rules = TextEditingController();
  final _capacity = TextEditingController(text: '50');
  final _userSearch = TextEditingController();
  List<CircleUser> _results = const [];
  final List<CircleUser> _teachers = [];
  CircleUser? _backupSupervisor;
  bool _isPrivate = false;
  String _gender = 'unspecified';
  String _language = 'ar';

  @override
  void dispose() {
    _name.dispose();
    _description.dispose();
    _rules.dispose();
    _capacity.dispose();
    _userSearch.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(createCircleControllerProvider);
    final rtl = Directionality.of(context) == TextDirection.rtl;
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? 'إنشاء حلقة' : 'Create circle')),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Form(
            key: _formKey,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                TextFormField(
                  key: const Key('createCircleNameField'),
                  controller: _name,
                  maxLength: 100,
                  decoration: InputDecoration(
                    labelText: rtl ? 'اسم الحلقة' : 'Circle name',
                    errorText: state.fieldErrors['name'],
                  ),
                  validator: (value) => (value?.trim().isEmpty ?? true)
                      ? (rtl ? 'اسم الحلقة مطلوب' : 'Circle name is required')
                      : null,
                ),
                TextFormField(
                  key: const Key('createCircleUserSearchField'),
                  controller: _userSearch,
                  decoration: InputDecoration(
                    labelText: rtl ? 'ابحث عن مستخدم' : 'Search users',
                    hintText: rtl
                        ? 'اكتب حرفين على الأقل'
                        : 'Type at least 2 characters',
                  ),
                  onChanged: _searchUsers,
                ),
                for (final user in _results)
                  ListTile(
                    title: Text(user.displayName),
                    trailing: Wrap(
                      spacing: 4,
                      children: [
                        TextButton(
                          onPressed: () => _addTeacher(user),
                          child: Text(rtl ? 'معلّم' : 'Teacher'),
                        ),
                        TextButton(
                          onPressed: () => _setSupervisor(user),
                          child: Text(rtl ? 'مشرف' : 'Supervisor'),
                        ),
                      ],
                    ),
                  ),
                if (_teachers.isNotEmpty || _backupSupervisor != null)
                  Wrap(
                    spacing: 8,
                    children: [
                      for (final teacher in _teachers)
                        InputChip(
                          label: Text(
                              '${rtl ? 'معلّم' : 'Teacher'}: ${teacher.displayName}'),
                          onDeleted: () =>
                              setState(() => _teachers.remove(teacher)),
                        ),
                      if (_backupSupervisor != null)
                        InputChip(
                          label: Text(
                            '${rtl ? 'مشرف' : 'Supervisor'}: ${_backupSupervisor!.displayName}',
                          ),
                          onDeleted: () =>
                              setState(() => _backupSupervisor = null),
                        ),
                    ],
                  ),
                TextFormField(
                  controller: _description,
                  maxLength: 500,
                  decoration: InputDecoration(
                    labelText: rtl ? 'الوصف' : 'Description',
                    errorText: state.fieldErrors['description'],
                  ),
                ),
                TextFormField(
                  controller: _rules,
                  maxLength: 1000,
                  maxLines: 3,
                  decoration: InputDecoration(
                    labelText: rtl ? 'القواعد' : 'Rules',
                    errorText: state.fieldErrors['rules'],
                  ),
                ),
                TextFormField(
                  key: const Key('createCircleCapacityField'),
                  controller: _capacity,
                  keyboardType: TextInputType.number,
                  decoration: InputDecoration(
                    labelText: rtl ? 'السعة' : 'Capacity',
                    errorText: state.fieldErrors['max_capacity'],
                  ),
                  validator: (value) {
                    final capacity = int.tryParse(value ?? '');
                    return capacity == null || capacity < 2 || capacity > 200
                        ? (rtl ? 'السعة بين 2 و200' : 'Capacity must be 2–200')
                        : null;
                  },
                ),
                DropdownButtonFormField<String>(
                  key: const Key('createCircleGenderField'),
                  initialValue: _gender,
                  decoration: InputDecoration(
                    labelText: rtl ? 'الفئة' : 'Student audience',
                    errorText: state.fieldErrors['gender_restriction'],
                  ),
                  items: [
                    DropdownMenuItem(
                      value: 'unspecified',
                      child: Text(rtl ? 'غير محدد' : 'Unspecified'),
                    ),
                    DropdownMenuItem(
                      value: 'male',
                      child: Text(rtl ? 'ذكور' : 'Male'),
                    ),
                    DropdownMenuItem(
                      value: 'female',
                      child: Text(rtl ? 'إناث' : 'Female'),
                    ),
                    DropdownMenuItem(
                      value: 'mixed',
                      child: Text(rtl ? 'مختلط' : 'Mixed'),
                    ),
                  ],
                  onChanged: (value) =>
                      setState(() => _gender = value ?? 'unspecified'),
                ),
                DropdownButtonFormField<String>(
                  initialValue: _language,
                  decoration: InputDecoration(
                    labelText: rtl ? 'لغة الحلقة' : 'Circle language',
                    errorText: state.fieldErrors['language'],
                  ),
                  items: const [
                    DropdownMenuItem(value: 'ar', child: Text('العربية')),
                    DropdownMenuItem(value: 'en', child: Text('English')),
                  ],
                  onChanged: (value) =>
                      setState(() => _language = value ?? 'ar'),
                ),
                SwitchListTile(
                  key: const Key('createCirclePrivateField'),
                  title: Text(rtl ? 'حلقة خاصة' : 'Private circle'),
                  value: _isPrivate,
                  onChanged: (value) => setState(() => _isPrivate = value),
                ),
                const SizedBox(height: 16),
                ElevatedButton(
                  key: const Key('createCircleSubmitButton'),
                  onPressed: state.isSaving ? null : _submit,
                  child: state.isSaving
                      ? const CircularProgressIndicator()
                      : Text(rtl ? 'إنشاء الحلقة' : 'Create circle'),
                ),
                if (state.errorMessage != null && state.fieldErrors.isEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: Text(state.errorMessage!,
                        style: TextStyle(
                            color: Theme.of(context).colorScheme.error)),
                  ),
                if (state.circle?.inviteLink case final link?)
                  Padding(
                    padding: const EdgeInsets.only(top: 16),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(rtl ? 'تم إنشاء الحلقة' : 'Circle created'),
                        SelectableText(link),
                        TextButton(
                          onPressed: () => _copyInviteLink(link),
                          child: Text(
                              rtl ? 'نسخ رابط الدعوة' : 'Copy invite link'),
                        ),
                      ],
                    ),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    final created =
        await ref.read(createCircleControllerProvider.notifier).create(
              CreateCircleRequest(
                name: _name.text.trim(),
                description: _description.text.trim().isEmpty
                    ? null
                    : _description.text.trim(),
                rules: _rules.text.trim().isEmpty ? null : _rules.text.trim(),
                maxCapacity: int.parse(_capacity.text),
                isPrivate: _isPrivate,
                genderRestriction: _gender,
                language: _language,
                teacherUserIds:
                    _teachers.map((user) => user.id).toList(growable: false),
                backupSupervisorUserId: _backupSupervisor?.id,
              ),
            );
    if (mounted && created) {
      widget.onCreated?.call(
        ref.read(createCircleControllerProvider).circle!,
      );
    }
  }

  Future<void> _searchUsers(String query) async {
    if (query.trim().length < 2) {
      setState(() => _results = const []);
      return;
    }
    final results = await ref
        .read(createCircleControllerProvider.notifier)
        .searchUsers(query);
    if (mounted && _userSearch.text == query) {
      setState(() => _results = results);
    }
  }

  void _addTeacher(CircleUser user) {
    setState(() {
      if (!_teachers.any((teacher) => teacher.id == user.id)) {
        _teachers.add(user);
      }
      if (_backupSupervisor?.id == user.id) _backupSupervisor = null;
    });
  }

  void _setSupervisor(CircleUser user) {
    setState(() {
      _backupSupervisor = user;
      _teachers.removeWhere((teacher) => teacher.id == user.id);
    });
  }

  Future<void> _copyInviteLink(String link) async {
    await Clipboard.setData(ClipboardData(text: link));
    if (mounted) {
      final rtl = Directionality.of(context) == TextDirection.rtl;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
            content: Text(rtl ? 'تم نسخ رابط الدعوة' : 'Invite link copied')),
      );
    }
  }
}
