# 功能要求

让用户可以声明一个Pod下所有Container的启动顺序：以容器Ready为标志，前一个容器Ready后才能start下一个容器。

# 当前实现

给每个Container的Env增加一个`KRUISE_CONTAINER_BARRIER`的环境变量

value from 是一个configMap，key则是容器的Launch Priority

Kruise通过创建一个空的ConfigMap，然后按顺序向ConfigMap中添加Key-Value，来实现容器启动顺序的控制

这是一种比较hack的方法，原理其实就是给所有的容器添加一个环境变量的依赖（最终的依赖对象是ConfigMap），如果对应的ConfigMap中Key还不存在，容器就会无法启动

换言之，只有ConfigMap中存在对应key的容器才能正常启动，通过这种方式来实现容器启动顺序的控制

因此，在Pod启动过程中，可能会出现 CreateContainerConfigError 的错误

# 参考

## K8S原生方案

- 1.18的非正式版本中有考虑增加对sidecar类容器的支持，但是最后没有落地

- 最新的1.28中，sidecar container相关特性的支持被正式合入：https://github.
  com/kubernetes/kubernetes/pull/116429/files；以ContainerRestartPolicy的形式支持了容器启动顺序，但是只能在initContainers中使用

## Tekton

- https://mp.weixin.qq.com/s/5UXhXpwPDBh2xuGKq9Nqig

这个可能参考的是比较早起的tekton实现，采用的是entry point增加一个父进程套壳，然后通过共享volume等待文件写入的方式来做，也比较hack

## Lifecycle

- https://cloudnative.to/blog/k8s-1.18-container-start-sequence-control/
- https://aber.sh/articles/Control-the-startup-sequence-of-containers-in-Pod/

其实对于控制普通容器的启动顺序，K8S本身是有提供对应机制的，就是容器的Lifecycle Hook；通过给容器添加postStart钩子，可以在对应容器启动后阻塞，直到钩子执行完毕后，再进行下一个容器的启动。

所以最简单的方式就是给所有容器都增加一个postStart钩子，钩子内做一个等待当前容器启动的逻辑，就可以实现容器启动顺序的控制了。

# How to？

基于Lifecycle钩子的方式关键点在于，要在容器postStart的钩子里，读到Container的Status（由k8s维护）

所以就变成了"如何将容器的Status传递给钩子"的问题，第一想法是看下k8s原生是否有这样的能力，于是找到下面的文档：

https://kubernetes.io/zh-cn/docs/tasks/inject-data-application/environment-variable-expose-pod-information/#use-container-fields-as-value-for-env-var

但在官方机制中，只有[downward API](https://kubernetes.io/zh-cn/docs/concepts/workloads/pods/downward-api/#available-fields)中的字段可以进行传递，而status不在其中，所以不能直接使用。不过这里可以看到Pod的Annotation是可以传递的，所以就有了理论上可行的第一种方案：

通过Controller将容器的Status同步到Pod的Annotation，然后通过downward API以环境变量(或者Volume)形式传递给容器，在postStart中监听对应环境变量值的变化，就可以实现等待前一个容器Ready

类比这个思路，也可以不使用downward api，比如由controller将容器的Status同步到一个ConfigMap中，然后通过env的方式挂载到容器中，也可以实现同样的效果

# 问题

1. 如何实现通用的postStart逻辑 => 用户的container里面的逻辑是无法定制的，怎么把通用的postStart逻辑注入进去？
2. 如果用户的容器本身也有postStart逻辑，怎么进行merge
3. 采用钩子机制必须要对用户的容器顺序重新进行排序

