import Foundation
import Security

// Xcode installs share an explicit keychain group. iloader re-signs bundle IDs
// and replaces entitlements, but gives the app and all extensions one App Group
// named group.<resigned main bundle ID>. App Groups are also keychain groups.
// Resolve that signed group at runtime so cloud builds need no Apple identity.
nonisolated enum PairingStore {
    private static var accessGroup: String {
        get throws {
            var containingApp = Bundle.main
            if Bundle.main.bundleURL.pathExtension == "appex",
               let bundle = Bundle(url: Bundle.main.bundleURL.deletingLastPathComponent().deletingLastPathComponent()) {
                containingApp = bundle
            }
            if let identifier = containingApp.bundleIdentifier {
                let sideloadGroup = "group." + identifier
                if FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: sideloadGroup) != nil {
                    return sideloadGroup
                }
            }
            guard let group = Bundle.main.object(forInfoDictionaryKey: "NearDropKeychainGroup") as? String,
                  !group.isEmpty, !group.contains("$(") else {
                throw NearDropError.message("The app and Share extension must be signed together. Install the complete NearDrop IPA, keeping its Share extension.")
            }
            return group
        }
    }

    private static var query: [String: Any] {
        get throws {
            return [kSecClass as String: kSecClassGenericPassword,
                    kSecAttrService as String: "NearDrop.PairedComputer.v1",
                    kSecAttrAccount as String: "receiver",
                    kSecAttrAccessGroup as String: try accessGroup]
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
