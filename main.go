package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"encoding/json"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	libDatabox "github.com/me-box/lib-go-databox"
)

var path string
var dockerCli *client.Client

const certsBasePath = "./certs"

func main() {

	DOCKER_API_VERSION := flag.String("API", "1.37", "Docker API version ")

	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	startCmdIP := startCmd.String("swarm-ip", "127.0.0.1", "The IP on the host to advertise the swarm.")
	startCmdRelease := startCmd.String("release", "0.5.0", "Databox version to start, can uses tagged versions or latest")
	startCmdRegistryHosts := startCmd.String("registryHost", "docker.io", "Override the default registry host, server where images are pulled form")
	startCmdRegistry := startCmd.String("registry", "databoxsystems", "Override the default registry path, where images are pulled form")
	startCmdPassword := startCmd.String("password", "", "Override the password if you dont want an auto generated one. Mainly for testing")
	startCmdHostPath := startCmd.String("host-path", "", "Pass the hosts path to the databox cmd. This is needed if you are starting databox from inside a container.")
	appStore := startCmd.String("appstore", "https://store.iotdatabox.com", "Override the default appstore where manifests are loaded form")
	cmImage := startCmd.String("cm", "databoxsystems/container-manager", "Override container-manager image")
	uiImage := startCmd.String("core-ui", "databoxsystems/core-ui", "Override core ui image")
	arbiterImage := startCmd.String("arbiter", "databoxsystems/arbiter", "Override arbiter image")
	coreNetworkImage := startCmd.String("core-network", "databoxsystems/core-network", "Override container-manager image")
	coreNetworkRelay := startCmd.String("core-network-relay", "databoxsystems/core-network-relay", "Override core-network-relay image")
	appServerImage := startCmd.String("app-server", "databoxsystems/app-server", "Override local app-server image")
	exportServerImage := startCmd.String("export-service", "databoxsystems/export-service", "Override export-service image")
	storeImage := startCmd.String("store", "databoxsystems/core-store", "Override core-store image")
	clearSLAdb := startCmd.Bool("flushSLAs", false, "Removes any saved apps or drivers from the SLA database so they will not restart")
	enableLogging := startCmd.Bool("v", false, "Enables verbose logging of the container-manager")
	arch := startCmd.String("arch", "", "Used to override the detected cpu architecture only useful for testing arm64v8 support using docker for mac.")
	sslHostName := startCmd.String("sslHostName", "", "Used to override the detected HostName for use in ssl cert.")
	ReGenerateDataboxCertificates := startCmd.Bool("regenerateCerts", false, "Fore databox to regenerate the databox root and certificate")
	stopCmd := flag.NewFlagSet("stop", flag.ExitOnError)
	logsCmd := flag.NewFlagSet("logs", flag.ExitOnError)

	sdkCmd := flag.NewFlagSet("sdk", flag.ExitOnError)
	startSDK := sdkCmd.Bool("start", false, "Use this to start the databox sdk")
	stopSDK := sdkCmd.Bool("stop", false, "Use this to stop the databox sdk")

	flag.Parse()

	os.Setenv("DOCKER_API_VERSION", *DOCKER_API_VERSION)
	dockerCli, _ = client.NewEnvClient()

	path = "" //this should not be set outside of a container use --host-path if needed

	if _, err := os.Stat("./certs"); err != nil {
		os.Mkdir("./certs", 0770)
	}
	if _, err := os.Stat("./slaStore"); err != nil {
		os.Mkdir("./slaStore", 0770)
	}

	if len(os.Args) == 1 {
		displayUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "start":
		startCmd.Parse(os.Args[2:])

		libDatabox.Info("Starting Databox " + *startCmdRelease)

		//get some info in the network configuration
		hostname, _ := os.Hostname()
		if *sslHostName != "" {
			hostname = *sslHostName
		}
		ipv4s := removeIPv6addresses(getLocalInterfaceIps())

		cpuArch := ""
		if *arch != "" {
			cpuArch = *arch
		} else if runtime.GOARCH == "amd64" {
			cpuArch = "amd64"
		} else if runtime.GOARCH == "arm64" {
			cpuArch = "arm64v8"
		} else {
			panic("Unsupported CPU architecture ")
		}

		if *startCmdHostPath != "" {
			path = *startCmdHostPath
		}

		opts := &libDatabox.ContainerManagerOptions{
			Version:               *startCmdRelease,
			SwarmAdvertiseAddress: *startCmdIP,
			ContainerManagerImage: *cmImage,
			CoreUIImage:           *uiImage,
			ArbiterImage:          *arbiterImage,
			CoreNetworkImage:      *coreNetworkImage,
			CoreNetworkRelayImage: *coreNetworkRelay,
			AppServerImage:        *appServerImage,
			ExportServiceImage:    *exportServerImage,
			DefaultStoreImage:     *storeImage,
			ClearSLAs:             *clearSLAdb,
			DefaultRegistryHost:   *startCmdRegistryHosts,
			DefaultRegistry:       *startCmdRegistry,
			DefaultAppStore:       *appStore,
			EnableDebugLogging:    *enableLogging,
			OverridePasword:       *startCmdPassword,
			HostPath:              path,
			ExternalIP:            getExternalIP(),
			InternalIPs:           ipv4s,
			Hostname:              hostname,
			Arch:                  cpuArch,
		}

		if *ReGenerateDataboxCertificates == true {
			libDatabox.Info("Forcing regoration of Databox certificates")
			os.RemoveAll(certsBasePath)
		}

		//This dir must exist! if its not here the cm wont start as its used as the service attempts to bind mount it!
		if _, err := os.Stat(certsBasePath); err != nil {
			os.Mkdir(certsBasePath, 0700)
		}

		Start(opts)
	case "stop":
		libDatabox.Info("Stoping Databox ...")
		stopCmd.Parse(os.Args[2:])
		Stop()
	case "logs":
		logsCmd.Parse(os.Args[2:])
		ShowLogs()
	case "sdk":
		sdkCmd.Parse(os.Args[2:])

		if *startSDK == true {
			libDatabox.Info("Starting Databox SDK")
			StartSDK()

		} else if *stopSDK == true {
			libDatabox.Info("Stoping Databox SDK")
			StopSDK()
		} else {
			sdkCmd.Usage()
		}

	default:
		displayUsage()
		os.Exit(2)
	}

}

