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

#import "PsiphonProviderNetwork.h"
#import "DefaultRouteMonitor.h"
#import "IPv6Synthesizer.h"
#import "NetworkID.h"
#import "Reachability.h"
#import "Reachability+ReachabilityProtocol.h"
#import "ReachabilityProtocol.h"

@implementation PsiphonProviderNetwork {
    id<ReachabilityProtocol> reachability;
    void (^logger) (NSString *_Nonnull);
}

- (void)initialize {
    if (@available(iOS 12.0, *)) {
        self->reachability = [[DefaultRouteMonitor alloc] init];
    } else {
        self->reachability = [Reachability reachabilityForInternetConnection];
    }
}

- (id)init {
    self = [super init];
    if (self) {
        [self initialize];
    }
    return self;
}

- (instancetype)initWithLogger:(void (^__nonnull)(NSString *_Nonnull))logger {
    self = [super init];
    if (self) {
        [self initialize];
        self->logger = logger;
    }
    return self;
}

- (void)log:(NSString*)notice {
    if (self->logger != nil) {
        self->logger(notice);
    }
}

- (long)hasNetworkConnectivity {
    return [self->reachability reachabilityStatus] != NetworkReachabilityNotReachable;
}


- (NSString *)iPv6Synthesize:(NSString *)IPv4Addr {
    return [IPv6Synthesizer IPv4ToIPv6:IPv4Addr];
}

- (NSString *)getNetworkID {

    NetworkReachability currentNetworkStatus = reachability.reachabilityStatus;

    NSError *err;
    NSString *activeInterface =
        [NetworkInterface getActiveInterfaceWithReachability:self->reachability
                                     andCurrentNetworkStatus:currentNetworkStatus
                                                       error:&err];
    if (err != nil) {
        [self log:[NSString stringWithFormat:@"error getting active interface %@", err.localizedDescription]];
        return kNetworkIDUnknown;
    }

    NSString *networkID = [NetworkID getNetworkID:currentNetworkStatus
                       defaultActiveInterfaceName:activeInterface
                                            error:&err];
    if (err != nil) {
        [self log:[NSString stringWithFormat:@"error getting network ID %@", err.localizedDescription]];
        return kNetworkIDUnknown;
    }

    return networkID;
}

@end
