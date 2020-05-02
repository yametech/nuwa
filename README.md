
# Nuwa 　　　　　　　　　　　　　　　　　　　　　　[中文](README_zh.md)

[![Build Status](https://github.com/yametech/nuwa/workflows/nuwa/badge.svg?event=push&branch=master)](https://github.com/yametech/nuwa/actions?workflow=nuwa)
[![Go Report Card](https://goreportcard.com/badge/github.com/yametech/nuwa)](https://goreportcard.com/badge/github.com/yametech/nuwa)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](http://github.com/yametech/nuwa/blob/master/LICENSE)

## Introduction

   Since the native Deployment and Statefulset provides basic publishing strategies that cannot meet our scenario, in some cases it does not support cross-computer room publishing, batch publishing by group, and Deployment does not support step control, etc., so our nuwa project as the data plane of our cloud platform, the project supports stateful resources (stone) and stateless resources (water), and has dynamic resource injection (injector).


## Features

* Multi-Zone deployment, can be specific to a machine in a rack in a zone.
* Delicate deployment strategy.
* Deploy by group, batch deployment.
* Blue-green deployment, canary deployment.
* Stateful and stateless deployment.
* Dynamic injection.


## Installation requirements

* Requires kubernetes cluster version greater than or equal to 1.15.x


## Label the machine

```shell script
kubectl  label nodes node1 nuwa.kubernetes.io/zone=A nuwa.kubernetes.io/rack=W-01 nuwa.kubernetes.io/host=node1 --overwrite
kubectl  label nodes node2 nuwa.kubernetes.io/zone=A nuwa.kubernetes.io/rack=S-02 nuwa.kubernetes.io/host=node2 --overwrite
kubectl  label nodes node3 nuwa.kubernetes.io/zone=B nuwa.kubernetes.io/rack=W-01 nuwa.kubernetes.io/host=node3 --overwrite
kubectl  label nodes node4 nuwa.kubernetes.io/zone=B nuwa.kubernetes.io/rack=S-02 nuwa.kubernetes.io/host=node4 --overwrite

```


 ## Install Nuwa CRD resource

 `
   kubectl apply -f https://github.com/yametech/nuwa/releases/download/v1.0.0/release.yaml
 `


## Water Resource

### Advanced implementation of Deployment based on kubernetes  native resource

1. Supports geographical location deployment, which can be specific to a machine in a rack in a zone.
2. Support delicate deployment strategy: Alpha, beta, release.


### Deployment Strategy

This deployment strategy occurs because the released Pod itself is wrong (such as a program startup error due to lack of configuration). If a large number of releases will cause machine jitter and affect the running of the running Pod, reduce the jitter as much as possible. Lowest, let the user confirm whether the first pod released is wrong, and confirm that the next pod will be released.

* Alpha: Find a node randomly and only deploy a Pod, waiting for the user to confirm whether there is an error.
* Beta:  Pods are deployed on each Node.
* Release: Full deployment.

### Water resource usage template

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
      host: node1
      replicas: 1
    - zone: A
      rack: S-02
      host: node2
      replicas: 1
    - zone: B
      rack: W-01
      host: node3
      replicas: 1
    - zone: B
      rack: S-02
      host: node4
      replicas: 1

```

## Stone resource
   
### Advanced implementation of Statefulset based on kubernetes native resource

1. Supports geo-location publishing, which can be specific to a machine in a rack in a zone.
2. Support deployment strategy by group: Alpha, Beta, Omega, Release release(
because of the need to mount the disk).

### Deployment Strategy

* Alpha: Pick one of the groups and publish 1 Pod
* Beta:  Pick one (multi-group / one-group) to publish 1 Pod
* Omega: Pick multiple groups and publish 1 Pod on each machine
* Release: Full release



### Stone resource usage template

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
        host: node1
      - zone: A
        rack: S-02
        host: node2
    - group: B
      replicas: 2
      zoneset:
      - zone: B
        rack: W-01
        host: node3
      - zone: B
        rack: S-02
        host: node4
```

## Injector resource

   The implementation of the Injector resource combined with the CRD + Sidecar, which is to allow users to dynamically inject log collection, configuration files, and  tracking agent injection. Supports injection before and after the business container, combined with Water resource and Stone resource. When Water is deleted, the corresponding Injector resource will also be GC.


### With the use of Water resources


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
      host: node1
      replicas: 2
    - zone: A
      rack: S-02
      host: node2
      replicas: 0
    - zone: B
      rack: W-01
      host: node3
      replicas: 0
    - zone: B
      rack: S-02
      host: node4
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