func displayUsage() {
	fmt.Println(`
		databox [cmd]
		Usage:
			start - start databox
			stop - stop databox
			logs - view databox logs
			sdk  - manage the databox sdk

		Use databox [cmd] help to see more options
		`)
}

func Start(opt *libDatabox.ContainerManagerOptions) {

	_, err := dockerCli.SwarmInit(context.Background(), swarm.InitRequest{
		ListenAddr:    "127.0.0.1",
		AdvertiseAddr: opt.SwarmAdvertiseAddress,
	})
	libDatabox.ChkErrFatal(err)

	os.Remove("/tmp/databox_relay")
	err = syscall.Mkfifo("/tmp/databox_relay", 0666)
	libDatabox.ChkErrFatal(err)

	createContainerManager(opt)

}

func Stop() {

	_, err := dockerCli.SwarmInspect(context.Background())
	if err != nil {
		//Not in swarm mode databox is not running
		return
	}
	filters := filters.NewArgs()
	filters.Add("label", "databox.type")

	services, err := dockerCli.ServiceList(context.Background(), types.ServiceListOptions{Filters: filters})
	libDatabox.ChkErr(err)

	if len(services) > 0 {
		for _, service := range services {
			libDatabox.Info("Removing old databox service " + service.Spec.Name)
			err := dockerCli.ServiceRemove(context.Background(), service.ID)
			libDatabox.ChkErr(err)
		}
	}

	dockerCli.SwarmLeave(context.Background(), true)

	containers, err := dockerCli.ContainerList(context.Background(), types.ContainerListOptions{Filters: filters})
	libDatabox.ChkErr(err)

	if len(containers) > 0 {
		for _, container := range containers {
			libDatabox.Info("Removing old databox container " + container.Image)
			err := dockerCli.ContainerStop(context.Background(), container.ID, nil)
			libDatabox.ChkErr(err)
			err = dockerCli.ContainerRemove(context.Background(), containers[0].ID, types.ContainerRemoveOptions{Force: true})
			libDatabox.ChkErr(err)
		}
	}

}

