//
//  PsiphonConfig.swift
//  PsiphonTunnel
//
//  Created by user on 2020-08-06.
//  Copyright © 2020 Psiphon Inc. All rights reserved.
//

import Foundation

// TODO: fileprivate?
let PsiphonConfigErrorDomain = "com.psiphon3.ios.PsiphonConfigErrorDomain"

enum PsiphonConfigErrorCode: Int, CaseIterable {

    /// TODO
    case decodeFailed

    /// TODO
    case encodeFailed

}

@objc final class PsiphonConfig : NSObject {

    @objc static func decode(configStr: String, err: NSErrorPointer) -> [String: Any]? {
        guard let data = (configStr as String).data(using: .utf8) else {
            return .none
        }
        do {
            let o = try JSONSerialization.jsonObject(with: data, options: .allowFragments)
            guard let d = o as? [String: Any] else {
                err?.pointee = NSError(domain: PsiphonConfigErrorDomain,
                                       code: PsiphonConfigErrorCode.decodeFailed.rawValue,
                                       userInfo: [NSLocalizedDescriptionKey: "Unexpected config type: \(type(of: o))"])
                return .none
            }
            return .some(d)
        } catch let error as NSError {
            err?.pointee = NSError(domain: PsiphonConfigErrorDomain,
                                   code: PsiphonConfigErrorCode.decodeFailed.rawValue,
                                   userInfo: [NSLocalizedDescriptionKey: "Decoding config failed",
                                              NSUnderlyingErrorKey: error])
            return .none
        } catch {
            // Should never happen.
            err?.pointee = NSError(domain: PsiphonConfigErrorDomain,
                                   code: PsiphonConfigErrorCode.decodeFailed.rawValue,
                                   userInfo: [NSLocalizedDescriptionKey: "Decoding config failed: " + error.localizedDescription])
            return .none
        }
    }

    @objc static func encode(config: [String: Any], err: NSErrorPointer) -> String? {
        do {
            let data = try JSONSerialization.data(withJSONObject: config, options: .fragmentsAllowed)
            return String(data: data, encoding: .utf8)
        } catch let error as NSError {
            err?.pointee = NSError(domain: PsiphonConfigErrorDomain,
                                   code: PsiphonConfigErrorCode.encodeFailed.rawValue,
                                   userInfo: [NSLocalizedDescriptionKey: "Encoding config failed",
                                              NSUnderlyingErrorKey: error])
            return .none
        } catch {
            // Should never happen.
            err?.pointee = NSError(domain: PsiphonConfigErrorDomain,
                                   code: PsiphonConfigErrorCode.encodeFailed.rawValue,
                                   userInfo: [NSLocalizedDescriptionKey: "Encoding config failed: " + error.localizedDescription])
            return .none
        }
    }
}
