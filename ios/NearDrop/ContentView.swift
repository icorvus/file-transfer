import AVFoundation
import SwiftUI

struct ContentView: View {
    @State private var computer: PairedComputer?
    @State private var pairingText = ""
    @State private var showingScanner = false
    @State private var busy = false
    @State private var message: String?
    @State private var reachable = false
    @State private var confirmingLink = false
    @State private var discovery = Discovery()

    var body: some View {
        NavigationStack {
            List {
                if let computer {
                    Section {
                        Label {
                            VStack(alignment: .leading, spacing: 5) {
                                Text(computer.name).font(.headline)
                                Text(reachable ? "Ready to receive" : "Paired computer")
                                    .font(.subheadline).foregroundStyle(.secondary)
                            }
                        } icon: { Image(systemName: "desktopcomputer").foregroundStyle(.tint) }
                        Button("Check connection", systemImage: "wifi") { checkConnection() }
                            .disabled(busy)
                    } header: { Text("Your computer") }
                    Section {
                        Label("Select photos in Photos", systemImage: "photo.on.rectangle")
                        Label("Tap Share, then NearDrop", systemImage: "square.and.arrow.up")
                        Label("Photos save directly to your PC", systemImage: "arrow.down.to.line")
                    } header: { Text("Send from the share sheet") }
                      footer: { Text("Add NearDrop to Favorites in the share sheet’s More menu. Keep the share sheet open until the transfer finishes.") }
                } else {
                    Section {
                        ContentUnavailableView {
                            Label("Your PC, one tap away", systemImage: "desktopcomputer")
                        } description: {
                            Text("Pair once. Then send photos from the share sheet, straight to your computer.")
                        }
                        .listRowBackground(Color.clear)
                    }
                }
                Section {
                    Button(computer == nil ? "Scan pairing code" : "Pair a different computer", systemImage: "qrcode.viewfinder") {
                        Task { await openScanner() }
                    }
                    .disabled(busy)
                    DisclosureGroup("Use a pairing link") {
                        TextField("Paste neardrop://pair?…", text: $pairingText, axis: .vertical)
                            .textInputAutocapitalization(.never).autocorrectionDisabled()
                            .lineLimit(3...6)
                        Button("Connect to PC") { connect(pairingText) }
                            .disabled(busy || pairingText.isEmpty)
                    }
                } header: { Text(computer == nil ? "Pair your PC" : "Pairing") }
                  footer: { Text("On your PC, open the NearDrop control page and choose Pair an iPhone. Connect both devices to the same local network.") }
                if busy {
                    Section { ProgressView("Connecting to your PC…") }
                }
                if let message {
                    Section { Text(message).foregroundStyle(.secondary).accessibilityLabel(message) }
                }
                if computer != nil {
                    Section {
                        Button("Forget this computer", role: .destructive) {
                            do { try PairingStore.remove(); computer = nil; reachable = false; message = nil }
                            catch { message = error.localizedDescription }
                        }.disabled(busy)
                    } footer: { Text("To revoke this iPhone’s access on the PC, use Unpair on the PC’s control page.") }
                }
            }
            .navigationTitle("NearDrop")
            .sheet(isPresented: $showingScanner) {
                ScannerView { code in
                    showingScanner = false
                    connect(code)
                }
            }
            .task {
                do { computer = try PairingStore.load() }
                catch { message = error.localizedDescription }
                if computer != nil { checkConnection() }
            }
            .onOpenURL { url in
                pairingText = url.absoluteString
                confirmingLink = true
            }
            .confirmationDialog("Pair the computer in this link?", isPresented: $confirmingLink, titleVisibility: .visible) {
                Button("Pair computer") { connect(pairingText) }
                Button("Cancel", role: .cancel) { pairingText = "" }
            } message: { Text("Only continue if you opened this pairing link from your PC’s NearDrop control page.") }
        }
    }

    private func openScanner() async {
        guard await AVCaptureDevice.requestAccess(for: .video) else {
            message = "Allow Camera access in Settings → Apps → NearDrop, or paste the pairing link instead."
            return
        }
        showingScanner = true
    }

    private func connect(_ raw: String) {
        guard !busy else { return }
        busy = true
        message = nil
        Task {
            defer { busy = false }
            do {
                let code = try PairingCode(raw)
                // Start Bonjour in the foreground app so iOS can present its
                // Local Network permission prompt before share-extension use.
                discovery.start(id: code.computer.id)
                defer { discovery.stop() }
                let paired = try await TransferClient(computer: code.computer).pair(code: code.code, phoneName: "My iPhone")
                try PairingStore.save(paired)
                computer = paired
                reachable = true
                pairingText = ""
                message = "Connected. You can now send photos from the share sheet."
            } catch {
                message = error.localizedDescription + " If the PC cannot be reached, check Local Network permission and your Wi-Fi connection."
            }
        }
    }

    private func checkConnection() {
        guard let computer, !busy else { return }
        busy = true
        message = nil
        Task {
            defer { busy = false }
            do {
                let located = try await discovery.locate(computer)
                _ = try await TransferClient(computer: located).status()
                try PairingStore.save(located)
                self.computer = located
                reachable = true
                message = "Your PC is ready. Open Photos and share with NearDrop."
            } catch {
                reachable = false
                message = "Couldn’t reach your PC. Keep NearDrop running on the PC, allow Local Network access, and check that both devices use the same network.\n\n" + error.localizedDescription
            }
        }
    }
}
