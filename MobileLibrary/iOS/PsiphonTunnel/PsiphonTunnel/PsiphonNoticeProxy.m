//
//  PsiphonNoticeProxy.m
//  PsiphonNoticeProxy
//
//  Created by user on 2020-08-05.
//  Copyright © 2020 Psiphon Inc. All rights reserved.
//

#import "PsiphonNoticeProxy.h"

// TODO: comment on the indirection.

@implementation PsiphonNoticeProxy {
    void (^logger) (NSString *_Nonnull);
}

/* TODO: name parameters */
- (id)initWithLogger:(void (^__nonnull)(NSString *_Nonnull))logger {
    self = [super init];
    if (self != nil) {
        self->logger = logger;
    }
    return self;
}

- (void)notice:(NSString *)noticeJSON {
    if (self->logger != nil) {
        self->logger(noticeJSON);
    }
}

@end
