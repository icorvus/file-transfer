import Foundation

nonisolated struct PairedComputer: Codable, Sendable {
    var id: String
    var name: String
    var host: String
    var port: Int
    var fingerprint: String
    var token: String
    var deviceId: String

    func url(_ path: String) throws -> URL {
        var components = URLComponents()
        components.scheme = "https"
        components.host = host
        components.port = port
        components.path = path
        guard let url = components.url else { throw NearDropError.invalidPairing }
        return url
    }
}

nonisolated struct PairingCode: Sendable {
    let computer: PairedComputer
    let code: String

    init(_ string: String) throws {
        guard let parts = URLComponents(string: string.trimmingCharacters(in: .whitespacesAndNewlines)),
              parts.scheme == "neardrop", parts.host == "pair", parts.fragment == nil,
              let items = parts.queryItems else { throw NearDropError.invalidPairing }
        // Do not trap on malformed QR codes containing duplicate query keys.
        var fields: [String: String] = [:]
        for item in items {
            guard fields[item.name] == nil, let value = item.value else { throw NearDropError.invalidPairing }
            fields[item.name] = value
        }
        guard fields["v"] == "1", let host = fields["host"], Self.isPrivateIPv4(host),
              let portText = fields["port"], let port = Int(portText), (1...65535).contains(port),
              let id = fields["id"], Self.isHex(id, count: 32),
              let fingerprint = fields["fp"], Self.isHex(fingerprint, count: 64),
              let code = fields["code"], Self.isHex(code, count: 64),
              let name = fields["name"], !name.isEmpty, name.utf8.count <= 120 else {
            throw NearDropError.invalidPairing
        }
        self.code = code
        computer = PairedComputer(id: id, name: name, host: host, port: port,
                                  fingerprint: fingerprint, token: "", deviceId: "")
    }

    static func isHex(_ value: String, count: Int) -> Bool {
        value.utf8.count == count && value.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
    }

    static func isPrivateIPv4(_ value: String) -> Bool {
        let parts = value.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count == 4 else { return false }
        let octets = parts.compactMap { UInt8($0) }
        guard octets.count == 4 else { return false }
        return octets[0] == 10 || (octets[0] == 172 && (16...31).contains(octets[1])) || (octets[0] == 192 && octets[1] == 168)
    }
}

nonisolated struct PairResponse: Decodable, Sendable {
    let id: String
    let name: String
    let deviceId: String
    let token: String
    let maxBytes: Int64
}

nonisolated struct ReceiverStatus: Decodable, Sendable {
    let id: String
    let name: String
    let maxBytes: Int64
}

nonisolated struct FileReceipt: Decodable, Sendable {
    let id: String
    let name: String
    let sha256: String
    let bytes: Int64
    let committed: Bool
}

nonisolated enum NearDropError: LocalizedError, Sendable {
    case invalidPairing
    case noPairing
    case keychain(Int32)
    case message(String)
    case server(Int, String)
    case invalidReceipt

    var errorDescription: String? {
        switch self {
        case .invalidPairing: "This isn’t a valid NearDrop pairing code. Create a new code on your PC."
        case .noPairing: "Open the NearDrop app and pair your PC first, then share your photos again."
        case .keychain(let status): "Could not access your saved computer (Keychain \(status)). Check that the app and Share extension use the same signing team and keychain group."
        case .message(let message), .server(_, let message): message
        case .invalidReceipt: "The PC did not confirm the complete file. Tap Retry to check and resend safely."
        }
    }
}
