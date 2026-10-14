import Foundation
import Network
import Observation
import Darwin

@MainActor @Observable final class Discovery: NSObject {
    private(set) var isSearching = false
    private(set) var message: String?
    private var browser: NetServiceBrowser?
    private var services: [NetService] = []
    private var wantedID: String?
    private var found: (String, Int)?

    func start(id: String) {
        stop()
        wantedID = id
        found = nil
        message = nil
        let browser = NetServiceBrowser()
        browser.delegate = self
        self.browser = browser
        isSearching = true
        browser.searchForServices(ofType: "_neardrop._tcp.", inDomain: "local.")
    }

    func stop() {
        browser?.stop()
        browser = nil
        services.forEach { $0.stop() }
        services.removeAll()
        isSearching = false
    }

    func locate(_ computer: PairedComputer) async throws -> PairedComputer {
        start(id: computer.id)
        defer { stop() }
        // Bonjour handles DHCP changes. Fall back to the last working address
        // when multicast is blocked; the pinned certificate checks identity.
        for _ in 0..<15 {
            try await Task.sleep(for: .milliseconds(200))
            if let (host, port) = found {
                var updated = computer
                updated.host = host
                updated.port = port
                return updated
            }
        }
        return computer
    }
}

extension Discovery: @preconcurrency NetServiceBrowserDelegate, @preconcurrency NetServiceDelegate {
    func netServiceBrowser(_ browser: NetServiceBrowser, didFind service: NetService, moreComing: Bool) {
        services.append(service)
        service.delegate = self
        service.resolve(withTimeout: 3)
    }

    func netServiceBrowser(_ browser: NetServiceBrowser, didRemove service: NetService, moreComing: Bool) {
        services.removeAll { $0 == service }
    }

    func netServiceBrowser(_ browser: NetServiceBrowser, didNotSearch errorDict: [String: NSNumber]) {
        message = "Allow Local Network access in Settings → Apps → NearDrop, and connect both devices to the same network."
        isSearching = false
    }

    func netServiceDidResolveAddress(_ sender: NetService) {
        guard let data = sender.txtRecordData(),
              let identity = NetService.dictionary(fromTXTRecord: data)["id"],
              String(data: identity, encoding: .utf8) == wantedID,
              sender.port > 0, sender.port <= 65535 else { return }
        // Use a numeric address: the service's SRV hostname need not be served
        // by the Linux machine's resolver. Only private IPv4 is advertised.
        for address in sender.addresses ?? [] {
            let host: String? = address.withUnsafeBytes { buffer in
                guard let base = buffer.baseAddress, buffer.count >= MemoryLayout<sockaddr>.size else { return nil }
                let socket = base.assumingMemoryBound(to: sockaddr.self)
                guard socket.pointee.sa_family == sa_family_t(AF_INET) else { return nil }
                var output = [CChar](repeating: 0, count: Int(NI_MAXHOST))
                guard getnameinfo(socket, socklen_t(buffer.count), &output, socklen_t(output.count), nil, 0, NI_NUMERICHOST) == 0 else { return nil }
                return String(cString: output)
            }
            if let host, PairingCode.isPrivateIPv4(host) { found = (host, sender.port); return }
        }
    }
}
