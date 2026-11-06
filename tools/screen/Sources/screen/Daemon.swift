import Foundation

enum Daemon {
    static let session: URLSession = {
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 1
        config.timeoutIntervalForResource = 1
        return URLSession(configuration: config)
    }()

    static func post(_ url: String, done: @escaping (Int?) -> Void = { _ in }) {
        guard let address = URL(string: url) else { fail("Invalid daemon URL: \(url)") }
        var request = URLRequest(url: address)
        request.httpMethod = "POST"
        session.dataTask(with: request) { _, response, error in
            let status = (response as? HTTPURLResponse)?.statusCode
            if let error { log("POST \(url): \(error)") }
            else if let status, !(200..<300).contains(status) { log("POST \(url): HTTP \(status)") }
            DispatchQueue.main.async { done(status) }
        }.resume()
    }
}
