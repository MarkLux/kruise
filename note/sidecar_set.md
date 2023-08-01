# SidecarSet

## 目标

提供一套可以用于横向管理Sidecar容器的CRD，能够支撑整个集群中Sidecar容器的注入、回滚等操作

参考：https://openkruise.io/zh/docs/user-manuals/sidecarset

## API设计

```go
// SidecarSet is the Schema for the sidecarsets API
type SidecarSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SidecarSetSpec   `json:"spec,omitempty"`
	Status SidecarSetStatus `json:"status,omitempty"`
}
```

### Spec

SidecarSet的Spec定义非常接近于一个常规的Deployment定义

```go
// SidecarSetSpec defines the desired state of SidecarSet
type SidecarSetSpec struct {
	// 用于圈选可注入Pod的LabelSelector
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// 声明作用的namespace（如果不声明，则作用于所有namespace）
	Namespace string `json:"namespace,omitempty"`

	// 同上，但是可匹配多个namespace
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// 要注入的InitContainers；InitContainers注入只能在Pod创建时注入，不会对已经存在对Pod生效
	InitContainers []SidecarContainer `json:"initContainers,omitempty"`

	// 要注入的Containers
	Containers []SidecarContainer `json:"containers,omitempty"`

	// 声明Sidecar Containers使用的存储卷
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Sidecar容器更新策略
	UpdateStrategy SidecarSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// Sidecar容器注入策略
	InjectionStrategy SidecarSetInjectionStrategy `json:"injectionStrategy,omitempty"`

	// 拉取容器镜像的凭证
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// RevisionHistoryLimit indicates the maximum quantity of stored revisions about the SidecarSet.
	// default value is 10
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// 用于支持对Pod元信息进行注入修改的诉求
	PatchPodMetadata []SidecarSetPatchPodMetadata `json:"patchPodMetadata,omitempty"`
}
```

### Status

```go
// SidecarSetStatus defines the observed state of SidecarSet
type SidecarSetStatus struct {
	// observedGeneration is the most recent generation observed for this SidecarSet. It corresponds to the
	// SidecarSet's generation, which is updated on mutation by the API Server.
	// 用于Controller中区分迭代次数
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// match了多少个Pod
	MatchedPods int32 `json:"matchedPods"`

	// match的Pod中，有多少个Pod已经更新到最新版本的sidecarset
	UpdatedPods int32 `json:"updatedPods"`

	// 处于Ready状态的Pod数量
	ReadyPods int32 `json:"readyPods"`

	// 处于Ready状态且更新到最新版本的Pod数量
	UpdatedReadyPods int32 `json:"updatedReadyPods,omitempty"`

	// SidecarSet最新的Controller Revision hash
	LatestRevision string `json:"latestRevision,omitempty"`

	// CollisionCount is the count of hash collisions for the SidecarSet. The SidecarSet controller
	// uses this field as a collision avoidance mechanism when it needs to create the name for the
	// newest ControllerRevision.
	CollisionCount *int32 `json:"collisionCount,omitempty"`
}
```

### Pod Annotation

```go
type SidecarSetUpgradeSpec struct {
	UpdateTimestamp              metav1.Time `json:"updateTimestamp"`
	SidecarSetHash               string      `json:"hash"`
	SidecarSetName               string      `json:"sidecarSetName"`
	SidecarList                  []string    `json:"sidecarList"`                  // sidecarSet container list
	SidecarSetControllerRevision string      `json:"controllerRevision,omitempty"` // sidecarSet controllerRevision name
}
```

