# Publish by ns limited native resources

If your machine or workload node already has a label, there are 2 nodes on zone A and rack R1,R2

```
kubectl create ns dxp
kubectl annotate --overwrite namespace dxp nuwa.kubernetes.io/default_resource_limit='[{"zone":"A","rack":"W-01","host":"node1"},{"zone":"A","rack":"W-02","host":"node2"}]'
kubectl create deployment --image nginx my-nginx
kubectl get deployment my-nginx

# Check whether to join node affinity
kind: Deployment
apiVersion: apps/v1
metadata:
  name: my-nginx
  namespace: dxp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-nginx
  template:
    metadata:
      creationTimestamp: null
      labels:
        app: my-nginx
    spec:
      containers:
        - name: nginx
          image: nginx
          resources: {}
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
          imagePullPolicy: Always
      restartPolicy: Always
      terminationGracePeriodSeconds: 30
      dnsPolicy: ClusterFirst
      securityContext: {}
      
      # note: The following information is added by nuwa controller
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: nuwa.kubernetes.io/zone
                    operator: In
                    values:
                      - A
                  - key: nuwa.kubernetes.io/rack
                    operator: In
                    values:
                      - W-01
                      - W-02
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              preference:
                matchExpressions:
                  - key: nuwa.kubernetes.io/host
                    operator: In
                    values:
                      - node1
                      - node2
```