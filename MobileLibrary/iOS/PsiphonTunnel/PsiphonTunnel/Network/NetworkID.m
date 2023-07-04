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

#import "NetworkID.h"
#import "NetworkInterface.h"
#import <CoreTelephony/CTTelephonyNetworkInfo.h>
#import <CoreTelephony/CTCarrier.h>
#import <SystemConfiguration/CaptiveNetwork.h>
#import "../json-framework/SBJson4.h"
#import <resolv.h>
#import <netdb.h>

NSString *networkIDExperimentStandardDefaultsKey = @"network-id-experiment";

@implementation NetworkID

// See comment in header.
+ (NSString *)getNetworkIDStats {

    NSDictionary<NSString*, NSDictionary<NSString*, NSNumber*>*> *networkIDStats =
        [[NSUserDefaults standardUserDefaults] objectForKey:networkIDExperimentStandardDefaultsKey];

    if (networkIDStats == nil) {
        return @"<no entries found>";
    }

    return [[[SBJson4Writer alloc] init] stringWithObject:networkIDStats];
}

+ (void)addInitialDNSCacheToNetworkIDStats:(NSString*)dnsCache forNetworkID:(NSString*)networkID {

    NSMutableDictionary<NSString*, NSDictionary<NSString*, NSNumber*>*> *networkIDStats;
    NSDictionary<NSString*, NSDictionary<NSString*, NSNumber*>*> *prevNetworkIDStats =
        [[NSUserDefaults standardUserDefaults] objectForKey:networkIDExperimentStandardDefaultsKey];

    if (prevNetworkIDStats == nil) {
        networkIDStats = [[NSMutableDictionary alloc] init];
    } else {
        networkIDStats = [[NSMutableDictionary alloc] initWithDictionary:prevNetworkIDStats];
    }

    NSMutableDictionary<NSString*, NSNumber*> *entry =
        [[NSMutableDictionary alloc] initWithDictionary:[networkIDStats objectForKey:networkID]];

    NSString *key = [NSString stringWithFormat:@"InitialDNSCache-%@", dnsCache];

    NSNumber *count = [entry objectForKey:key];
    if (count == nil) {
        count = [NSNumber numberWithInt:1];
    } else {
        count = [NSNumber numberWithInteger:[count intValue] + 1];
    }
    entry[key] = count;

    [networkIDStats setObject:entry forKey:networkID];

    [[NSUserDefaults standardUserDefaults]
     setObject:networkIDStats forKey:networkIDExperimentStandardDefaultsKey];
}

+ (void)addGatewayIPsToNetworkIDStats:(NSString*)gatewayIP forNetworkID:(NSString*)networkID {

    NSMutableDictionary<NSString*, NSDictionary<NSString*, NSNumber*>*> *networkIDStats;
    NSDictionary<NSString*, NSDictionary<NSString*, NSNumber*>*> *prevNetworkIDStats =
        [[NSUserDefaults standardUserDefaults] objectForKey:networkIDExperimentStandardDefaultsKey];

    if (prevNetworkIDStats == nil) {
        networkIDStats = [[NSMutableDictionary alloc] init];
    } else {
        networkIDStats = [[NSMutableDictionary alloc] initWithDictionary:prevNetworkIDStats];
    }

    NSMutableDictionary<NSString*, NSNumber*> *entry =
        [[NSMutableDictionary alloc] initWithDictionary:[networkIDStats objectForKey:networkID]];

    NSString *key = [NSString stringWithFormat:@"Gateway-%@", gatewayIP];

    NSNumber *count = [entry objectForKey:key];
    if (count == nil) {
        count = [NSNumber numberWithInt:1];
    } else {
        count = [NSNumber numberWithInteger:[count intValue] + 1];
    }
    entry[key] = count;

    [networkIDStats setObject:entry forKey:networkID];

    [[NSUserDefaults standardUserDefaults]
     setObject:networkIDStats forKey:networkIDExperimentStandardDefaultsKey];
}

