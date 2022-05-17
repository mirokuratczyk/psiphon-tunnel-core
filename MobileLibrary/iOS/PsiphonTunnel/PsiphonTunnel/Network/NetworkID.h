/*
 * Copyright (c) 2020, Psiphon Inc.
 * All rights reserved.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

#import <Foundation/Foundation.h>
#import "ReachabilityProtocol.h"

NS_ASSUME_NONNULL_BEGIN

extern NSString *kNetworkIDUnknown;

@interface NetworkID : NSObject

/// The network ID contains potential PII. In tunnel-core, the network ID
/// is used only locally in the client and not sent to the server.
///
/// See network ID requirements here:
/// https://godoc.org/github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon#NetworkIDGetter
/// @param networkReachability Network reachability status.
/// @param defaultActiveInterfaceName Interface associated with the default route on the device.
/// @param outError If non-nil, then an error occurred while trying determine the network ID.
+ (NSString*_Nullable)getNetworkID:(NetworkReachability)networkReachability
        defaultActiveInterfaceName:(NSString*_Nullable)defaultActiveInterfaceName
                             error:(NSError *_Nullable *_Nonnull)outError;

@end

NS_ASSUME_NONNULL_END
