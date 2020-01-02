kubectl  label nodes node2 nuwa.io/room=A nuwa.io/cabinet=W-01 nuwa.io/host=node2 --overwrite
kubectl  label nodes node3 nuwa.io/room=A nuwa.io/cabinet=S-02 nuwa.io/host=node3 --overwrite
kubectl  label nodes node4 nuwa.io/room=B nuwa.io/cabinet=W-01 nuwa.io/host=node4 --overwrite
kubectl  label nodes node5 nuwa.io/room=B nuwa.io/cabinet=S-02 nuwa.io/host=node5 --overwrite

kubectl get node --show-labels