+ (NSString *)getSystemDNSServers {

    NSMutableArray<NSString *> *serverList = [NSMutableArray new];

    res_state _state;
    _state = malloc(sizeof(struct __res_state));

    if (res_ninit(_state) < 0) {
        free(_state);
        return @"res_ninit.failed";
    }

    union res_sockaddr_union servers[NI_MAXSERV];  // Default max 32

    int numServersFound = res_getservers(_state, servers, NI_MAXSERV);

    char hostBuf[NI_MAXHOST];
    for (int i = 0; i < numServersFound; i++) {
        union res_sockaddr_union s = servers[i];
        if (s.sin.sin_len > 0) {
            int ret_code = getnameinfo((struct sockaddr *)&s.sin,
              (socklen_t)s.sin.sin_len,
              (char *)&hostBuf,
              sizeof(hostBuf),
              nil,
              0,
              NI_NUMERICHOST); // Flag "numeric form of hostname"

            if (EXIT_SUCCESS == ret_code) {
                [serverList addObject:[NSString stringWithUTF8String:hostBuf]];
            } else {
                return [NSString stringWithFormat:@"getSystemDNSServers: getnameinfo failed: %d", ret_code];
            }
        }
    }

    // Clear memory used by res_ninit
    res_ndestroy(_state);
    free(_state);

    return [serverList componentsJoinedByString:@"_"];
}