```yaml
# 注入的SidecarSet的Hash元信息列表（key: sidecarSetName,value: SidecarSetUpgradeSpec)
kruise.io/sidecarset-hash: '{"demo-sidecarset":{"updateTimestamp":"2023-07-01T06:38:17Z","hash":"6wxw44dw4555d6b5c5cf5vf6d844bw948ddw5bv7zw5955c4x47vbf6fbc2v484z","sidecarSetName":"demo-sidecarset","sidecarList":["nginx-sidecar"],"controllerRevision":"demo-sidecarset-575984d5d8"},"test-sidecarset":{"updateTimestamp":"2023-07-01T06:39:56Z","hash":"x729b548zfwcw4cvbxb6w4vwzxfc2wdfc5fxcw984589548c94d9v776922xb8z6","sidecarSetName":"test-sidecarset","sidecarList":["busybox-sidecar"],"controllerRevision":"test-sidecarset-58d845b69d"}}'
# 注入的SidecarSet的Hash元信息列表，但不包含Container Image（key: sidecarSetName, value: SidecarSetUpgradeSpec）
kruise.io/sidecarset-hash-without-image: '{"demo-sidecarset":{"updateTimestamp":"2023-07-01T06:35:51Z","hash":"8vdxd4xz4cw2v2fwf6xf429d772d8xd467wf95zffb6d599v5d8c5z47z6c7579v","sidecarSetName":"demo-sidecarset","sidecarList":["nginx-sidecar"]},"test-sidecarset":{"updateTimestamp":"2023-07-01T06:35:51Z","hash":"6465vvx7v49wddz68w54bv5zw7zx282db2v8x467785x2729zx6c687bf8cx4885","sidecarSetName":"test-sidecarset","sidecarList":["busybox-sidecar"]}}'
# 已经注入的sidecarset的name list
kruise.io/sidecarset-injected-list: demo-sidecarset,test-sidecarset
# sidecarset的inplace update状态，细化到容器, (key: sidecarSetName, value: {revision, updateTimestamp, [lastContainerStatuses]})
kruise.io/sidecarset-inplace-update-state: '{"demo-sidecarset":{"revision":"6wxw44dw4555d6b5c5cf5vf6d844bw948ddw5bv7zw5955c4x47vbf6fbc2v484z","updateTimestamp":"2023-07-01T06:38:17Z","lastContainerStatuses":{"nginx-sidecar":{"imageID":"docker-pullable://nginx@sha256:af296b188c7b7df99ba960ca614439c99cb7cf252ed7bbc23e90cfda59092305"}}}」'
# sidecarset热更新状态
kruise.io/sidecarset-working-hotupgrade-container: ''
```

## 关键能力

### Sidecar注入

SidecarSet的注入逻辑在Webhook中实现 (`webhook/pod/sidecarset.go`)，该Webhook属于MutatingAdmissionWebhook，会在Pod的创建和更新时触发

文档说明Sidecar的注入只会发生在Pod的创建阶段，但看代码Update也会处理【？？】，不过下面的注入流程分析我们还是只讨论Create的情况

注入的核心流程：

1. List得到所有的SidecarSet对象
2. 判断当前Pod是否有匹配的SidecarSet对象
3. 对于匹配上的SidecarSet对象，根据InjectPolicy声明的Revision，获取指定的Revision版本对象（通过Controller History），如果没有指定Revision版本的话，使用latest

    ```go
    matchedSidecarSets := make([]sidecarcontrol.SidecarControl, 0)
    for _, sidecarSet := range sidecarsetList.Items {
        if sidecarSet.Spec.InjectionStrategy.Paused {
            continue
        }
        if matched, err := sidecarcontrol.PodMatchedSidecarSet(h.Client, pod, &sidecarSet); err != nil {
            return false, err
        } else if !matched {
            continue
        }
        // 【3】获取指定的Revision版本对象，如果没有声明，则使用latest
        suitableSidecarSet, err := h.getSuitableRevisionSidecarSet(&sidecarSet, oldPod, pod, req.AdmissionRequest.Operation)
        if err != nil {
            return false, err
        }
        // 把最终匹配到的SidecarSet对象封装成SidecarControl对象
        control := sidecarcontrol.New(suitableSidecarSet)
        if !control.IsActiveSidecarSet() {
            continue
        }
        matchedSidecarSets = append(matchedSidecarSets, control)
    }
    ```

4. 处理 `PatchPodMetadata`，如果对应的SidecarSet中声明了 `PatchPodMetadata`，则通过Patch的方式修改Pod的元信息

    ```go
    for _, control := range matchedSidecarSets {
		sidecarSet := control.GetSidecarset()
		sk, err := sidecarcontrol.PatchPodMetadata(&pod.ObjectMeta, sidecarSet.Spec.PatchPodMetadata)
		if err != nil {
			klog.Errorf("sidecarSet(%s) update pod(%s/%s) metadata failed: %s", sidecarSet.Name, pod.Namespace, pod.Name, err.Error())
			return false, err
		} else if !sk {
			// skip = false
			skip = false
		}
	}
    ```
