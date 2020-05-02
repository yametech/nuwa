kubectl label nodes node1 nuwa.kubernetes.io/zone=A nuwa.kubernetes.io/rack=W-01 nuwa.kubernetes.io/host=node1 --overwrite
kubectl label nodes node2 nuwa.kubernetes.io/zone=A nuwa.kubernetes.io/rack=S-02 nuwa.kubernetes.io/host=node2 --overwrite
kubectl label nodes node3 nuwa.kubernetes.io/zone=B nuwa.kubernetes.io/rack=W-01 nuwa.kubernetes.io/host=node3 --overwrite
kubectl label nodes node4 nuwa.kubernetes.io/zone=B nuwa.kubernetes.io/rack=S-02 nuwa.kubernetes.io/host=node4 --overwrite

kubectl get node --show-labels
