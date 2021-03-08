/*
 * Copyright (c) 2015, Psiphon Inc.
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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon"
	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common"
	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/buildinfo"
	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/errors"
	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/tun"

	"github.com/eycorsican/go-tun2socks/common/dns/blocker"
	"github.com/eycorsican/go-tun2socks/common/log"
	_ "github.com/eycorsican/go-tun2socks/common/log/simple" // Register a simple logger.
	"github.com/eycorsican/go-tun2socks/core"
	"github.com/eycorsican/go-tun2socks/proxy/socks"
	"github.com/eycorsican/go-tun2socks/routes"
	tun2 "github.com/eycorsican/go-tun2socks/tun"
)

func init() {
	args.addFlag(fProxyServer)
	args.addFlag(fUdpTimeout)

	registerHandlerCreater("socks", func() {
		// Verify proxy server address.
		proxyAddr, err := net.ResolveTCPAddr("tcp", *args.ProxyServer)
		if err != nil {
			log.Fatalf("invalid proxy server address: %v", err)
		}
		proxyHost := proxyAddr.IP.String()
		proxyPort := uint16(proxyAddr.Port)

		core.RegisterTCPConnHandler(socks.NewTCPHandler(proxyHost, proxyPort))
		core.RegisterUDPConnHandler(socks.NewUDPHandler(proxyHost, proxyPort, *args.UdpTimeout))
	})
}

func main() {
	main2()

	// Define command-line parameters

	var configFilename string
	flag.StringVar(&configFilename, "config", "", "configuration input file")

	var dataRootDirectory string
	flag.StringVar(&dataRootDirectory, "dataRootDirectory", "", "directory where persistent files will be stored")

	var embeddedServerEntryListFilename string
	flag.StringVar(&embeddedServerEntryListFilename, "serverList", "", "embedded server entry list input file")

	var formatNotices bool
	flag.BoolVar(&formatNotices, "formatNotices", false, "emit notices in human-readable format")

	var interfaceName string
	flag.StringVar(&interfaceName, "listenInterface", "", "bind local proxies to specified interface")

	var versionDetails bool
	flag.BoolVar(&versionDetails, "version", false, "print build information and exit")
	flag.BoolVar(&versionDetails, "v", false, "print build information and exit")

	var feedbackUpload bool
	flag.BoolVar(&feedbackUpload, "feedbackUpload", false,
		"Run in feedback upload mode to send a feedback package to Psiphon Inc.\n"+
			"The feedback package will be read as a UTF-8 encoded string from stdin.\n"+
			"Informational notices will be written to stdout. If the upload succeeds,\n"+
			"the process will exit with status code 0; otherwise, the process will\n"+
			"exit with status code 1. A feedback compatible config must be specified\n"+
			"with the \"-config\" flag. Config must be provided by Psiphon Inc.")

	var feedbackUploadPath string
	flag.StringVar(&feedbackUploadPath, "feedbackUploadPath", "",
		"The path at which to upload the feedback package when the \"-feedbackUpload\"\n"+
			"flag is provided. Must be provided by Psiphon Inc.")

	var tunDevice, tunBindInterface, tunPrimaryDNS, tunSecondaryDNS string
	if tun.IsSupported() {

		// When tunDevice is specified, a packet tunnel is run and packets are relayed between
		// the specified tun device and the server.
		//
		// The tun device is expected to exist and should be configured with an IP address and
		// routing.
		//
		// The tunBindInterface/tunPrimaryDNS/tunSecondaryDNS parameters are used to bypass any
		// tun device routing when connecting to Psiphon servers.
		//
		// For transparent tunneled DNS, set the host or DNS clients to use the address specfied
		// in tun.GetTransparentDNSResolverIPv4Address().
		//
		// Packet tunnel mode is supported only on certains platforms.

		flag.StringVar(&tunDevice, "tunDevice", "", "run packet tunnel for specified tun device")
		flag.StringVar(&tunBindInterface, "tunBindInterface", tun.DEFAULT_PUBLIC_INTERFACE_NAME, "bypass tun device via specified interface")
		flag.StringVar(&tunPrimaryDNS, "tunPrimaryDNS", "8.8.8.8", "primary DNS resolver for bypass")
		flag.StringVar(&tunSecondaryDNS, "tunSecondaryDNS", "8.8.4.4", "secondary DNS resolver for bypass")
	}

	var noticeFilename string
	flag.StringVar(&noticeFilename, "notices", "", "notices output file (defaults to stderr)")

	var useNoticeFiles bool
	useNoticeFilesUsage := fmt.Sprintf("output homepage notices and rotating notices to <dataRootDirectory>/%s and <dataRootDirectory>/%s respectively", psiphon.HomepageFilename, psiphon.NoticesFilename)
	flag.BoolVar(&useNoticeFiles, "useNoticeFiles", false, useNoticeFilesUsage)

	var rotatingFileSize int
	flag.IntVar(&rotatingFileSize, "rotatingFileSize", 1<<20, "rotating notices file size")

	var rotatingSyncFrequency int
	flag.IntVar(&rotatingSyncFrequency, "rotatingSyncFrequency", 100, "rotating notices file sync frequency")

	flag.Parse()

	if versionDetails {
		b := buildinfo.GetBuildInfo()

		var printableDependencies bytes.Buffer
		var dependencyMap map[string]string
		longestRepoUrl := 0
		json.Unmarshal(b.Dependencies, &dependencyMap)

		sortedRepoUrls := make([]string, 0, len(dependencyMap))
		for repoUrl := range dependencyMap {
			repoUrlLength := len(repoUrl)
			if repoUrlLength > longestRepoUrl {
				longestRepoUrl = repoUrlLength
			}

			sortedRepoUrls = append(sortedRepoUrls, repoUrl)
		}
		sort.Strings(sortedRepoUrls)

		for repoUrl := range sortedRepoUrls {
			printableDependencies.WriteString(fmt.Sprintf("    %s  ", sortedRepoUrls[repoUrl]))
			for i := 0; i < (longestRepoUrl - len(sortedRepoUrls[repoUrl])); i++ {
				printableDependencies.WriteString(" ")
			}
			printableDependencies.WriteString(fmt.Sprintf("%s\n", dependencyMap[sortedRepoUrls[repoUrl]]))
		}

		fmt.Printf("Psiphon Console Client\n  Build Date: %s\n  Built With: %s\n  Repository: %s\n  Revision: %s\n  Dependencies:\n%s\n", b.BuildDate, b.GoVersion, b.BuildRepo, b.BuildRev, printableDependencies.String())
		os.Exit(0)
	}

	// Initialize notice output

	var noticeWriter io.Writer
	noticeWriter = os.Stderr

	if noticeFilename != "" {
		noticeFile, err := os.OpenFile(noticeFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Printf("error opening notice file: %s\n", err)
			os.Exit(1)
		}
		defer noticeFile.Close()
		noticeWriter = noticeFile
	}

	if formatNotices {
		noticeWriter = psiphon.NewNoticeConsoleRewriter(noticeWriter)
	}
	psiphon.SetNoticeWriter(noticeWriter)

	// Handle required config file parameter

	// EmitDiagnosticNotices is set by LoadConfig; force to true
	// and emit diagnostics when LoadConfig-related errors occur.

	if configFilename == "" {
		psiphon.SetEmitDiagnosticNotices(true, false)
		psiphon.NoticeError("configuration file is required")
		os.Exit(1)
	}
	configFileContents, err := ioutil.ReadFile(configFilename)
	if err != nil {
		psiphon.SetEmitDiagnosticNotices(true, false)
		psiphon.NoticeError("error loading configuration file: %s", err)
		os.Exit(1)
	}
	config, err := psiphon.LoadConfig(configFileContents)
	if err != nil {
		psiphon.SetEmitDiagnosticNotices(true, false)
		psiphon.NoticeError("error processing configuration file: %s", err)
		os.Exit(1)
	}

	// Set data root directory
	if dataRootDirectory != "" {
		config.DataRootDirectory = dataRootDirectory
	}

	if interfaceName != "" {
		config.ListenInterface = interfaceName
	}

	// Configure notice files

	if useNoticeFiles {
		config.UseNoticeFiles = &psiphon.UseNoticeFiles{
			RotatingFileSize:      rotatingFileSize,
			RotatingSyncFrequency: rotatingSyncFrequency,
		}
	}

	// Configure packet tunnel, including updating the config.

	if tun.IsSupported() && tunDevice != "" {
		tunDeviceFile, err := configurePacketTunnel(
			config, tunDevice, tunBindInterface, tunPrimaryDNS, tunSecondaryDNS)
		if err != nil {
			psiphon.SetEmitDiagnosticNotices(true, false)
			psiphon.NoticeError("error configuring packet tunnel: %s", err)
			os.Exit(1)
		}
		defer tunDeviceFile.Close()
	}

	// All config fields should be set before calling Commit.

	err = config.Commit(true)
	if err != nil {
		psiphon.SetEmitDiagnosticNotices(true, false)
		psiphon.NoticeError("error loading configuration file: %s", err)
		os.Exit(1)
	}

	// BuildInfo is a diagnostic notice, so emit only after config.Commit
	// sets EmitDiagnosticNotices.

	psiphon.NoticeBuildInfo()

	var worker Worker

	if feedbackUpload {
		// Feedback upload mode
		worker = &FeedbackWorker{
			feedbackUploadPath: feedbackUploadPath,
		}
	} else {
		// Tunnel mode
		worker = &TunnelWorker{
			embeddedServerEntryListFilename: embeddedServerEntryListFilename,
		}
	}

	err = worker.Init(config)
	if err != nil {
		psiphon.NoticeError("error in init: %s", err)
		os.Exit(1)
	}

	workCtx, stopWork := context.WithCancel(context.Background())
	defer stopWork()

	workWaitGroup := new(sync.WaitGroup)
	workWaitGroup.Add(1)
	go func() {
		defer workWaitGroup.Done()

		err := worker.Run(workCtx)
		if err != nil {
			psiphon.NoticeError("%s", err)
			stopWork()
			os.Exit(1)
		}

		// Signal the <-controllerCtx.Done() case below. If the <-systemStopSignal
		// case already called stopController, this is a noop.
		stopWork()
	}()

	systemStopSignal := make(chan os.Signal, 1)
	signal.Notify(systemStopSignal, os.Interrupt, syscall.SIGTERM)

	// writeProfilesSignal is nil and non-functional on Windows
	writeProfilesSignal := makeSIGUSR2Channel()

	// Wait for an OS signal or a Run stop signal, then stop Psiphon and exit

	for exit := false; !exit; {
		select {
		case <-writeProfilesSignal:
			psiphon.NoticeInfo("write profiles")
			profileSampleDurationSeconds := 5
			common.WriteRuntimeProfiles(
				psiphon.NoticeCommonLogger(),
				config.DataRootDirectory,
				"",
				profileSampleDurationSeconds,
				profileSampleDurationSeconds)
		case <-systemStopSignal:
			psiphon.NoticeInfo("shutdown by system")
			stopWork()
			workWaitGroup.Wait()
			exit = true
		case <-workCtx.Done():
			psiphon.NoticeInfo("shutdown by controller")
			exit = true
		}
	}
}

func configurePacketTunnel(
	config *psiphon.Config,
	tunDevice, tunBindInterface, tunPrimaryDNS, tunSecondaryDNS string) (*os.File, error) {

	file, _, err := tun.OpenTunDevice(tunDevice)
	if err != nil {
		return nil, errors.Trace(err)
	}

	provider := &tunProvider{
		bindInterface: tunBindInterface,
		primaryDNS:    tunPrimaryDNS,
		secondaryDNS:  tunSecondaryDNS,
	}

	config.PacketTunnelTunFileDescriptor = int(file.Fd())
	config.DeviceBinder = provider
	config.DnsServerGetter = provider

	return file, nil
}

type tunProvider struct {
	bindInterface string
	primaryDNS    string
	secondaryDNS  string
}

// BindToDevice implements the psiphon.DeviceBinder interface.
func (p *tunProvider) BindToDevice(fileDescriptor int) (string, error) {
	return p.bindInterface, tun.BindToDevice(fileDescriptor, p.bindInterface)
}

// GetPrimaryDnsServer implements the psiphon.DnsServerGetter interface.
func (p *tunProvider) GetPrimaryDnsServer() string {
	return p.primaryDNS
}

// GetSecondaryDnsServer implements the psiphon.DnsServerGetter interface.
func (p *tunProvider) GetSecondaryDnsServer() string {
	return p.secondaryDNS
}

// Worker creates a protocol around the different run modes provided by the
// compiled executable.
type Worker interface {
	// Init is called once for the worker to perform any initialization.
	Init(config *psiphon.Config) error
	// Run is called once, after Init(..), for the worker to perform its
	// work. The provided context should control the lifetime of the work
	// being performed.
	Run(ctx context.Context) error
}

// TunnelWorker is the Worker protocol implementation used for tunnel mode.
type TunnelWorker struct {
	embeddedServerEntryListFilename string
	embeddedServerListWaitGroup     *sync.WaitGroup
	controller                      *psiphon.Controller
}

// Init implements the Worker interface.
func (w *TunnelWorker) Init(config *psiphon.Config) error {

	// Initialize data store

	err := psiphon.OpenDataStore(config)
	if err != nil {
		psiphon.NoticeError("error initializing datastore: %s", err)
		os.Exit(1)
	}

	// If specified, the embedded server list is loaded and stored. When there
	// are no server candidates at all, we wait for this import to complete
	// before starting the Psiphon controller. Otherwise, we import while
	// concurrently starting the controller to minimize delay before attempting
	// to connect to existing candidate servers.
	//
	// If the import fails, an error notice is emitted, but the controller is
	// still started: either existing candidate servers may suffice, or the
	// remote server list fetch may obtain candidate servers.
	//
	// TODO: abort import if controller run ctx is cancelled. Currently, this
	// import will block shutdown.
	if w.embeddedServerEntryListFilename != "" {
		w.embeddedServerListWaitGroup = new(sync.WaitGroup)
		w.embeddedServerListWaitGroup.Add(1)
		go func() {
			defer w.embeddedServerListWaitGroup.Done()

			err = psiphon.ImportEmbeddedServerEntries(
				config,
				w.embeddedServerEntryListFilename,
				"")

			if err != nil {
				psiphon.NoticeError("error importing embedded server entry list: %s", err)
				return
			}
		}()

		if !psiphon.HasServerEntries() {
			psiphon.NoticeInfo("awaiting embedded server entry list import")
			w.embeddedServerListWaitGroup.Wait()
		}
	}

	controller, err := psiphon.NewController(config)
	if err != nil {
		psiphon.NoticeError("error creating controller: %s", err)
		return errors.Trace(err)
	}
	w.controller = controller

	return nil
}

// Run implements the Worker interface.
func (w *TunnelWorker) Run(ctx context.Context) error {
	defer psiphon.CloseDataStore()
	if w.embeddedServerListWaitGroup != nil {
		defer w.embeddedServerListWaitGroup.Wait()
	}

	w.controller.Run(ctx)
	return nil
}

// FeedbackWorker is the Worker protocol implementation used for feedback
// upload mode.
type FeedbackWorker struct {
	config             *psiphon.Config
	feedbackUploadPath string
}

// Init implements the Worker interface.
func (f *FeedbackWorker) Init(config *psiphon.Config) error {

	// The datastore is not opened here, with psiphon.OpenDatastore,
	// because it is opened/closed transiently in the psiphon.SendFeedback
	// operation. We do not want to contest database access incase another
	// process needs to use the database. E.g. a process running in tunnel
	// mode, which will fail if it cannot aquire a lock on the database
	// within a short period of time.

	f.config = config

	return nil
}

// Run implements the Worker interface.
func (f *FeedbackWorker) Run(ctx context.Context) error {

	// TODO: cancel blocking read when worker context cancelled?
	diagnostics, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return errors.TraceMsg(err, "FeedbackUpload: read stdin failed")
	}

	if len(diagnostics) == 0 {
		return errors.TraceNew("FeedbackUpload: error zero bytes of diagnostics read from stdin")
	}

	err = psiphon.SendFeedback(ctx, f.config, string(diagnostics), f.feedbackUploadPath)
	if err != nil {
		return errors.TraceMsg(err, "FeedbackUpload: upload failed")
	}

	psiphon.NoticeInfo("FeedbackUpload: upload succeeded")

	return nil
}

var version = "undefined"

var handlerCreater = make(map[string]func(), 0)

func registerHandlerCreater(name string, creater func()) {
	handlerCreater[name] = creater
}

var postFlagsInitFn = make([]func(), 0)

func addPostFlagsInitFn(fn func()) {
	postFlagsInitFn = append(postFlagsInitFn, fn)
}

type CmdArgs struct {
	Version         *bool
	TunName         *string
	TunAddr         *string
	TunGw           *string
	TunMask         *string
	TunDns          *string
	TunMTU          *int
	BlockOutsideDns *bool
	ProxyType       *string
	ProxyServer     *string
	UdpTimeout      *time.Duration
	LogLevel        *string
	DnsFallback     *bool
	Routes          *string
	Exclude         *string
}

type cmdFlag uint

const (
	fProxyServer cmdFlag = iota
	fUdpTimeout
)

var flagCreaters = map[cmdFlag]func(){
	fProxyServer: func() {
		if args.ProxyServer == nil {
			args.ProxyServer = flag.String("proxyServer", "1.2.3.4:1087", "Proxy server address")
		}
	},
	fUdpTimeout: func() {
		if args.UdpTimeout == nil {
			args.UdpTimeout = flag.Duration("udpTimeout", 1*time.Minute, "UDP session timeout")
		}
	},
}

func (a *CmdArgs) addFlag(f cmdFlag) {
	if fn, found := flagCreaters[f]; found && fn != nil {
		fn()
	} else {
		log.Fatalf("unsupported flag")
	}
}

var args = new(CmdArgs)

const (
	maxMTU = 65535
)

func main2() {
	// linux and darwin pick up the tun index automatically
	// windows requires the exact tun name
	defaultTunName := ""
	switch runtime.GOOS {
	case "darwin":
		defaultTunName = "utun"
	case "windows":
		defaultTunName = "socks2tun"
	}
	args.TunName = flag.String("tunName", defaultTunName, "TUN interface name")

	args.Version = flag.Bool("version", false, "Print version")
	args.TunAddr = flag.String("tunAddr", "10.255.0.2", "TUN interface address")
	args.TunGw = flag.String("tunGw", "10.255.0.1", "TUN interface gateway")
	args.TunMask = flag.String("tunMask", "255.255.255.255", "TUN interface netmask, it should be a prefixlen (a number) for IPv6 address")
	args.TunDns = flag.String("tunDns", "8.8.8.8,8.8.4.4", "DNS resolvers for TUN interface (only need on Windows)")
	args.TunMTU = flag.Int("tunMTU", 1300, "TUN interface MTU")
	args.BlockOutsideDns = flag.Bool("blockOutsideDns", false, "Prevent DNS leaks by blocking plaintext DNS queries going out through non-TUN interface (may require admin privileges) (Windows only) ")
	args.ProxyType = flag.String("proxyType", "socks", "Proxy handler type")
	args.LogLevel = flag.String("loglevel", "info", "Logging level. (debug, info, warn, error, none)")
	args.Routes = flag.String("routes", "", "Subnets to forward via TUN interface")
	args.Exclude = flag.String("exclude", "", "Subnets or hostnames to exclude from forwarding via TUN interface")

	flag.Parse()

	if *args.Version {
		fmt.Println(version)
		os.Exit(0)
	}

	if *args.TunMTU > maxMTU {
		fmt.Printf("MTU exceeds %d\n", maxMTU)
		os.Exit(1)
	}

	// Initialization ops after parsing flags.
	for _, fn := range postFlagsInitFn {
		if fn != nil {
			fn()
		}
	}

	// Set log level.
	switch strings.ToLower(*args.LogLevel) {
	case "debug":
		log.SetLevel(log.DEBUG)
	case "info":
		log.SetLevel(log.INFO)
	case "warn":
		log.SetLevel(log.WARN)
	case "error":
		log.SetLevel(log.ERROR)
	case "none":
		log.SetLevel(log.NONE)
	default:
		panic("unsupport logging level")
	}

	err := run()
	if err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	tunGw, tunRoutes, err := routes.Get(*args.Routes, *args.Exclude, *args.TunAddr, *args.TunGw, *args.TunMask)
	if err != nil {
		return fmt.Errorf("cannot parse config values: %v", err)
	}

	// Open the tun device.
	dnsServers := strings.Split(*args.TunDns, ",")
	tunDev, err := tun2.OpenTunDevice(*args.TunName, *args.TunAddr, *args.TunGw, *args.TunMask, *args.TunMTU, dnsServers)
	if err != nil {
		return fmt.Errorf("failed to open tun device: %v", err)
	}

	// close the tun device
	defer tunDev.Close()

	// unset routes on exit, when provided
	defer routes.Unset(*args.TunName, tunGw, tunRoutes)

	// set routes, when provided
	routes.Set(*args.TunName, tunGw, tunRoutes)

	if runtime.GOOS == "windows" && *args.BlockOutsideDns {
		if err := blocker.BlockOutsideDns(*args.TunName); err != nil {
			return fmt.Errorf("failed to block outside DNS: %v", err)
		}
	}

	// Setup TCP/IP stack.
	lwipWriter := core.NewLWIPStack().(io.Writer)

	// Register TCP and UDP handlers to handle accepted connections.
	if creater, found := handlerCreater[*args.ProxyType]; found {
		creater()
	} else {
		return fmt.Errorf("unsupported proxy type: %s", *args.ProxyType)
	}

	if args.DnsFallback != nil && *args.DnsFallback {
		// Override the UDP handler with a DNS-over-TCP (fallback) UDP handler.
		if creater, found := handlerCreater["dnsfallback"]; found {
			creater()
		} else {
			return fmt.Errorf("DNS fallback connection handler not found, build with `dnsfallback` tag")
		}
	}

	// Register an output callback to write packets output from lwip stack to tun
	// device, output function should be set before input any packets.
	core.RegisterOutputFn(func(data []byte) (int, error) {
		return tunDev.Write(data)
	})

	// Copy packets from tun device to lwip stack, it's the main loop.
	errChan := make(chan error, 1)
	go func() {
		_, err := io.CopyBuffer(lwipWriter, tunDev, make([]byte, maxMTU))
		if err != nil {
			errChan <- fmt.Errorf("copying data failed: %v", err)
		}
	}()

	log.Infof("Running tun2socks")

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGHUP)

	select {
	case err := <-errChan:
		return err
	case <-osSignals:
		return nil
	}
}