5. 构建要注入的容器、Annotation等信息

   ```go
   // 【params】
   // pod：pod对象
   // matchedSidecarSets：该pod匹配到的SidecarSet对象
   // 【returns】
   // sidecarContainers：要注入该pod的sidecar container
   // sidecarInitContainers：要注入该pod的 init container
   // sidecarSecrets：要注入该pod的pull image secret
   // volumesInSidecars：要注入的Volumes
   // injectedAnnotations：要注入该pod的annotation
   func buildSidecars(isUpdated bool, pod *corev1.Pod, oldPod *corev1.Pod, matchedSidecarSets []sidecarcontrol.SidecarControl) (
       sidecarContainers, sidecarInitContainers []*appsv1alpha1.SidecarContainer, sidecarSecrets []corev1.LocalObjectReference,
       volumesInSidecars []corev1.Volume, injectedAnnotations map[string]string, err error) {

       // 【1】为Pod生成Sidecarset所需的Annoataion
       injectedAnnotations = make(map[string]string)
       // Annotation1：kruise.io/sidecarset-hash
       // sidecarSet.name -> sidecarSet hash struct
       sidecarSetHash := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
       // Annotation2：SidecarSet Without Image Hash
       // sidecarSet.name -> sidecarSet hash(without image) struct
       sidecarSetHashWithoutImage := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
       // 从Pod的Annotation中解析已有的SidecarSet Hash（略）
       if oldHashStr := pod.Annotations[sidecarcontrol.SidecarSetHashAnnotation]; len(oldHashStr) > 0 {
           // ...
       }
       // 从Pod的Annotation中解析已有的SidecarSet Without Image Hash（略）
       if oldHashStr := pod.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation]; len(oldHashStr) > 0 {
           // ...
       }
       // 计算SidecarSet热更新相关的Annotation
       // hotUpgrade work info, sidecarSet.spec.container[x].name -> pod.spec.container[x].name
       // for example: mesh -> mesh-1, envoy -> envoy-2
       hotUpgradeWorkInfo := sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(pod)
       // 计算SidecarSetList的Annotation
       sidecarSetNames := sets.NewString()
       if sidecarSetListStr := pod.Annotations[sidecarcontrol.SidecarSetListAnnotation]; sidecarSetListStr != "" {
           sidecarSetNames.Insert(strings.Split(sidecarSetListStr, ",")...)
       }

       for _, control := range matchedSidecarSets {
           sidecarSet := control.GetSidecarset()
           klog.V(3).Infof("build pod(%s/%s) sidecar containers for sidecarSet(%s)", pod.Namespace, pod.Name, sidecarSet.Name)
           // sidecarSet List
           sidecarSetNames.Insert(sidecarSet.Name)
           // pre-process volumes only in sidecar
           volumesMap := getVolumesMapInSidecarSet(sidecarSet)
           // 计算生成SidecarSetHash Annotation
           setUpgrade1 := sidecarcontrol.SidecarSetUpgradeSpec{
               UpdateTimestamp:              metav1.Now(),
               SidecarSetHash:               sidecarcontrol.GetSidecarSetRevision(sidecarSet),
               SidecarSetName:               sidecarSet.Name,
               SidecarSetControllerRevision: sidecarSet.Status.LatestRevision,
           }
           // 计算生成SidecarSet Without Image Hash Annotation 
           setUpgrade2 := sidecarcontrol.SidecarSetUpgradeSpec{
               UpdateTimestamp: metav1.Now(),
               SidecarSetHash:  sidecarcontrol.GetSidecarSetWithoutImageRevision(sidecarSet),
               SidecarSetName:  sidecarSet.Name,
           }

           // 处理SidecarSet中的InitContainers和PullSecrets，这两个只有Create阶段可以注入
           if !isUpdated {
               for i := range sidecarSet.Spec.InitContainers {
                   initContainer := &sidecarSet.Spec.InitContainers[i]
                   // volumeMounts that injected into sidecar container
                   // when volumeMounts SubPathExpr contains expansions, then need copy container EnvVars(injectEnvs)
                   injectedMounts, injectedEnvs := sidecarcontrol.GetInjectedVolumeMountsAndEnvs(control, initContainer, pod)
                   // get injected env & mounts explicitly so that can be compared with old ones in pod
                   transferEnvs := sidecarcontrol.GetSidecarTransferEnvs(initContainer, pod)
                   // append volumeMounts SubPathExpr environments
                   transferEnvs = util.MergeEnvVar(transferEnvs, injectedEnvs)
                   klog.Infof("try to inject initContainer sidecar %v@%v/%v, with injected envs: %v, volumeMounts: %v",
                       initContainer.Name, pod.Namespace, pod.Name, transferEnvs, injectedMounts)
                   // 计算过程中将需要注入的VolumeMounts加入到volumesInSidecars中
                   for _, mount := range initContainer.VolumeMounts {
                       volumesInSidecars = append(volumesInSidecars, *volumesMap[mount.Name])
                   }
                   // merge VolumeMounts from sidecar.VolumeMounts and shared VolumeMounts
                   initContainer.VolumeMounts = util.MergeVolumeMounts(initContainer.VolumeMounts, injectedMounts)
                   // add "IS_INJECTED" env in initContainer's envs
                   initContainer.Env = append(initContainer.Env, corev1.EnvVar{Name: sidecarcontrol.SidecarEnvKey, Value: "true"})
                   // merged Env from sidecar.Env and transfer envs
                   initContainer.Env = util.MergeEnvVar(initContainer.Env, transferEnvs)
                  【2】计算生成要注入的InitContainers
                   sidecarInitContainers = append(sidecarInitContainers, initContainer)
               }
               // 【3】计算生成 imagePullSecrets
               sidecarSecrets = append(sidecarSecrets, sidecarSet.Spec.ImagePullSecrets...)
           }

           sidecarList := sets.NewString()
           isInjecting := false
           // 处理常规sidecar容器
           for i := range sidecarSet.Spec.Containers {
               sidecarContainer := &sidecarSet.Spec.Containers[i]
               sidecarList.Insert(sidecarContainer.Name)
               // volumeMounts that injected into sidecar container
               // when volumeMounts SubPathExpr contains expansions, then need copy container EnvVars(injectEnvs)
               injectedMounts, injectedEnvs := sidecarcontrol.GetInjectedVolumeMountsAndEnvs(control, sidecarContainer, pod)
               // get injected env & mounts explicitly so that can be compared with old ones in pod
               transferEnvs := sidecarcontrol.GetSidecarTransferEnvs(sidecarContainer, pod)
               // append volumeMounts SubPathExpr environments
               transferEnvs = util.MergeEnvVar(transferEnvs, injectedEnvs)
               klog.Infof("try to inject Container sidecar %v@%v/%v, with injected envs: %v, volumeMounts: %v",
                   sidecarContainer.Name, pod.Namespace, pod.Name, transferEnvs, injectedMounts)
               //when update pod object
               if isUpdated {
                   // judge whether inject sidecar container into pod
                   needInject, existSidecars, existVolumes := control.NeedToInjectInUpdatedPod(pod, oldPod, sidecarContainer, transferEnvs, injectedMounts)
                   if !needInject {
                       sidecarContainers = append(sidecarContainers, existSidecars...)
                       volumesInSidecars = append(volumesInSidecars, existVolumes...)
                       continue
                   }

                   klog.V(3).Infof("upgrade or insert sidecar container %v during upgrade in pod %v/%v",
                       sidecarContainer.Name, pod.Namespace, pod.Name)
                   //when created pod object, need inject sidecar container into pod
               } else {
                   klog.V(3).Infof("inject new sidecar container %v during creation in pod %v/%v",
                       sidecarContainer.Name, pod.Namespace, pod.Name)
               }
               isInjecting = true
               // 计算过程中将需要注入的VolumeMounts加入到volumesInSidecars中
               for _, mount := range sidecarContainer.VolumeMounts {
                   volumesInSidecars = append(volumesInSidecars, *volumesMap[mount.Name])
               }
               // merge VolumeMounts from sidecar.VolumeMounts and shared VolumeMounts
               sidecarContainer.VolumeMounts = util.MergeVolumeMounts(sidecarContainer.VolumeMounts, injectedMounts)
               // add the "Injected" env to the sidecar container
               sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{Name: sidecarcontrol.SidecarEnvKey, Value: "true"})
               // merged Env from sidecar.Env and transfer envs
               sidecarContainer.Env = util.MergeEnvVar(sidecarContainer.Env, transferEnvs)

               // when sidecar container UpgradeStrategy is HotUpgrade
               if sidecarcontrol.IsHotUpgradeContainer(sidecarContainer) {
                   hotContainers, annotations := injectHotUpgradeContainers(hotUpgradeWorkInfo, sidecarContainer)
                   sidecarContainers = append(sidecarContainers, hotContainers...)
                   for k, v := range annotations {
                       injectedAnnotations[k] = v
                   }
               } else {
                   // 【4】计算生成要注入的常规Sidecar Containers
                   sidecarContainers = append(sidecarContainers, sidecarContainer)
               }
           }
           // the container was (re)injected and the annotations need to be updated
           if isInjecting {
               setUpgrade1.SidecarList = sidecarList.List()
               setUpgrade2.SidecarList = sidecarList.List()
               sidecarSetHash[sidecarSet.Name] = setUpgrade1
               sidecarSetHashWithoutImage[sidecarSet.Name] = setUpgrade2
           }
       }

       // 【5】处理要注入的Annotations
       by, _ := json.Marshal(sidecarSetHash)
       injectedAnnotations[sidecarcontrol.SidecarSetHashAnnotation] = string(by)
       by, _ = json.Marshal(sidecarSetHashWithoutImage)
       injectedAnnotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = string(by)
       sidecarSetNameList := strings.Join(sidecarSetNames.List(), ",")
       // store matched sidecarset list in pod annotations
       injectedAnnotations[sidecarcontrol.SidecarSetListAnnotation] = sidecarSetNameList
       return sidecarContainers, sidecarInitContainers, sidecarSecrets, volumesInSidecars, injectedAnnotations, nil
   }
    ```
   
