import SwiftUI
import VisionKit

struct ScannerView: View {
    let onCode: (String) -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var error: String?

    var body: some View {
        NavigationStack {
            Group {
                if DataScannerViewController.isSupported && DataScannerViewController.isAvailable {
                    CodeScanner(onCode: onCode, onError: { error = $0 })
                        .overlay(alignment: .bottom) {
                            if let error {
                                Text(error).padding().background(.regularMaterial).padding()
                            }
                        }
                } else {
                    ContentUnavailableView("Camera unavailable", systemImage: "camera",
                                           description: Text("Close the scanner and paste the pairing link from your PC instead."))
                }
            }
            .navigationTitle("Scan your PC’s code")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } } }
        }
    }
}

private struct CodeScanner: UIViewControllerRepresentable {
    let onCode: (String) -> Void
    let onError: (String) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(onCode: onCode, onError: onError) }

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let scanner = DataScannerViewController(recognizedDataTypes: [.barcode(symbologies: [.qr])],
            qualityLevel: .balanced, recognizesMultipleItems: false, isHighFrameRateTrackingEnabled: false,
            isPinchToZoomEnabled: true, isGuidanceEnabled: true, isHighlightingEnabled: true)
        scanner.delegate = context.coordinator
        do { try scanner.startScanning() }
        catch { Task { @MainActor in onError(error.localizedDescription) } }
        return scanner
    }

    func updateUIViewController(_ controller: DataScannerViewController, context: Context) {}
    static func dismantleUIViewController(_ controller: DataScannerViewController, coordinator: Coordinator) { controller.stopScanning() }

    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        let onCode: (String) -> Void
        let onError: (String) -> Void
        private var delivered = false

        init(onCode: @escaping (String) -> Void, onError: @escaping (String) -> Void) {
            self.onCode = onCode
            self.onError = onError
        }

        func dataScanner(_ scanner: DataScannerViewController, didAdd addedItems: [RecognizedItem], allItems: [RecognizedItem]) {
            for item in addedItems { accept(item, scanner: scanner) }
        }
        func dataScanner(_ scanner: DataScannerViewController, didTapOn item: RecognizedItem) { accept(item, scanner: scanner) }
        func dataScanner(_ scanner: DataScannerViewController, becameUnavailableWithError error: DataScannerViewController.ScanningUnavailable) {
            onError("Camera scanning stopped. Close this screen and use the pairing link instead.")
        }
        private func accept(_ item: RecognizedItem, scanner: DataScannerViewController) {
            guard !delivered, case .barcode(let barcode) = item,
                  let code = barcode.payloadStringValue, code.hasPrefix("neardrop://pair?") else { return }
            delivered = true
            scanner.stopScanning()
            onCode(code)
        }
    }
}
