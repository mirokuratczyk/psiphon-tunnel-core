//
//  Error.swift
//  PsiphonTunnel
//
//  Created by user on 2020-08-17.
//  Copyright © 2020 Psiphon Inc. All rights reserved.
//

import Foundation

extension NSError {
    @objc func psi_toDescriptiveString() -> String {
        let desc = "\(self.domain).\(self.code): \(self.localizedDescription)"
        if let underlyingErr = self.userInfo[NSUnderlyingErrorKey] as? NSError {
            // TODO: note about inf. recursion
            return "\(desc) \(underlyingErr.psi_toDescriptiveString())"
        }
        return desc
    }
}