// See comment in header.
+ (NSString *)getNetworkIDWithReachability:(id<ReachabilityProtocol>)reachability
                   andCurrentNetworkStatus:(NetworkReachability)currentNetworkStatus
                         tunnelWholeDevice:(BOOL)tunnelWholeDevice
                                   warning:(NSError *_Nullable *_Nonnull)outWarn {

    *outWarn = nil;

    NSMutableDictionary<NSString*, NSDictionary<NSString*, NSNumber*>*> *networkIDStats;
    NSDictionary<NSString*, NSDictionary<NSString*, NSNumber*>*> *prevNetworkIDStats =
        [[NSUserDefaults standardUserDefaults] objectForKey:networkIDExperimentStandardDefaultsKey];

    if (prevNetworkIDStats == nil) {
        networkIDStats = [[NSMutableDictionary alloc] init];
    } else {
        networkIDStats = [[NSMutableDictionary alloc] initWithDictionary:prevNetworkIDStats];
    }

    // NetworkID is "VPN" if the library is used in non-VPN mode,
    // and an active VPN is found on the system.
    // This method is not exact and relies on CFNetworkCopySystemProxySettings,
    // specifically it may not return tun interfaces for some VPNs on macOS.
    if (!tunnelWholeDevice) {
        NSDictionary *_Nullable proxies = (__bridge NSDictionary *) CFNetworkCopySystemProxySettings();
        for (NSString *interface in [proxies[@"__SCOPED__"] allKeys]) {
            if ([interface containsString:@"tun"] || [interface containsString:@"tap"] || [interface containsString:@"ppp"] || [interface containsString:@"ipsec"]) {
                return @"VPN";
            }
        }
    }

    NSMutableString *networkID = [NSMutableString stringWithString:@"UNKNOWN"];
    if (currentNetworkStatus == NetworkReachabilityReachableViaWiFi) {
        [networkID setString:@"WIFI"];
        NSArray *networkInterfaceNames = (__bridge_transfer id)CNCopySupportedInterfaces();
        for (NSString *networkInterfaceName in networkInterfaceNames) {
            NSDictionary *networkInterfaceInfo = (__bridge_transfer id)CNCopyCurrentNetworkInfo((__bridge CFStringRef)networkInterfaceName);
            if (networkInterfaceInfo[(__bridge NSString*)kCNNetworkInfoKeyBSSID]) {
                [networkID appendFormat:@"-%@", networkInterfaceInfo[(__bridge NSString*)kCNNetworkInfoKeyBSSID]];
            }
        }
    } else if (currentNetworkStatus == NetworkReachabilityReachableViaCellular) {
        [networkID setString:@"MOBILE"];
        CTTelephonyNetworkInfo *telephonyNetworkinfo = [[CTTelephonyNetworkInfo alloc] init];
        CTCarrier *cellularProvider = [telephonyNetworkinfo subscriberCellularProvider];
        if (cellularProvider != nil) {
            NSString *mcc = [cellularProvider mobileCountryCode];
            NSString *mnc = [cellularProvider mobileNetworkCode];
            [networkID appendFormat:@"-%@-%@", mcc, mnc];
        }
    } else if (currentNetworkStatus == NetworkReachabilityReachableViaWired) {
        [networkID setString:@"WIRED"];

        NSError *err;
        NSString *activeInterface =
        [NetworkInterface getActiveInterfaceWithReachability:reachability
                                     andCurrentNetworkStatus:currentNetworkStatus
                                                       error:&err];
        if (err != nil) {
            NSString *localizedDescription = [NSString stringWithFormat:@"error getting active interface %@", err.localizedDescription];
            *outWarn = [[NSError alloc] initWithDomain:@"iOSLibrary"
                                                  code:1
                                              userInfo:@{NSLocalizedDescriptionKey:localizedDescription}];
            return networkID;
        }

        if (activeInterface != nil) {
            NSError *err;
            NSString *interfaceAddress = [NetworkInterface getInterfaceAddress:activeInterface
                                                                         error:&err];
            if (err != nil) {
                NSString *localizedDescription =
                [NSString stringWithFormat:@"getNetworkID: error getting interface address %@", err.localizedDescription];
                *outWarn = [[NSError alloc] initWithDomain:@"iOSLibrary" code:1 userInfo:@{NSLocalizedDescriptionKey: localizedDescription}];
                return networkID;
            } else if (interfaceAddress != nil) {
                [networkID appendFormat:@"-%@", interfaceAddress];
            }
        }
    } else if (currentNetworkStatus == NetworkReachabilityReachableViaLoopback) {
        [networkID setString:@"LOOPBACK"];
    }

    NSError *err;
    NSString *activeInterface =
    [NetworkInterface getActiveInterfaceWithReachability:reachability
                                 andCurrentNetworkStatus:currentNetworkStatus
                                                   error:&err];
    if (err != nil) {
        NSString *localizedDescription = [NSString stringWithFormat:@"error getting active interface %@", err.localizedDescription];
        *outWarn = [[NSError alloc] initWithDomain:@"iOSLibrary"
                                              code:1
                                          userInfo:@{NSLocalizedDescriptionKey:localizedDescription}];
        return networkID;
    }

    if (activeInterface == nil) {
        *outWarn = [[NSError alloc] initWithDomain:@"iOSLibrary" code:1 userInfo:@{NSLocalizedDescriptionKey: @"active interface nil"}];
        return networkID;
    }

    NSString *interfaceAddress = [NetworkInterface getInterfaceAddress:activeInterface
                                                                 error:&err];
    if (err != nil) {
        NSString *localizedDescription =
        [NSString stringWithFormat:@"getNetworkID: error getting interface address %@", err.localizedDescription];
        *outWarn = [[NSError alloc] initWithDomain:@"iOSLibrary" code:1 userInfo:@{NSLocalizedDescriptionKey: localizedDescription}];
        return networkID;
    }

    // Active interface

    NSString *ipType = @"ActiveInterface";
    NSString *key = [NSString stringWithFormat:@"%@-%@", ipType, interfaceAddress];

    NSMutableDictionary<NSString*, NSNumber*> *entry =
        [[NSMutableDictionary alloc] initWithDictionary:[networkIDStats objectForKey:networkID]];

    NSNumber *count = [entry objectForKey:key];
    if (count == nil) {
        count = [NSNumber numberWithInt:1];
    } else {
        count = [NSNumber numberWithInteger:[count intValue] + 1];
    }
    entry[key] = count;

    // System DNS servers

    ipType = @"SystemDNSServers";
    NSString *systemDNSServers = [NetworkID getSystemDNSServers];

    key = [NSString stringWithFormat:@"%@-%@", ipType, systemDNSServers];
    count = [entry objectForKey:key];
    if (count == nil) {
        count = [NSNumber numberWithInt:1];
    } else {
        count = [NSNumber numberWithInteger:[count intValue] + 1];
    }
    entry[key] = count;

    [networkIDStats setObject:entry forKey:networkID];

    [[NSUserDefaults standardUserDefaults]
     setObject:networkIDStats forKey:networkIDExperimentStandardDefaultsKey];

    return networkID;
}

@end
