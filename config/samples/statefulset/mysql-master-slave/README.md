# label you nodes
reference ../label_node/label.sh
# create a nuwa statfulset mysql master slave
kubectl apply -f .
kubectl get nuwasts,service,pod
# access you mysql master
kubectl exec mysql-0 /bin/bash
# entry with mysql client
mysql
mysql> create database test;
mysql> create table a(id int,name varchar(128));
mysql> insert into a value(1,"test_nuwa_statefulset");

# access you mysql slave1
kubectl exec mysql-1 /bin/bash
# entry with mysql client
mysql
mysql> select * from test.a;

# access you mysql slave2
kubectl exec mysql-2 /bin/bash
# entry with mysql client
mysql
mysql> select * from test.a;