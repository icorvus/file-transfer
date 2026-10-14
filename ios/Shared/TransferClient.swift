import CryptoKit
import Foundation
import Security

nonisolated final class PinnedSessionDelegate: NSObject, URLSessionDelegate, URLSessionTaskDelegate, Sendable {
    let fingerprint: String
    let progress: @Sendable (Int64, Int64) -> Void

    init(fingerprint: String, progress: @escaping @Sendable (Int64, Int64) -> Void) {
        self.fingerprint = fingerprint
        self.progress = progress
    }

    func urlSession(_ session: URLSession, didReceive challenge: URLAuthenticationChallenge,
                    completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        guard challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust else {
            completionHandler(.performDefaultHandling, nil); return
        }
        guard let trust = challenge.protectionSpace.serverTrust,
              let chain = SecTrustCopyCertificateChain(trust) as? [SecCertificate],
              let certificate = chain.first else {
            completionHandler(.cancelAuthenticationChallenge, nil); return
        }
        let data = SecCertificateCopyData(certificate) as Data
        let actual = SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
        guard actual == fingerprint else {
            completionHandler(.cancelAuthenticationChallenge, nil); return
        }
        // Trust is established out of band by the PC's QR, never by Bonjour.
        completionHandler(.useCredential, URLCredential(trust: trust))
    }

    func urlSession(_ session: URLSession, task: URLSessionTask,
                    willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest,
                    completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil) // Never send bearer credentials to a redirect target.
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didSendBodyData bytesSent: Int64,
                    totalBytesSent: Int64, totalBytesExpectedToSend: Int64) {
        progress(totalBytesSent, totalBytesExpectedToSend)
    }
}

nonisolated struct TransferClient: Sendable {
    let computer: PairedComputer

    private func session(waitForConnectivity: Bool = false, progress: @escaping @Sendable (Int64, Int64) -> Void = { _, _ in }) -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 30
        configuration.timeoutIntervalForResource = waitForConnectivity ? 45 : 1800
        configuration.waitsForConnectivity = waitForConnectivity
        configuration.httpMaximumConnectionsPerHost = 1
        configuration.urlCache = nil
        return URLSession(configuration: configuration,
                          delegate: PinnedSessionDelegate(fingerprint: computer.fingerprint, progress: progress),
                          delegateQueue: nil)
    }

    private func validate(_ data: Data, _ response: URLResponse) throws {
        guard let response = response as? HTTPURLResponse else { throw NearDropError.invalidReceipt }
        guard (200...299).contains(response.statusCode) else {
            struct Failure: Decodable { let error: String }
            let message = (try? JSONDecoder().decode(Failure.self, from: data).error) ?? "The PC could not save this file. Try again."
            throw NearDropError.server(response.statusCode, message)
        }
    }

    func pair(code: String, phoneName: String) async throws -> PairedComputer {
        var request = URLRequest(url: try computer.url("/v1/pair"))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(["code": code, "name": phoneName])
        let session = session(waitForConnectivity: true)
        defer { session.invalidateAndCancel() }
        let (data, response) = try await session.data(for: request)
        try validate(data, response)
        let paired = try JSONDecoder().decode(PairResponse.self, from: data)
        guard paired.id == computer.id, PairingCode.isHex(paired.token, count: 64),
              PairingCode.isHex(paired.deviceId, count: 32) else { throw NearDropError.invalidPairing }
        var result = computer
        result.name = paired.name
        result.deviceId = paired.deviceId
        result.token = paired.token
        return result
    }

    func status() async throws -> ReceiverStatus {
        var request = URLRequest(url: try computer.url("/v1/status"), timeoutInterval: 4)
        request.setValue("Bearer \(computer.token)", forHTTPHeaderField: "Authorization")
        let session = session()
        defer { session.invalidateAndCancel() }
        let (data, response) = try await session.data(for: request)
        try validate(data, response)
        let status = try JSONDecoder().decode(ReceiverStatus.self, from: data)
        guard status.id == computer.id else { throw NearDropError.invalidPairing }
        return status
    }

    func upload(file: URL, name: String, id: String, hash: String, bytes: Int64,
                progress: @escaping @Sendable (Int64, Int64) -> Void) async throws -> FileReceipt {
        var request = URLRequest(url: try computer.url("/v1/files"))
        request.httpMethod = "POST"
        request.setValue("Bearer \(computer.token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        request.setValue(Data(name.utf8).base64EncodedString(), forHTTPHeaderField: "X-Filename-Base64")
        request.setValue(id, forHTTPHeaderField: "X-Transfer-ID")
        request.setValue(hash, forHTTPHeaderField: "X-Content-SHA256")
        request.setValue(String(bytes), forHTTPHeaderField: "Content-Length")
        let session = session(progress: progress)
        defer { session.invalidateAndCancel() }
        let (data, response) = try await session.upload(for: request, fromFile: file)
        try validate(data, response)
        let receipt = try JSONDecoder().decode(FileReceipt.self, from: data)
        guard receipt.id == id, receipt.sha256 == hash, receipt.bytes == bytes, receipt.committed else {
            throw NearDropError.invalidReceipt
        }
        return receipt
    }
}

nonisolated enum FileHash {
    @concurrent static func sha256(_ url: URL) async throws -> (String, Int64) {
        let file = try FileHandle(forReadingFrom: url)
        defer { try? file.close() }
        var hash = SHA256(), count: Int64 = 0
        while let data = try file.read(upToCount: 1024 * 1024), !data.isEmpty {
            try Task.checkCancellation()
            hash.update(data: data)
            count += Int64(data.count)
        }
        return (hash.finalize().map { String(format: "%02x", $0) }.joined(), count)
    }
}
