# 女娲(nuwa)
[![Build Status](https://github.com/yametech/nuwa/workflows/nuwa/badge.svg?event=push&branch=master)](https://github.com/yametech/nuwa/actions?workflow=nuwa)
[![Go Report Card](https://goreportcard.com/badge/github.com/yametech/nuwa)](https://goreportcard.com/badge/github.com/yametech/nuwa)

## Required for installation
1. Have a kubernetes cluster required at least greater than 1.15.x version
2. install nuwa CRD

## Development
1. customize env-setting.sh
2. make all 

## Identify your machine label
```shell script
kubectl  label nodes node2 nuwa.io/zone=A nuwa.io/rack=W-01 nuwa.io/host=node2 --overwrite
kubectl  label nodes node3 nuwa.io/zone=A nuwa.io/rack=S-02 nuwa.io/host=node3 --overwrite
kubectl  label nodes node4 nuwa.io/zone=B nuwa.io/rack=W-01 nuwa.io/host=node4 --overwrite
kubectl  label nodes node5 nuwa.io/zone=B nuwa.io/rack=S-02 nuwa.io/host=node5 --overwrite

kubectl get node --show-labels
```

## Water 
Advanced deployment resource manager which implements geographical identification to manage which areas, racks, and machines an application should be placed in

### Update strategy
Resources fluctuate the resources of the machine at the same time in a large number of applications, so the following strategies

* Alpha find a node publish 1 pod test release
* Beta releases 1 pod in each node
* Release release as expected

Refer to the following template to define your parameter template
```shell script
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

## Stone 
* Based on the high-level implementation of nuwa statefulset, it can automatically create service associations and customize a set of standards in complex parameters using statefulset
* The statefulset created by stone cannot be managed separately, because stone will coordinate the statefulset state with the desired goals defined by stone, but the rolling update method of statefulset can be customized by the user
* Resource management statefulset will be grouped and grouped by geographical identification groups, and grids will be used to break up pods on each node to achieve high availability
* The implementation of each group is a unit, so in the case of a single computer room, a group is used

### Update strategy
Resources fluctuate the resources of the machine at the same time in a large number of applications, so the following strategies

* In Alpha multi-group release, find a node with the smallest group number to publish 1 pod test release
* Beta releases 1 pod in each group
* Omega covers each pod according to node data in each group
* Release release as expected


Refer to the following template to define your parameter template
```shell script
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
## Injector
 Injector is a new Sidecar resource similar to Kubernetes v1.18.x, but here, the injector only intercepts Water and Stone resources, and associates the above resources as the owner
 
 Example for Water
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

 ## Release
 `
   kubectl apply -f https://github.com/yametech/nuwa/releases/download/${VERSION}/nuwa.yaml
 `