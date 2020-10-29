# Micro [![License](https://img.shields.io/badge/license-polyform:shield-blue)](https://polyformproject.org/licenses/shield/1.0.0/) [![Go Report Card](https://goreportcard.com/badge/micro/micro)](https://goreportcard.com/report/github.com/micro/micro)

Micro is a platform for cloud native development.

## Overview

Micro addresses the key requirements for building services in the cloud. It leverages the microservices
architecture pattern and provides a set of services which act as the building blocks of a platform. Micro deals
with the complexity of distributed systems and provides simpler programmable abstractions to build on. 

## Install

Install from source

```sh
go get github.com/micro/micro/v3
```

Using a docker image

```sh
docker pull micro/micro
```

Latest release binaries

```sh
# MacOS
curl -fsSL https://raw.githubusercontent.com/micro/micro/master/scripts/install.sh | /bin/bash

# Linux
wget -q  https://raw.githubusercontent.com/micro/micro/master/scripts/install.sh -O - | /bin/bash

# Windows
powershell -Command "iwr -useb https://raw.githubusercontent.com/micro/micro/master/scripts/install.ps1 | iex"
```

## Getting Started

Run the server locally

```
micro server
```

Login to the server

```
# user: admin pass: micro
micro login
```

Create a service

```sh
# generate a service (follow instructions in output)
micro new helloworld

# run the service
micro run helloworld

# list services
micro services

# call a service
micro helloworld --name=Alice

# curl via the api
curl -d '{"name": "Alice"}' http://localhost:8080/helloworld
```

## Usage

See the [docs](https://m3o.org) for detailed information on the architecture, installation and use of the platform.

## Features

Micro is built as a microservices architecture. The server is composed of the following services.

- **API** - HTTP Gateway which dynamically maps http/json requests to RPC using path based resolution
- **Auth** - Authentication and authorization out of the box using jwt tokens and rule based access control.
- **Broker** - Ephemeral pubsub messaging for asynchronous communication and distributing notifications
- **Config** - Dynamic configuration and secrets management for service level config without the need to restart
- **Events** - Event streaming with ordered messaging, replay from offsets and persistent storage
- **Network** - Inter-service networking, isolation and routing plane for all internal request traffic
- **Proxy** - gRPC identity aware proxy used for remote access and any external grpc request traffic
- **Runtime** - Service lifecyle and process management with support for source to running with auto builds
- **Registry** - Centralised service discovery and endpoint explorer with feature rich metadata
- **Store** - Key-Value storage with TTL expiry and persistent crud to keep microservices stateless

Micro additionaly now contains the incredibly popular [Go Micro](https://github.com/asim/go-micro) framework built in for service development.

- **Framework** - A Go framework which makes it drop dead simple to write your services without having to piece together lines 
and lines of boilerplate. Auto configured and initialised by default, just import and get started quickly.

We also include a command line interface which can speak to any remote environment securely. 

- **CLI** - A command line interface with a dynamic command mapping for all services running on the platform. Turns any 
service instantly into a CLI command along with flag parsing for inputs.

Finally Micro bakes in the concept of `Environments` and multi-tenancy through `Namespaces`. Run your server locally for 
development and in the cloud for staging and production, seamlessly switch between them using `micro env set [environment]` 
and `micro user set [namespace]`.

## License

See [LICENSE](LICENSE) which makes use of [Polyform Shield](https://polyformproject.org/licenses/shield/1.0.0/).

## Hosting

If you're interested in a hosted version of Micro see [m3o.com](https://m3o.com). Docs at [m3o.dev](https://m3o.dev).

## Commercial Use

If you want to sell or offer Micro as a Service please email [contact@m3o.com](mailto:contact@m3o.com)

## Community

Join us on [Discord](https://discord.gg/hbmJEct), [Slack](https://slack.micro.mu) or follow on [Twitter](https://twitter.com/microhq) for updates.
