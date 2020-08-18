//
//  PsiphonNoticeProxy.h
//  PsiphonTunnel
//
//  Created by user on 2020-08-05.
//  Copyright © 2020 Psiphon Inc. All rights reserved.
//

#import <Foundation/Foundation.h>
#import "PsiphonTunnel.h"
#import <PsiphonTunnel/PsiphonTunnel-Swift.h>
#import "Psi-meta.h"

NS_ASSUME_NONNULL_BEGIN

@interface PsiphonNoticeProxy : NSObject <GoPsiPsiphonProviderNoticeHandler>

/* TODO: name parameters */
- (id)initWithLogger:(void (^__nonnull)(NSString *_Nonnull))logger;

@end

NS_ASSUME_NONNULL_END
