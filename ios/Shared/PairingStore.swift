import Foundation
import Security

// Both targets use the same explicitly configured Keychain access group. This
// avoids a plaintext token in UserDefaults and does not need an App Group.
nonisolated enum PairingStore {
    private static var query: [String: Any] {
        get throws {
            guard let group = Bundle.main.object(forInfoDictionaryKey: "NearDropKeychainGroup") as? String,
                  !group.isEmpty, !group.contains("$(") else {
                throw NearDropError.message("Set your Apple signing team in Xcode before installing NearDrop.")
            }
            return [kSecClass as String: kSecClassGenericPassword,
                    kSecAttrService as String: "NearDrop.PairedComputer.v1",
                    kSecAttrAccount as String: "receiver",
                    kSecAttrAccessGroup as String: group]
        }
    }

    static func load() throws -> PairedComputer? {
        var request = try query
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(request as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let data = result as? Data else { throw NearDropError.keychain(status) }
        return try JSONDecoder().decode(PairedComputer.self, from: data)
    }

    static func save(_ computer: PairedComputer) throws {
        let request = try query
        let data = try JSONEncoder().encode(computer)
        let changes: [String: Any] = [kSecValueData as String: data,
                                    kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly]
        var status = SecItemUpdate(request as CFDictionary, changes as CFDictionary)
        if status == errSecItemNotFound {
            status = SecItemAdd(request.merging(changes) { _, new in new } as CFDictionary, nil)
        }
        guard status == errSecSuccess else { throw NearDropError.keychain(status) }
    }

    static func remove() throws {
        let status = SecItemDelete(try query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else { throw NearDropError.keychain(status) }
    }
}
