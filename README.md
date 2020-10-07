[English](https://github.com/txn2/kubefwd/blob/master/README.md)|[中文](https://github.com/txn2/kubefwd/blob/master/README_CN.md)

Kubernetes port forwarding for local development, Contributions welcome!

![kubefwd - kubernetes bulk port forwarding](https://raw.githubusercontent.com/txn2/kubefwd/master/kubefwd-mast2.jpg)

[![Build Status](https://travis-ci.com/txn2/kubefwd.svg?branch=master)](https://travis-ci.com/txn2/kubefwd)
[![GitHub license](https://img.shields.io/github/license/txn2/kubefwd.svg)](https://github.com/txn2/kubefwd/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/txn2/kubefwd)](https://goreportcard.com/report/github.com/txn2/kubefwd)
[![GitHub release](https://img.shields.io/github/release/txn2/kubefwd.svg)](https://github.com/txn2/kubefwd/releases)

# kubefwd (Kube Forward)

Read [Kubernetes Port Forwarding for Local Development](https://mk.imti.co/kubernetes-port-forwarding/) for background and a detailed guide to **kubefwd**. Follow [Craig Johnston](https://twitter.com/cjimti) on Twitter for project updates.

**kubefwd** is a command line utility built to port forward multiple [services] within one or more [namespaces] on one or more Kubernetes clusters. **kubefwd** uses the same port exposed by the service and forwards it from a loopback IP address on your local workstation. **kubefwd** temporally adds domain entries to your `/etc/hosts` file with the service names it forwards.

When working on our local workstation, my team and I often build applications that access services through their service names and ports within a [Kubernetes] namespace. **kubefwd** allows us to develop locally with services available as they would be in the cluster.

![kubefwd - Kubernetes port forward](kubefwd_ani.gif)

<p align="center">
  <img width="654" height="684" src="https://mk.imti.co/images/content/kubefwd-net.png" alt="kubefwd - Kubernetes Port Forward Diagram">
</p>

## OS

Tested directly on **macOS** and **Linux** based docker containers.

## MacOs Install / Update

**kubefwd** assumes you have **kubectl** installed and configured with access to a Kubernetes cluster. **kubefwd** uses the **kubectl** current context. The **kubectl** configuration is not used. However, its configuration is needed to access a Kubernetes cluster.

Ensure you have a context by running:
```bash
kubectl config current-context
```

If you are running MacOS and use [homebrew] you can install **kubefwd** directly from the [txn2] tap:

```bash
brew install kubefwd
```

To upgrade:
```bash
brew upgrade kubefwd
```

## Windows Install / Update

```batch
scoop install kubefwd
```

To upgrade:
```batch
scoop update kubefwd
```

## Docker

Forward all services from the namespace **the-project** to a Docker container named **the-project**:

```bash
docker run -it --rm --privileged --name the-project \
    -v "$(echo $HOME)/.kube/":/root/.kube/ \
    txn2/kubefwd services -n the-project
```


Execute a curl call to an Elasticsearch service in your Kubernetes cluster:

```bash
docker exec the-project curl -s elasticsearch:9200
```

## Alternative Installs (tar.gz, RPM, deb, snap)
Check out the [releases](https://github.com/txn2/kubefwd/releases) section on Github for alternative binaries.

## Contribute
[Fork kubefwd](https://github.com/txn2/kubefwd) and build a custom version. We welcome any useful pull requests.

## Usage

Forward all services for the namespace `the-project`. Kubefwd finds the first Pod associated with each Kubernetes service found in the Namespace and port forwards it based on the Service spec to a local IP  address and port. A domain name is added to your /etc/hosts file pointing to the local IP.

### Update
Forwarding of headlesss Service is currently supported, Kubefwd forward all Pods for headless service; At the same time, the namespace-level service monitoring is supported. When a new service is created or the old service is deleted under the namespace, kubefwd can automatically start/end forwarding; Supports Pod-level forwarding monitoring. When the forwarded Pod is deleted (such as updating the deployment, etc.), the forwarding of the service to which the pod belongs is automatically restarted;

```bash
sudo kubefwd svc -n the-project
```

Forward all svc for the namespace `the-project` where labeled `system: wx`:

```bash
sudo kubefwd svc -l system=wx -n the-project
```

## Help

```bash
$ kubefwd svc --help

INFO[20:48:38]  _          _           __             _
INFO[20:48:38] | | ___   _| |__   ___ / _|_      ____| |
INFO[20:48:38] | |/ / | | | '_ \ / _ \ |_\ \ /\ / / _  |
INFO[20:48:38] |   <| |_| | |_) |  __/  _|\ V  V / (_| |
INFO[20:48:38] |_|\_\\__,_|_.__/ \___|_|   \_/\_/ \__,_|
INFO[20:48:38]
INFO[20:48:38] Version 1.11.0
INFO[20:48:38] https://github.com/txn2/kubefwd
INFO[20:48:38]
Forward multiple Kubernetes services from one or more namespaces. Filter services with selector.

Usage:
  kubefwd services [flags]

Aliases:
  services, svcs, svc

Examples:
  kubefwd svc -n the-project
  kubefwd svc -n the-project -l app=wx,component=api
  kubefwd svc -n default -n the-project
  kubefwd svc -n default -d internal.example.com
  kubefwd svc -n the-project -x prod-cluster


Flags:
  -x, --context strings     specify a context to override the current context
  -d, --domain string       Append a pseudo domain name to generated host names.
      --exitonfailure       Exit(1) on failure. Useful for forcing a container restart.
  -h, --help                help for services
  -c, --kubeconfig string   absolute path to a kubectl config file
  -n, --namespace strings   Specify a namespace. Specify multiple namespaces by duplicating this argument.
  -l, --selector string     Selector (label query) to filter on; supports '=', '==', and '!=' (e.g. -l key1=value1,key2=value2).
  -v, --verbose             Verbose output.
```

## Development

### Build and Run
 Local

```bash
go run ./cmd/kubefwd/kubefwd.go
```

### Build Release

Build test release:
```bash
goreleaser --skip-publish --rm-dist --skip-validate
```

Build and release:
```bash
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --rm-dist
```

### License

Apache License 2.0

### Sponsor

Open source utility by [Craig Johnston](https://twitter.com/cjimti), [imti blog](http://imti.co/) and sponsored by [Deasil Works, Inc.]


## Similar projects, products, alternatives and complementary utilities

- [inlets](https://inlets.dev) - inlets provides an L7 HTTP tunnel for applications through the use of an exit node, it is used by the inlets operator
- [inlets-operator](https://github.com/inlets/inlets-operator) - L4 TCP tunnel, which can tunnel any TCP traffic and is on the roadmap for the inlets-operator
- [telepresence.io](https://www.telepresence.io/) - Telepresence substitutes a two-way network proxy for your normal pod running in the Kubernetes cluster.



[Kubernetes]:https://kubernetes.io/
[namespaces]:https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
[services]:https://kubernetes.io/docs/concepts/services-networking/service/
[homebrew]:https://brew.sh/
[txn2]:https://txn2.com/
[golang:1.11.5]:https://hub.docker.com/_/golang/
[Deasil Works, Inc.]:https://deasil.works/
