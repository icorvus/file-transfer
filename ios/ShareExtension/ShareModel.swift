import Foundation
import Observation
import UniformTypeIdentifiers

@MainActor @Observable final class ShareModel {
    enum Phase { case preparing, sending, complete, failed }
    var phase: Phase = .preparing
    var title = "Getting ready…"
    var detail = "Finding your computer"
    var progress: Double = 0
    var sent = 0
    var total = 0
    var computerName = "your PC"
    var errorMessage: String?
    private var providers: [NSItemProvider] = []
    private var ids: [String] = []
    private var pending: (index: Int, attachment: StagedAttachment, hash: String, bytes: Int64)?
    private var directory: URL?
    private var operation: Task<Void, Never>?
    private let discovery = Discovery()
    private var initialized = false

    var canRetry: Bool { directory != nil && total > 0 && total <= 100 }

    func begin(items: [NSExtensionItem]) {
        guard !initialized else { return }
        initialized = true
        providers = items.flatMap { $0.attachments ?? [] }
        total = providers.count
        ids = providers.map { _ in UUID().uuidString }
        guard total > 0 else { fail("No photos were shared. Select photos and try again."); return }
        guard total <= 100 else { fail("Send up to 100 items at a time. Split this selection into smaller batches."); return }
        guard providers.allSatisfy({ Attachments.representation(for: $0) != nil }) else {
            fail("This selection includes an unsupported item. Share photos and videos only."); return
        }
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("NearDrop-" + UUID().uuidString, isDirectory: true)
        do { try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true); directory = root }
        catch { fail("Not enough iPhone storage to prepare the transfer."); return }
        retry()
    }

    func retry() {
        guard operation == nil, canRetry else { return }
        phase = .preparing
        errorMessage = nil
        operation = Task { [weak self] in
            guard let self else { return }
            defer {
                self.operation = nil
                if Task.isCancelled { self.cleanup() }
            }
            do { try await self.send() }
            catch is CancellationError { }
            catch {
                if !Task.isCancelled { self.fail(error.localizedDescription) }
            }
        }
    }

    private func send() async throws {
        guard let saved = try PairingStore.load() else { throw NearDropError.noPairing }
        computerName = saved.name
        title = "Connecting to \(saved.name)…"
        let computer = try await discovery.locate(saved)
        let client = TransferClient(computer: computer)
        let status = try await client.status()
        try PairingStore.save(computer)
        guard let directory else { throw NearDropError.message("Close this sheet and share your photos again.") }
        while sent < total {
            try Task.checkCancellation()
            let index = sent
            phase = .preparing
            title = "Preparing \(index + 1) of \(total)…"
            detail = "Getting the file from Photos"
            if pending?.index != index {
                let attachment = try await Attachments.stage(providers[index], directory: directory)
                do {
                    let (hash, bytes) = try await FileHash.sha256(attachment.url)
                    guard bytes > 0 else { throw NearDropError.message("This file is empty. Open it in Photos and try again.") }
                    guard bytes <= status.maxBytes else {
                        throw NearDropError.message("\(attachment.name) exceeds the PC’s per-file size limit. Increase --max-mb on the receiver or send a smaller file.")
                    }
                    pending = (index, attachment, hash, bytes)
                } catch { try? FileManager.default.removeItem(at: attachment.url); throw error }
            }
            guard let file = pending else { throw NearDropError.invalidReceipt }
            phase = .sending
            title = "Sending \(index + 1) of \(total)"
            detail = file.attachment.name
            progress = Double(sent) / Double(total)
            let receipt = try await client.upload(file: file.attachment.url, name: file.attachment.name,
                id: ids[index], hash: file.hash, bytes: file.bytes) { [weak self] bytes, expected in
                Task { @MainActor in
                    guard let self, self.sent == index, self.phase == .sending else { return }
                    let fraction = expected > 0 ? min(1, Double(bytes) / Double(expected)) : 0
                    self.progress = (Double(index) + fraction) / Double(self.total)
                    if fraction == 1 { self.detail = "Confirming the file is saved on your PC…" }
                }
            }
            try Task.checkCancellation()
            sent += 1
            detail = receipt.name
            pending = nil
            try? FileManager.default.removeItem(at: file.attachment.url)
        }
        phase = .complete
        progress = 1
        title = total == 1 ? "Saved to \(computerName)" : "\(total) files saved to \(computerName)"
        detail = "Your files are on your PC."
        cleanup()
    }

    private func fail(_ message: String) {
        phase = .failed
        title = "Transfer paused"
        errorMessage = message
        detail = "\(sent) of \(total) saved. Keep your PC awake and both devices on the same network."
    }

    func cancel() {
        discovery.stop()
        if let operation {
            // The running task may still be staging or reading this file.
            // Its defer removes the directory after cancellation unwinds.
            operation.cancel()
        } else {
            // Failed transfers retain a staged file for Retry. If the user
            // closes the sheet instead, there is no running task to clean it.
            cleanup()
        }
    }
    func cleanup() {
        if let directory { try? FileManager.default.removeItem(at: directory) }
        directory = nil
    }
}
