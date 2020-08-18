//
//  PsiphonNotice.swift
//  PsiphonTunnel
//
//  Created by user on 2020-08-06.
//  Copyright © 2020 Psiphon Inc. All rights reserved.
//

import Foundation

let PsiphonNoticeErrorDomain = "com.psiphon3.ios.PsiphonNoticeErrorDomain"

enum PsiphonNoticeErrorCode: Int, CaseIterable {

    /// TODO: manually set codes?

    /// TODO
    case encodeUtf8Failed

    /// TODO
    case decodeUtf8Failed

    /// TODO
    case decodeJsonFailed

    /// TODO
    case encodeJsonFailed

    /// TODO
    case dataMissing

}

@objc final class PsiphonDateFormatter : DateFormatter {
    @objc static func rfc3339Formatter() -> DateFormatter {
        let rfc3339Formatter = DateFormatter()
        let enUSPOSIXLocale = Locale(identifier: "en_US_POSIX")
        rfc3339Formatter.locale = enUSPOSIXLocale

        rfc3339Formatter.dateFormat = "yyyy'-'MM'-'dd'T'HH':'mm':'ss.SSSZZZZZ"
        rfc3339Formatter.timeZone = TimeZone(secondsFromGMT: 0)

        return rfc3339Formatter
    }
}

@objc final class PsiphonNotice : NSObject {
    @objc var noticeType: String
    @objc var data: [String: Any]?
    @objc var timestamp: String?

    @objc init(noticeType: String, data: [String: Any], timestamp: String) {
        self.noticeType = noticeType
        self.data = data
        self.timestamp = timestamp
        super.init()
    }

    @objc init?(jsonStr: String, err: NSErrorPointer) {

        err?.pointee = .none

        guard let jsonData = jsonStr.data(using: .utf8) else {
            err?.pointee = NSError(domain: PsiphonNoticeErrorDomain,
                                   code: PsiphonNoticeErrorCode.decodeUtf8Failed.rawValue,
                                   userInfo: .none)
            return nil
        }

        do {
            let o = try JSONSerialization.jsonObject(with: jsonData, options: .allowFragments)
            guard let dict = o as? [String: Any] else {
                return nil
            }
            guard let noticeType = dict["noticeType"] as? String else {
                return nil
            }
            self.noticeType = noticeType
            self.timestamp = dict["timestamp"] as? String
            self.data = dict["data"] as? [String: Any]
            super.init()
        } catch let error as NSError {
            err?.pointee = NSError(domain: PsiphonNoticeErrorDomain,
                                   code: PsiphonNoticeErrorCode.decodeJsonFailed.rawValue,
                                   userInfo: [NSLocalizedDescriptionKey: "Decoding JSON failed",
                                              NSUnderlyingErrorKey: error])
            return nil
        } catch {
            // Should never happen.
            err?.pointee = NSError(domain: PsiphonNoticeErrorDomain,
                                   code: PsiphonNoticeErrorCode.decodeJsonFailed.rawValue,
                                   userInfo: [NSLocalizedDescriptionKey: "Decoding JSON failed: "
                                                + error.localizedDescription])
            return nil
        }
    }

    @objc func toDiagnosticMessage(err: NSErrorPointer) -> PsiphonDiagnosticMessage? {

        guard let data = self.data else {
            err?.pointee = NSError(domain: PsiphonNoticeErrorDomain,
                                   code: PsiphonNoticeErrorCode.dataMissing.rawValue,
                                   userInfo: [NSUnderlyingErrorKey: "No data to encode diagnostic message"])

            return .none
        }

        do {
            let jsonData = try JSONSerialization.data(withJSONObject: data, options: .fragmentsAllowed)
            guard let dataStr = String.init(data: jsonData, encoding: .utf8) else {
                err?.pointee = NSError(domain: PsiphonNoticeErrorDomain,
                                       code: PsiphonNoticeErrorCode.encodeUtf8Failed.rawValue,
                                       userInfo: [NSUnderlyingErrorKey: "Failed to encode JSON data"])
                return .none
            }

            guard let timestamp = self.timestamp else {
                err?.pointee = NSError(domain: PsiphonNoticeErrorDomain,
                                       code: PsiphonNoticeErrorCode.dataMissing.rawValue,
                                       userInfo: [NSUnderlyingErrorKey: "Timestamp missing"])
                return .none
            }

            let diagnosticMessage = String(format: "%@: %@", self.noticeType, dataStr)

            return .some(PsiphonDiagnosticMessage(message: diagnosticMessage, timestamp: timestamp))
        } catch let error as NSError {
            err?.pointee = NSError(domain: PsiphonNoticeErrorDomain,
                                   code: PsiphonNoticeErrorCode.encodeJsonFailed.rawValue,
                                   userInfo: [NSLocalizedDescriptionKey: "Encoding JSON failed",
                                              NSUnderlyingErrorKey: error])
            return .none
        } catch {
            // Should never happen.
            err?.pointee = NSError(domain: PsiphonNoticeErrorDomain,
                                   code: PsiphonNoticeErrorCode.decodeJsonFailed.rawValue,
                                   userInfo: [NSLocalizedDescriptionKey: "Encoding JSON failed: " + error.localizedDescription])
            return .none
        }
    }
}
