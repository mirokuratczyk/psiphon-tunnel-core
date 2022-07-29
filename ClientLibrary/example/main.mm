#include "libpsiphontunnel.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <ctype.h>
#include <time.h>
 
#include <mach/mach_port.h>
#include <mach/mach_interface.h>
#include <mach/mach_init.h>
 
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <IOKit/IOMessage.h>
 
io_connect_t  root_port; // a reference to the Root Power Domain IOService

bool running_psiphon = false;

dispatch_queue_t psiphonQueue = dispatch_queue_create("com.psiphon3.library.PsiphonQueue", DISPATCH_QUEUE_SERIAL);

void print_time(const char *tag) {
    time_t now;
    time(&now);
    char *ctime_no_newline = strtok(ctime(&now), "\n");
    printf("%s: %s\n", ctime_no_newline, tag);
}

char *read_file(char *filename) {
    char *buffer = NULL;
    size_t size = 0;

    FILE *fp = fopen(filename, "r");

    if (!fp) {
        return NULL;
    }

    fseek(fp, 0, SEEK_END);
    size = ftell(fp);

    rewind(fp);
    buffer = (char*)malloc((size + 1) * sizeof(*buffer));

    fread(buffer, size, 1, fp);
    buffer[size] = '\0';

    return buffer;
}

void run_psiphon() {
    // load config
    char * const default_config = "psiphon_config";

    char * config = NULL; // = argv[1];

    if (!config) {
        config = default_config;
        printf("Using default config file: %s\n", default_config);
    }

    char *psiphon_config = read_file(config);
    if (!psiphon_config) {
        printf("Could not find config file: %s\n", config);
        return; // return 1;
    }

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
    int32_t timeout = 5;

    struct Parameters params;
    params.sizeofStruct = sizeof(struct Parameters);
    params.dataRootDirectory = ".";
    params.clientPlatform = client_platform;
    params.networkID = network_id;
    params.establishTunnelTimeoutSeconds = &timeout;

    // connect 100 times
    for (int i = 0; i < 100; i++) {
        // start will return once Psiphon connects or does not connect for timeout seconds
        char *result = PsiphonTunnelStart(psiphon_config, server_list, &params);

        // print results
        print_time(result);

        // The underlying memory of `result` is managed by PsiphonTunnel and is freed in Stop
        PsiphonTunnelStop();
    }

    free(client_platform);
    client_platform = NULL;
    free(psiphon_config);
    psiphon_config = NULL;

    print_time("done running Psiphon")
}

void
MySleepCallBack( void * refCon, io_service_t service, natural_t messageType, void * messageArgument )
{
    printf( "messageType %08lx, arg %08lx\n",
        (long unsigned int)messageType,
        (long unsigned int)messageArgument );

    switch ( messageType )
    {
 
        case kIOMessageCanSystemSleep:
            /* Idle sleep is about to kick in. This message will not be sent for forced sleep.
                Applications have a chance to prevent sleep by calling IOCancelPowerChange.
                Most applications should not prevent idle sleep.
 
                Power Management waits up to 30 seconds for you to either allow or deny idle
                sleep. If you don't acknowledge this power change by calling either
                IOAllowPowerChange or IOCancelPowerChange, the system will wait 30
                seconds then go to sleep.
            */
            

            print_time("kIOMessageCanSystemSleep");
 
            //Uncomment to cancel idle sleep
            //IOCancelPowerChange( root_port, (long)messageArgument );
            // we will allow idle sleep
            IOAllowPowerChange( root_port, (long)messageArgument );
            break;
 
        case kIOMessageSystemWillSleep:
            /* The system WILL go to sleep. If you do not call IOAllowPowerChange or
                IOCancelPowerChange to acknowledge this message, sleep will be
                delayed by 30 seconds.
 
                NOTE: If you call IOCancelPowerChange to deny sleep it returns
                kIOReturnSuccess, however the system WILL still go to sleep.
            */
            print_time("kIOMessageSystemWillSleep");

            IOAllowPowerChange( root_port, (long)messageArgument );

            dispatch_async(psiphonQueue, ^{
                print_time("running block on psiphonQueue");

                // Delay start to ensure system is powered down
                int ret = sleep(5);
                if (ret != 0) {
                    printf("Sleep failed %d\n", ret);
                }

                if (running_psiphon == false) {
                    print_time("run_psiphon()");
                    running_psiphon = true;
                    run_psiphon();
                }
            });

            break;
 
        case kIOMessageSystemWillPowerOn:
            //System has started the wake up process...
            print_time("kIOMessageSystemWillPowerOn");
            break;
 
        case kIOMessageSystemHasPoweredOn:
            //System has finished waking up...
            print_time("kIOMessageSystemHasPoweredOn");
            break;
 
        default:
            break;
    }
}

int main(int argc, char *argv[]) {

    // notification port allocated by IORegisterForSystemPower
    IONotificationPortRef  notifyPortRef;
 
    // notifier object, used to deregister later
    io_object_t            notifierObject;
    // this parameter is passed to the callback
    void*                  refCon;
 
    // register to receive system sleep notifications
 
    root_port = IORegisterForSystemPower( refCon, &notifyPortRef, MySleepCallBack, &notifierObject );
    if ( root_port == 0 )
    {
        printf("IORegisterForSystemPower failed\n");
        return 1;
    }
 
    // add the notification port to the application runloop
    CFRunLoopAddSource( CFRunLoopGetCurrent(),
            IONotificationPortGetRunLoopSource(notifyPortRef), kCFRunLoopCommonModes );
 
    /* Start the run loop to receive sleep notifications. Don't call CFRunLoopRun if this code
        is running on the main thread of a Cocoa or Carbon application. Cocoa and Carbon
        manage the main thread's run loop for you as part of their event handling
        mechanisms.
    */
    CFRunLoopRun();
 
    //Not reached, CFRunLoopRun doesn't return in this case.
    return (0);
}

