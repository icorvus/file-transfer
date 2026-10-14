import Foundation
import Testing
@testable import NearDrop

struct PairingTests {
    private let valid = "neardrop://pair?v=1&host=192.168.1.50&port=8787&id=" + String(repeating: "a", count: 32) + "&name=My%20PC&fp=" + String(repeating: "b", count: 64) + "&code=" + String(repeating: "c", count: 64)

    @Test func parsesPairing() throws {
        let pairing = try PairingCode(valid)
        #expect(pairing.computer.host == "192.168.1.50")
        #expect(pairing.computer.name == "My PC")
        #expect(pairing.computer.token.isEmpty)
        #expect(try pairing.computer.url("/v1/files").absoluteString == "https://192.168.1.50:8787/v1/files")
    }

    @Test func rejectsDuplicateKeys() {
        #expect(throws: NearDropError.self) { try PairingCode(valid + "&host=10.0.0.1") }
    }

    @Test(arguments: ["8.8.8.8", "127.0.0.1", "example.com", "192.168.1.999", "10.1.1", "10.1.1.2.3", "169.254.1.1"])
    func rejectsNonLANHost(_ host: String) {
        #expect(throws: NearDropError.self) { try PairingCode(valid.replacingOccurrences(of: "192.168.1.50", with: host)) }
    }

    @Test func requiresCertificatePin() {
        #expect(throws: NearDropError.self) { try PairingCode(valid.replacingOccurrences(of: String(repeating: "b", count: 64), with: "bad")) }
    }

    @Test func hashesFileBytes() async throws {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try Data("abc".utf8).write(to: url)
        defer { try? FileManager.default.removeItem(at: url) }
        let (hash, size) = try await FileHash.sha256(url)
        #expect(hash == "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
        #expect(size == 3)
    }

    @Test func pairingKeychainRoundTrip() throws {
        // Exercise the actual entitlement and persistence path, not a mock.
        // Preserve an existing pairing if these tests run on a used simulator.
        let previous = try PairingStore.load()
        defer {
            if let previous { try? PairingStore.save(previous) }
            else { try? PairingStore.remove() }
        }
        var computer = try PairingCode(valid).computer
        computer.token = String(repeating: "d", count: 64)
        computer.deviceId = String(repeating: "e", count: 32)
        try PairingStore.save(computer)
        let loaded = try PairingStore.load()
        #expect(loaded?.id == computer.id)
        #expect(loaded?.token == computer.token)
        #expect(loaded?.deviceId == computer.deviceId)
        try PairingStore.remove()
        #expect(try PairingStore.load() == nil)
    }
}
