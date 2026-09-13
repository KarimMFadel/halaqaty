import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:file_picker/file_picker.dart';
import 'package:image_picker/image_picker.dart';

/// Native picking seam (FR-021): like [VoiceRecorder]/[PreviewPlayer], the
/// platform pickers cannot run in tests, so presentation depends on this
/// pure interface and production wiring injects [NativeChatAttachmentPicker].
abstract class ChatAttachmentPicker {
  /// Returns the picked JPEG/PNG path, or null when the user cancelled.
  Future<String?> pickImage();

  /// Returns the picked PDF path, or null when the user cancelled.
  Future<String?> pickPdf();
}

/// [ChatAttachmentPicker] over the native image and document pickers.
class NativeChatAttachmentPicker implements ChatAttachmentPicker {
  @override
  Future<String?> pickImage() async {
    final photo = await ImagePicker().pickImage(
      source: ImageSource.gallery,
      // Downscale/re-encode on pick so typical camera photos fit the 5 MB
      // product limit; the server stays authoritative (FR-021).
      maxWidth: 2560,
      imageQuality: 85,
    );
    return photo?.path;
  }

  @override
  Future<String?> pickPdf() async {
    final result = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: ['pdf'],
      withData: false,
    );
    return result?.files.single.path;
  }
}

final chatAttachmentPickerProvider = Provider<ChatAttachmentPicker>(
  (ref) => NativeChatAttachmentPicker(),
);
