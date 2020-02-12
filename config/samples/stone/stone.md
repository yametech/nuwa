nuwa stone 资源
--
* 基于 nuwa statefulset 高层实现,实现自动创建service关联关系的试,让在使用statefulset的复杂参数中定制一套标准;
* 由stone创建的statefulset不能单独管理,因为stone会把statefulset状态以stone定义的期望目标实现协调,但是statefulset的滚动更新的方式可以由用户自定义;
* 资源管理statefulset以组的方式,将以地理标识组来实现分组并网格化打散pod在各有个node上,实现高可用;
* 每组的实现都是一个单元化,故在单机房的情况下就使用一个组;

安装使用
--
1. 首先有一个kubernetes 集群,至少大于 1.15.x 版本 
2. 安装 nuwa crd

模板参考
--
[![stone example](https://raw.githubusercontent.com/yametech/nuwa/master/config/samples/stone/nuwa_v1_stone.yaml)](https://raw.githubusercontent.com/yametech/nuwa/master/config/samples/stone/nuwa_v1_stone.yaml)

更新策略
--
在现实中,发布中的失败或者密集起应用会让机器的资源抖动,所以设计以下策略的方式

* Alpha 多组发布中,找最小组号的一个节点发布1个pod测试发布
* Beta  在每个组中都发布1个pod
* Omega 在每个组中依照node的数据覆盖每个pod
* Release 按照预期发布的方式发布

镜像容器发布
--
* 更新镜像的策略由statefulset的参数影响,如果组需要做尝试单个更新镜像滚动升级的操作,需要熟知 maxUnavailable,partition 参数;
* maxUnavailable 默认为1 顾名思义,就是最大不可用数, partition 默认为0,statefulset 的更新方式是由大到小的下标号更新,如果为0就是从大到小全部更新; 
````
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      podUpdatePolicy: InPlaceIfPossible
      maxUnavailable: 2
      partition: 2
````

其他部署方式
--
可与 injector 结合可以实现容器内上下文容器监控更新