func createContainerManager(options *libDatabox.ContainerManagerOptions) {

	portConfig := []swarm.PortConfig{
		swarm.PortConfig{
			TargetPort:    443,
			PublishedPort: 443,
			PublishMode:   "host",
		},
		swarm.PortConfig{
			TargetPort:    80,
			PublishedPort: 80,
			PublishMode:   "host",
		},
	}

	certsPath, _ := filepath.Abs("./certs")
	slaStorePath, _ := filepath.Abs("./sdk")

	//create options secret
	optionsJSON, err := json.Marshal(options)
	libDatabox.ChkErrFatal(err)
	secretCreateResponse, err := dockerCli.SecretCreate(context.Background(), swarm.SecretSpec{
		Annotations: swarm.Annotations{
			Name: "DATABOX_CM_OPTIONS",
		},
		Data: optionsJSON,
	})
	libDatabox.ChkErrFatal(err)

	cmOptionsSecret := swarm.SecretReference{
		SecretID:   secretCreateResponse.ID,
		SecretName: "DATABOX_CM_OPTIONS",
		File: &swarm.SecretReferenceFileTarget{
			Name: "DATABOX_CM_OPTIONS",
			UID:  "0",
			GID:  "0",
			Mode: 0444,
		},
	}

	service := swarm.ServiceSpec{
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image:    options.ContainerManagerImage + "-" + options.Arch + ":" + options.Version,
				Hostname: "container-manager",
				Labels:   map[string]string{"databox.type": "container-manager"},
				Env: []string{
					"DATABOX_ARBITER_ENDPOINT=tcp://arbiter:4444",
					"DATABOX_SDK=0",
				},
				Mounts: []mount.Mount{
					mount.Mount{
						Type:   mount.TypeBind,
						Source: "/var/run/docker.sock",
						Target: "/var/run/docker.sock",
					},
					mount.Mount{
						Type:   mount.TypeBind,
						Source: options.HostPath + certsPath,
						Target: "/certs",
					},
					mount.Mount{
						Type:   mount.TypeBind,
						Source: options.HostPath + slaStorePath,
						Target: "/sdk",
					},
				},
				Secrets: []*swarm.SecretReference{&cmOptionsSecret},
			},
			Placement: &swarm.Placement{
				Constraints: []string{"node.role == manager"},
			},
		},
		EndpointSpec: &swarm.EndpointSpec{
			Mode:  "dnsrr",
			Ports: portConfig,
		},
	}

	service.Name = "container-manager"

	serviceOptions := types.ServiceCreateOptions{}

	pullImage(service.TaskTemplate.ContainerSpec.Image, options)
	_, err = dockerCli.ServiceCreate(context.Background(), service, serviceOptions)
	libDatabox.ChkErr(err)

}

func pullImage(image string, options *libDatabox.ContainerManagerOptions) {

	needToPull := true

	//do we have the image on disk?
	images, _ := dockerCli.ImageList(context.Background(), types.ImageListOptions{})
	for _, i := range images {
		for _, tag := range i.RepoTags {
			if image == tag {
				//we have the image no need to pull it !!
				needToPull = false
				break
			}
		}
	}

	//is it from the default registry (databoxsystems or whatever we overroad with) and tagged with latest?
	if strings.Contains(image, options.DefaultRegistry) == true && strings.Contains(image, ":latest") == true {
		//its in the default registry and has the :latest tag lets pull it to make sure we are up-to-date
		needToPull = true
	}

	fmt.Println("Need to pull = ", needToPull)

	if needToPull == true {
		libDatabox.Info("Pulling Image " + image)
		reader, err := dockerCli.ImagePull(context.Background(), image, types.ImagePullOptions{})
		libDatabox.ChkErr(err)
		io.Copy(ioutil.Discard, reader)
		libDatabox.Info("Done pulling Image " + image)
		reader.Close()
	}
}

func removeContainer(name string) {
	filters := filters.NewArgs()
	filters.Add("name", name)
	containers, clerr := dockerCli.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: filters,
		All:     true,
	})
	libDatabox.ChkErr(clerr)

	if len(containers) > 0 {
		rerr := dockerCli.ContainerRemove(context.Background(), containers[0].ID, types.ContainerRemoveOptions{Force: true})
		libDatabox.ChkErr(rerr)
	}
}

func getLocalInterfaceIps() []string {
	IPs := []string{}
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				IPs = append(IPs, v.IP.String())
			case *net.IPAddr:
				IPs = append(IPs, v.IP.String())
			}
		}
	}
	return IPs
}

func removeIPv6addresses(addresses []string) []string {
	var filteredIPs []string
	for _, ip := range addresses {
		parsedIP := net.ParseIP(ip)
		if parsedIP.To4() != nil {
			filteredIPs = append(filteredIPs, ip)
		}
	}
	return filteredIPs
}

func getExternalIP() string {
	var netClient = &http.Client{
		Timeout: time.Second * 3,
	}
	response, err := netClient.Get("http://whatismyip.akamai.com/")
	libDatabox.ChkErrFatal(err)
	ip, err := ioutil.ReadAll(response.Body)
	libDatabox.ChkErrFatal(err)
	response.Body.Close()
	libDatabox.Debug("External IP found " + string(ip))
	return string(ip)
}
