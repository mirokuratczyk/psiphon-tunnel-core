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

#import "AppDelegate.h"
#import "libpsiphontunnel.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#import "util.h"

@interface AppDelegate ()

@end

@implementation AppDelegate


- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions {

    NSString *bundlePath = [NSBundle.mainBundle resourcePath];
    NSString *configPath = [bundlePath stringByAppendingPathComponent:@"psiphon_config"];
    const char *config = [configPath cStringUsingEncoding:NSUTF8StringEncoding];

    char *psiphon_config = read_file(config);
    if (!psiphon_config) {
        NSLog(@"Could not find config file: %s\n", config);
        exit(1);
    }

    // From https://developer.apple.com/forums/thread/19002?answerId=60913022#60913022
    NSString *cachesPath = [NSSearchPathForDirectoriesInDomains(NSCachesDirectory, NSUserDomainMask, YES) lastObject];

    // set server list
    char *server_list = "";

    // set client platform
    char * const os = "OSName"; // "Android", "iOS", "Windows", etc.
    char * const os_version = "OSVersion"; // "4.0.4", "10.3", "10.0.10240", etc.
    char * const bundle_identifier = "com.example.exampleClientLibraryApp";
    char * client_platform = (char *)malloc(sizeof(char) * (strlen(os) + strlen(os_version) + strlen(bundle_identifier) + 4)); // 4 for 3 underscores and null terminating character

    int n = sprintf(client_platform, "%s_%s_%s", os, os_version, bundle_identifier);

    // set network ID
    char * const network_id = "TEST";

    // set timeout
    int32_t timeout = 60;

    struct Parameters params;
    params.sizeofStruct = sizeof(struct Parameters);
    params.dataRootDirectory = [cachesPath cStringUsingEncoding:NSUTF8StringEncoding];
    params.clientPlatform = client_platform;
    params.networkID = network_id;
    params.establishTunnelTimeoutSeconds = &timeout;

    NSURLRequest *req = [NSURLRequest requestWithURL:[NSURL URLWithString:@"https://freegeoip.app/json/"]];

    NSURLSessionConfiguration *sessionConfig = NSURLSessionConfiguration.ephemeralSessionConfiguration;
    [sessionConfig setRequestCachePolicy:NSURLRequestReloadIgnoringLocalCacheData];
    [sessionConfig setTimeoutIntervalForRequest:60 * 5];
    [sessionConfig setConnectionProxyDictionary:@{(__bridge NSString*)kCFStreamPropertySOCKSProxy: @1,
                                                  (__bridge NSString*)kCFStreamPropertySOCKSProxyHost: @"127.0.0.1",
                                                  (__bridge NSString*)kCFStreamPropertySOCKSProxyPort: @5555}];

    NSURLSession *session = [NSURLSession sessionWithConfiguration:sessionConfig];

    NSURLSessionDataTask *task = [session dataTaskWithRequest:req completionHandler:^(NSData * _Nullable data, NSURLResponse * _Nullable response, NSError * _Nullable error) {

        //TODO: handle errors
        //TODO: check response encoding
        
        NSLog(@"%@", [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding]);

        // The underlying memory of `result` is managed by PsiphonTunnel and is freed in Stop
        PsiphonTunnelStop();
    }];

    // start will return once Psiphon connects or does not connect for timeout seconds
    char *result = PsiphonTunnelStart(psiphon_config, server_list, &params);

    // print results
    printf("Result: %s\n", result);

    [task resume];

    free(client_platform);
    client_platform = NULL;
    free(psiphon_config);
    psiphon_config = NULL;

    // Override point for customization after application launch.
    return YES;
}


- (void)applicationWillResignActive:(UIApplication *)application {
    // Sent when the application is about to move from active to inactive state. This can occur for certain types of temporary interruptions (such as an incoming phone call or SMS message) or when the user quits the application and it begins the transition to the background state.
    // Use this method to pause ongoing tasks, disable timers, and throttle down OpenGL ES frame rates. Games should use this method to pause the game.
}


- (void)applicationDidEnterBackground:(UIApplication *)application {
    // Use this method to release shared resources, save user data, invalidate timers, and store enough application state information to restore your application to its current state in case it is terminated later.
}


- (void)applicationWillEnterForeground:(UIApplication *)application {
    // Called as part of the transition from the background to the active state; here you can undo many of the changes made on entering the background.
}


- (void)applicationDidBecomeActive:(UIApplication *)application {
    // Restart any tasks that were paused (or not yet started) while the application was inactive. If the application was previously in the background, optionally refresh the user interface.
}


@end
