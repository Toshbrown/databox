# Databox Container Manager
Databox OS container manager and dashboard server.

## Installation

The container manager must run in a docker container with access to /var/run/docker.sock. So first install docker for your platform. 

**Linux** Install docker https://docs.docker.com/engine/installation/linux/  

**MacOS** Install docker https://docs.docker.com/docker-for-mac/  

Known issues: 

In some cases, the time in docker containers on mac can get out of sync with the system clock. This causes the HTTPS certs generated by the CM from being valid. See https://github.com/docker/for-mac/issues/17. 
Fix: restart docker for mac.  

**Windows** Install docker https://docs.docker.com/docker-for-windows/ (The old Docker Toolbox is not supported)

##Running

Once docker is installed just run the flowing to get your databox up and running. 

		docker run \
			-v /var/run/docker.sock:/var/run/docker.sock \
			--name databox-cm \
			--label databox.type=container-manager \
			-p 8989:8989 \
			-t toshdatabox/databox-cm

Once Its started point a web browser at 127.0.0.1:8989 and have fun. This is databoxes normal mode of 
operation an will use an external app store and image repository for apps.  

##Devlopment

To develop for the databox platform. It may be necessary to run the platform in dev mode. 
This will enable a local app store and Image repository to be run in containers on your machine.  
In this mode it is passable to build and replace any part of the platform. 

First get the source code:

			git clone https://github.com/me-box/databox-container-manager.git  
			cd databox-container-manager  
			npm install --production  

Build the container:

			docker create \
				-v /var/run/docker.sock:/var/run/docker.sock \
				-v `pwd`:/cm \
				--name databox-cm \
				--label databox.type=container-manager \
				-e "DATABOX_DEV=1" \
				-p 8989:8989 \
				-it node:alpine sh

				N.B you should mount ./certs and ./slaStore as volumes if you want ssl certs and launched apps to save between restarts. 

Start the databox in dev:

			docker run -it databox-cm sh

			Then:

			npm --prefix /cm start 

In dev mode you will need to install an custom ssl cert into the docker cert store. 
Instructions are provided on first run. You will also need to seed your local registry with the required base docker images
for the databox platform. A scrip called update  updateLocalRegistry.sh

#### ENV VARS

    DATABOX_DEV=1 Enable dev mode Note: in dev mode some extra configuration is required. Follow the on screen instructions.   
	PORT=8081 Default port is 8989, but can be overridden using the PORT environment variable, i.e.:


