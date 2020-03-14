# 女娲(nuwa)
[![Build Status](https://github.com/yametech/nuwa/workflows/nuwa/badge.svg?event=push&branch=master)](https://github.com/yametech/nuwa/actions?workflow=nuwa)
[![Go Report Card](https://goreportcard.com/badge/github.com/yametech/nuwa)](https://goreportcard.com/badge/github.com/yametech/nuwa)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](http://github.com/yametech/nuwa/blob/master/LICENSE)


## 背景简介
   由于原生的Deployment和Statefulset提供了基本的发布策略不能满足我们的场景,比如不支持跨机房发布,按组批量发布,Deployment 不支持步长控制等,所以诞生了我们的女娲项目,女娲项目作为我们云平台的数据平面支持了有状态的资源(Stone)和无状态的资源(Water),并且具有动态资源注入(Injector)。


## 功能特点

* 支持多机房发布,可以具体到某个机房上的某个机柜上的某台机器
* 支持细腻的发布次略
* 支持按组发布,批量发布
* 支持蓝绿发布,金丝雀发布
* 支持有状态和无状态的发布
* 支持动态注入




## Water资源

### Water资源是高级的Deployment资源的实现

1. 支持地理位置标识发布,可以具体到某个机房的某个机架上的某台机器上的发布。
2. 支持细腻的发布策略:Alpha,Beta,Release发布。


### 发布策略

出现这种发布策略,是因为发布的Pod本身是错误的(比如缺少配置导致程序启动错误),如果大量发布，会导致机器的抖动，影响正在运行的Pod的运行,尽可能的把抖动降到最低,让用户确认发布的第一个Pod是否有误,确认会进行下一次发布。

* Alpha: 找到一个节点,仅且发布一个Pod,等待用户确认是否有误。
* Beta:  在每个Node上都发布Pod。
* Release: 全量发布

### Water资源使用模板

``` shell script

apiVersion: nuwa.nip.io/v1
kind: Water
metadata:
  name: water-sample
spec:
  strategy: Release
  template:
    metadata:
      name:  water-sample
      labels:
        app: water-sample
    spec:
      containers:
        - name: cn-0
          image: nginx:latest
          imagePullPolicy: IfNotPresent
  service:
    ports:
      - name: default-web-port
        protocol: TCP
        port: 80
        targetPort: 80
    type: NodePort
  coordinates:
    - zone: A
      rack: W-01
      host: node2
      replicas: 1
    - zone: A
      rack: S-02
      host: node3
      replicas: 1
    - zone: B
      rack: W-01
      host: node4
      replicas: 1
    - zone: B
      rack: S-02
      host: node5
      replicas: 1

```

## Stone资源
   
### 基于k8s原生资源Statefulset的高级实现

1. 支持地理位置标识发布,可以具体到某个机房的某个机架上的某台机器上的发布。
2. 支持按组发布策略:Alpha,Beta,Omega,Release发布。

### 发布策略

* Alpha: 挑选其中一组的一个发布1个Pod
* Beta:  挑选(多组/一组)的一个发布1个Pod
* Omega: 挑选多组的每个机器上发布1个Pod
* Release: 全量发布



### Stone资源使用模板

``` shell script

apiVersion: nuwa.nip.io/v1
kind: Stone
metadata:
  name: stone-example
spec:
  strategy: Release
  template:
    metadata:
      name: sample
      labels:
        app: stone-example
    spec:
      containers:
        - name: cn-0
          image: nginx:alpine
          imagePullPolicy: IfNotPresent
  service:
    ports:
      - name: default-web-port
        protocol: TCP
        port: 80
        targetPort: 80
    type: NodePort
  coordinates:
    - group: A
      replicas: 3
      zoneset:
      - zone: A
        rack: W-01
        host: node2
      - zone: A
        rack: S-02
        host: node3
    - group: B
      replicas: 2
      zoneset:
      - zone: B
        rack: W-01
        host: node4
      - zone: B
        rack: S-02
        host: node5
```

## Injector资源

   Injector资源结合CRD+Sidecar方式的实现是为了让用户动态的注入日志收集,配置文件,链路追踪的agent的注入。支持在业务容器前和后注入,和Water资源和Stone资源结合,当Water被delete,相应的Injector资源也会被GC。


### 配合Water资源和Stone资源的使用案例


 ```shell script
apiVersion: nuwa.nip.io/v1
kind: Water
metadata:
  name: water-sample
spec:
  strategy: Release
  template:
    metadata:
      name: sample
      labels:
        app: water-sample
    spec:
      containers:
        - name: cn-0
          image: nginx:latest
          imagePullPolicy: IfNotPresent
  service:
    ports:
      - name: default-web-port
        protocol: TCP
        port: 80
        targetPort: 80
    type: NodePort
  coordinates:
    - zone: A
      rack: W-01
      host: node2
      replicas: 2
    - zone: A
      rack: S-02
      host: node3
      replicas: 0
    - zone: B
      rack: W-01
      host: node4
      replicas: 0
    - zone: B
      rack: S-02
      host: node5
      replicas: 0
---


# Injector intercepting the above resources will be injected into a busybox container before the business container
apiVersion: nuwa.nip.io/v1
kind: Injector
metadata:
  name: water-sample
spec:
  namespace: ${CURRENT_NAMESPACE}
  name: water-sample
  resourceType: Water
  selector:
    matchLabels:
      app: water-sample
  preContainers:
    - name: count
      image: busybox
      args:
        - /bin/sh
        - -c
        - >
          i=0;
          while true;
          do
            echo "$i: $(date)" >> /var/log/1.log;
            echo "$(date) INFO $i" >> /var/log/2.log;
            i=$((i+1));
            sleep 1;
          done
      volumeMounts:
        - name: varlog
          mountPath: /var/log
  volumes:
    - name: varlog
      emptyDir: {}
```


## 安装需求

* 需要 kubernetes 集群大于或等于1.15.x的版本


## 给机器打标签

```shell script

kubectl  label nodes node2 nuwa.io/zone=A nuwa.io/rack=W-01 nuwa.io/host=node2 --overwrite
kubectl  label nodes node3 nuwa.io/zone=A nuwa.io/rack=S-02 nuwa.io/host=node3 --overwrite
kubectl  label nodes node4 nuwa.io/zone=B nuwa.io/rack=W-01 nuwa.io/host=node4 --overwrite
kubectl  label nodes node5 nuwa.io/zone=B nuwa.io/rack=S-02 nuwa.io/host=node5 --overwrite

```


 ## 安装女娲CRD

 `
   kubectl apply -f https://github.com/yametech/nuwa/releases/download/${VERSION}/nuwa.yaml
 `
