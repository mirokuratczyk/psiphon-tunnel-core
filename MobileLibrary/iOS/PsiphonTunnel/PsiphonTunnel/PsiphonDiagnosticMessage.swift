//
//  PsiphonDiagnosticMessage.swift
//  PsiphonTunnel
//
//  Created by user on 2020-08-06.
//  Copyright © 2020 Psiphon Inc. All rights reserved.
//

import Foundation

@objc final class PsiphonDiagnosticMessage : NSObject {
    @objc var message: String
    @objc var timestamp: String

    // TODO: warning
    @objc init(message: String) {
        self.message = message
        self.timestamp = PsiphonDateFormatter.rfc3339Formatter().string(from: Date())
        super.init()
    }

    @objc init(message: String, timestamp: String) {
        self.message = message
        self.timestamp = timestamp
        super.init()
    }

    @objc init(message: String, dateFormatter: DateFormatter) {
        self.message = message
        self.timestamp = dateFormatter.string(from: Date())
        super.init()
    }
}
