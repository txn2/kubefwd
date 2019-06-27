实现批量端口转发让本地能方便访问远程Kubernetes服务, 欢迎贡献！

![kubefwd - kubernetes批量端口转发](https://raw.githubusercontent.com/txn2/kubefwd/master/kubefwd-mast2.jpg)

[![GitHub license](https://img.shields.io/github/license/txn2/kubefwd.svg)](https://github.com/txn2/kubefwd/blob/master/LICENSE)
[![Maintainability](https://api.codeclimate.com/v1/badges/bc696045260db8e0ba89/maintainability)](https://codeclimate.com/github/txn2/kubefwd/maintainability)
[![Go Report Card](https://goreportcard.com/badge/github.com/txn2/kubefwd)](https://goreportcard.com/report/github.com/txn2/kubefwd)
[![GitHub release](https://img.shields.io/github/release/txn2/kubefwd.svg)](https://github.com/txn2/kubefwd/releases)

# kubefwd (Kube Forward)

阅读 [Kubernetes Port Forwarding for Local Development](https://mk.imti.co/kubernetes-port-forwarding/) 的背景资料和**kubefwd**的详细指南。

**kubefwd** 是一个用于端口转发Kubernetes中指定namespace下的全部或者部分pod的命令行工具。 **kubefwd** 使用本地的环回IP地址转发需要访问的service，并且使用与service相同的端口。 **kubefwd** 会临时将service的域条目添加到 `/etc/hosts` 文件中。

启动**kubefwd**后，在本地就能像在Kubernetes集群中一样使用service名字与端口访问对应的应用程序。

![kubefwd - kubernetes批量端口转发](kubefwd_ani.gif)

<p align="center">
  <img width="654" height="684" src="https://mk.imti.co/images/content/kubefwd-net.png" alt="kubefwd - Kubernetes Port Forward Diagram">
</p>

## OS

直接在**macOS**或者**Linux**的docker容器上测试。

## MacOs 安装 / 升级

**kubefwd** 默认你已经安装了 **kubectl** 工具并且也已经设置好了访问Kubernetes集群的配置文件。**kubefwd** 使用 **kubectl** 的上下文运行环境. **kubectl** 工具并不会用到，但是它的配置文件会被用来访问Kubernetes集群。

确保你有上下文运行环境配置：
```bash
kubectl config current-context
```

如果你使用MacOs，并且安装了 [homebrew] ，那么你可以直接使用下面的命令来安装**kubefwd**:

```bash
brew install txn2/tap/kubefwd
```

升级:
```bash
brew upgrade kubefwd
```

## Windows 安装 / 升级

```batch
scoop install kubefwd
```

升级:
```batch
scoop update kubefwd
```

## Docker

将namespace为**the-project**的所有服务转发到名为**the-project**的Docker容器：

```bash
docker run -it --rm --privileged --name the-project \
    -v "$(echo $HOME)/.kube/":/root/.kube/ \
    txn2/kubefwd services -n the-project
```


通过curl命令访问Kubernetes集群下的Elasticsearch service :

```bash
docker exec the-project curl -s elasticsearch:9200
```

## 其它安装方式 (tar.gz, RPM, deb, snap)
查看在Github上 [releases](https://github.com/txn2/kubefwd/releases) 部分的二进制包。

## 贡献
[Fork kubefwd](https://github.com/txn2/kubefwd) 并构建自定义版本。我们也非常欢迎大家贡献自己的智慧。

## 用法

转发namespace `the-project`下的所有服务。 Kubefwd找到Kubernetess集群中，该namespace下对应的Service端口匹配的第一个Pod，并将其转发到本地IP地址和端口。同时service的域名将被添加到本地的 hosts文件中。
```bash
sudo kubefwd svc -n the-project
```

转发namespace `the-project`下所有的带有label为`system: wx`的service：

```bash
sudo kubefwd svc -l system=wx -n the-project
```

## 帮助说明

```bash
$ kubefwd svc --help

2019/03/09 21:13:18  _          _           __             _
2019/03/09 21:13:18 | | ___   _| |__   ___ / _|_      ____| |
2019/03/09 21:13:18 | |/ / | | | '_ \ / _ \ |_\ \ /\ / / _  |
2019/03/09 21:13:18 |   <| |_| | |_) |  __/  _|\ V  V / (_| |
2019/03/09 21:13:18 |_|\_\\__,_|_.__/ \___|_|   \_/\_/ \__,_|
2019/03/09 21:13:18
2019/03/09 21:13:18 Version 1.7.3
2019/03/09 21:13:18 https://github.com/txn2/kubefwd
2019/03/09 21:13:18
Forward multiple Kubernetes services from one or more namespaces. Filter services with selector.

Usage:
  kubefwd services [flags]

Aliases:
  services, svcs, svc

Examples:
  kubefwd svc -n the-project
  kubefwd svc -n the-project -l env=dev,component=api
  kubefwd svc -n default -l "app in (ws, api)"
  kubefwd svc -n default -n the-project
  kubefwd svc -n default -d internal.example.com
  kubefwd svc -n the-project -x prod-cluster


Flags:
  -x, --context strings     specify a context to override the current context
  -d, --domain string       Append a pseudo domain name to generated host names.
      --exitonfailure       Exit(1) on failure. Useful for forcing a container restart.
  -h, --help                help for services
  -c, --kubeconfig string   absolute path to a kubectl config fil (default "/Users/cjimti/.kube/config")
  -n, --namespace strings   Specify a namespace. Specify multiple namespaces by duplicating this argument.
  -l, --selector string     Selector (label query) to filter on; supports '=', '==', and '!=' (e.g. -l key1=value1,key2=value2).
  -v, --verbose             Verbose output.
```

## 开发

### 构建并运行
 本地

```bash
go run ./cmd/kubefwd/kubefwd.go
```

### 使用Docker构建并运行

使用镜像 [golang:1.11.5] 运行容器:
```bash
docker run -it --rm --privileged \
    -v "$(pwd)":/kubefwd \
    -v "$(echo $HOME)/.kube/":/root/.kube/ \
    -w /kubefwd golang:1.11.5 bash
```

```bash
sudo go run -mod vendor ./cmd/kubefwd/kubefwd.go svc
```

### 构建版本

构建并测试：
```bash
goreleaser --skip-publish --rm-dist --skip-validate
```

构建并发布:
```bash
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --rm-dist
```

### 使用Snap测试

```bash
multipass launch -n testvm
cd ./dist
multipass copy-files *.snap testvm:
multipass shell testvm
sudo snap install --dangerous kubefwd_64-bit.snap
```


### 开源协议 

Apache License 2.0  

### 赞助

由 [Deasil Works, Inc.] &
[Craig Johnston](https://imti.co) 赞助的开源工具

[Kubernetes]:https://kubernetes.io/
[Kubernetes namespace]:https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
[homebrew]:https://brew.sh/
[txn2]:https://txn2.com/
[golang:1.11.5]:https://hub.docker.com/_/golang/
[Deasil Works, Inc.]:https://deasil.works/