6. 上一步骤中，通过计算最终得到以下需要注入的内容：
   - sidecarContainers：需要注入的常规Sidecar Containers
   - sidecarInitContainers：需要注入的InitContainers
   - sidecarSecrets：需要注入的imagePullSecrets
   - volumesInSidecars：需要注入的VolumeMounts
   - injectedAnnotations：需要注入的Annotations
   
   接下来就是分步骤进行实际的注入（即更新Pod Spec）

7. (注入1)：完成InitContainers的注入

    ```go
    sort.SliceStable(sidecarInitContainers, func(i, j int) bool {
		return sidecarInitContainers[i].Name < sidecarInitContainers[j].Name
	})
	for _, initContainer := range sidecarInitContainers {
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, initContainer.Container)
	}
    ```
   
8. (注入2)：完成常规Sidecar Containers的注入

    ```go
	pod.Spec.Containers = mergeSidecarContainers(pod.Spec.Containers, sidecarContainers)
   
    // 这里merge函数会根据PodInjectPolicy的BeforeAppContainers和AfterAppContainers决定注入的位置
    // 如果容器中已经有对应name的container，会执行替换（index不变，但定义变为sidecarContainer中的定义）
    ```
   
9. (注入3)：完成Volumes的注入

    ```go
    pod.Spec.Volumes = util.MergeVolumes(pod.Spec.Volumes, volumesInSidecar)
    ```
10. (注入4)：完成imagePullSecrets的注入

    ```go
    pod.Spec.ImagePullSecrets = mergeSidecarSecrets(pod.Spec.ImagePullSecrets, sidecarSecrets)
    ```

11. （注入5）：完成Pod Annotation注入

   ```go
   for k, v := range injectedAnnotations {
		pod.Annotations[k] = v
   }
   ```

至此注入完毕，更新通过 `*pod` 指针传递，最终由Pod Webhook进行Mutating，修改实际持久化的对象

### 版本控制与更新策略

### 打散更新

### 热升级

## 核心代码

### 状态计算

### Reconcile整体流程