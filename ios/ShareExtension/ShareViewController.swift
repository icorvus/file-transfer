import SwiftUI
import UIKit

final class ShareViewController: UIViewController {
    private let model = ShareModel()

    override func viewDidLoad() {
        super.viewDidLoad()
        let host = UIHostingController(rootView: ShareView(model: model, finish: { [weak self] in
            self?.model.cancel()
            self?.extensionContext?.completeRequest(returningItems: nil)
        }, cancel: { [weak self] in
            self?.model.cancel()
            self?.extensionContext?.cancelRequest(withError: NSError(domain: NSCocoaErrorDomain, code: NSUserCancelledError))
        }))
        addChild(host)
        view.addSubview(host.view)
        host.view.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            host.view.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            host.view.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            host.view.topAnchor.constraint(equalTo: view.topAnchor),
            host.view.bottomAnchor.constraint(equalTo: view.bottomAnchor)
        ])
        host.didMove(toParent: self)
        model.begin(items: extensionContext?.inputItems as? [NSExtensionItem] ?? [])
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        model.cancel()
    }
}

private struct ShareView: View {
    @Bindable var model: ShareModel
    let finish: () -> Void
    let cancel: () -> Void

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 24) {
                    Image(systemName: model.phase == .complete ? "checkmark.circle.fill" : "desktopcomputer")
                        .font(.system(size: 52, weight: .regular))
                        .foregroundStyle(model.phase == .complete ? Color.green : Color.accentColor)
                        .accessibilityHidden(true)
                    Text(model.title).font(.title2.bold()).multilineTextAlignment(.center)
                        .accessibilityAddTraits(.isHeader)
                    Text(model.detail).font(.body).foregroundStyle(.secondary).multilineTextAlignment(.center)
                    if model.phase == .sending {
                        ProgressView(value: model.progress)
                            .accessibilityLabel("Transfer progress")
                            .accessibilityValue(model.progress.formatted(.percent.precision(.fractionLength(0))))
                    } else if model.phase == .preparing {
                        ProgressView().accessibilityLabel("Preparing transfer")
                    }
                    if let error = model.errorMessage {
                        Text(error).font(.callout).foregroundStyle(.secondary).multilineTextAlignment(.center)
                    }
                    if model.phase == .failed && model.canRetry {
                        Button("Retry remaining files", systemImage: "arrow.clockwise", action: model.retry)
                            .buttonStyle(.borderedProminent).controlSize(.large)
                    }
                    if model.phase == .complete {
                        Button("Done", action: finish).buttonStyle(.borderedProminent).controlSize(.large)
                    } else {
                        Text("Keep this sheet open until the files are saved.")
                            .font(.footnote).foregroundStyle(.secondary).multilineTextAlignment(.center)
                    }
                }
                .padding(28)
                .frame(maxWidth: .infinity)
            }
            .navigationTitle("NearDrop")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                if model.phase != .complete {
                    ToolbarItem(placement: .cancellationAction) { Button("Cancel", action: cancel) }
                }
            }
        }
    }
}
