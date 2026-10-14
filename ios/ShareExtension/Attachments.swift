import Foundation
import UniformTypeIdentifiers

nonisolated struct StagedAttachment: Sendable {
    let url: URL
    let name: String
}

@MainActor enum Attachments {
    static func representation(for provider: NSItemProvider) -> String? {
        provider.registeredTypeIdentifiers.first { identifier in
            guard let type = UTType(identifier) else { return false }
            return type.conforms(to: .image) || type.conforms(to: .movie)
        }
    }

    static func stage(_ provider: NSItemProvider, directory: URL) async throws -> StagedAttachment {
        guard let type = representation(for: provider) else {
            throw NearDropError.message("This item isn’t a photo or video. Share photos or videos from Photos or Files.")
        }
        let suggestedName = provider.suggestedName
        // The provider's URL expires when its callback returns. Copy inside the
        // callback, before resuming, and never load image bytes into UIImage.
        return try await withCheckedThrowingContinuation { continuation in
            provider.loadFileRepresentation(forTypeIdentifier: type) { url, error in
                guard let url else {
                    continuation.resume(throwing: error ?? NearDropError.message("Couldn’t prepare this photo. Download it from iCloud in Photos, then try again."))
                    return
                }
                do {
                    let ext = url.pathExtension.isEmpty ? (UTType(type)?.preferredFilenameExtension ?? "jpg") : url.pathExtension
                    var name = suggestedName.flatMap { $0.isEmpty ? nil : $0 } ?? url.lastPathComponent
                    if URL(fileURLWithPath: name).pathExtension.isEmpty { name += "." + ext }
                    let destination = directory.appendingPathComponent(UUID().uuidString).appendingPathExtension(ext)
                    let values = try url.resourceValues(forKeys: [.isRegularFileKey])
                    guard values.isRegularFile == true else {
                        throw NearDropError.message("This photo was supplied as a package. In Photos, share its still image or export the video separately.")
                    }
                    try FileManager.default.copyItem(at: url, to: destination)
                    continuation.resume(returning: StagedAttachment(url: destination, name: name))
                } catch { continuation.resume(throwing: error) }
            }
        }
    }
}
