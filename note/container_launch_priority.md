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
  - 方案1：修改用户容器镜像的entrypoint，套壳；
  - 问题：可能会影响正常的业务逻辑，会增加容器的镜像大小，同时还会影响资源计算
2. 如果用户的容器本身也有postStart逻辑，怎么进行merge
3. 采用钩子机制必须要对用户的容器顺序重新进行排序

上述问题主要还是1，2两点比较难解决，事实上，在之前K8S 关于 Sidecar Container的 KEP里也有相关的讨论：https://github.
com/kubernetes/enhancements/tree/master/keps/sig-node/753-sidecar-containers#poststart-hook

# Tekton实现

基本原理和思路：hack容器的entry point，注入一个二进制作为替代原有的entry point，在这个二进制引导程序中完成等待操作，然后以子进程方式启动原有的entry point。

EntryPoint 逻辑：

入口 `cmd/entrypoint/main.go`，逻辑由 `pkg/entrypoint/entrypointer.go`实现

```go
// Go optionally waits for a file, runs the command, and writes a
// post file.
func (e Entrypointer) Go() error {
	prod, _ := zap.NewProduction()
	logger := prod.Sugar()

	output := []result.RunResult{}
	defer func() {
		if wErr := termination.WriteMessage(e.TerminationPath, output); wErr != nil {
			logger.Fatalf("Error while writing message: %s", wErr)
		}
		_ = logger.Sync()
	}()

	for _, f := range e.WaitFiles {
		if err := e.Waiter.Wait(f, e.WaitFileContent, e.BreakpointOnFailure); err != nil {
			// An error happened while waiting, so we bail
			// *but* we write postfile to make next steps bail too.
			// In case of breakpoint on failure do not write post file.
			if !e.BreakpointOnFailure {
				e.WritePostFile(e.PostFile, err)
			}
			output = append(output, result.RunResult{
				Key:        "StartedAt",
				Value:      time.Now().Format(timeFormat),
				ResultType: result.InternalTektonResultType,
			})
			return err
		}
	}

	output = append(output, result.RunResult{
		Key:        "StartedAt",
		Value:      time.Now().Format(timeFormat),
		ResultType: result.InternalTektonResultType,
	})

	ctx := context.Background()
	var err error

	if e.Timeout != nil && *e.Timeout < time.Duration(0) {
		err = fmt.Errorf("negative timeout specified")
	}

	if err == nil {
		var cancel context.CancelFunc
		if e.Timeout != nil && *e.Timeout != time.Duration(0) {
			ctx, cancel = context.WithTimeout(ctx, *e.Timeout)
			defer cancel()
		}
		err = e.Runner.Run(ctx, e.Command...)
		if errors.Is(err, context.DeadlineExceeded) {
			output = append(output, result.RunResult{
				Key:        "Reason",
				Value:      "TimeoutExceeded",
				ResultType: result.InternalTektonResultType,
			})
		}
	}

	var ee *exec.ExitError
	switch {
	case err != nil && e.BreakpointOnFailure:
		logger.Info("Skipping writing to PostFile")
	case e.OnError == ContinueOnError && errors.As(err, &ee):
		// with continue on error and an ExitError, write non-zero exit code and a post file
		exitCode := strconv.Itoa(ee.ExitCode())
		output = append(output, result.RunResult{
			Key:        "ExitCode",
			Value:      exitCode,
			ResultType: result.InternalTektonResultType,
		})
		e.WritePostFile(e.PostFile, nil)
		e.WriteExitCodeFile(e.StepMetadataDir, exitCode)
	case err == nil:
		// if err is nil, write zero exit code and a post file
		e.WritePostFile(e.PostFile, nil)
		e.WriteExitCodeFile(e.StepMetadataDir, "0")
	default:
		// for a step without continue on error and any error, write a post file with .err
		e.WritePostFile(e.PostFile, err)
	}

	// strings.Split(..) with an empty string returns an array that contains one element, an empty string.
	// This creates an error when trying to open the result folder as a file.
	if len(e.Results) >= 1 && e.Results[0] != "" {
		resultPath := pipeline.DefaultResultPath
		if e.ResultsDirectory != "" {
			resultPath = e.ResultsDirectory
		}
		if err := e.readResultsFromDisk(ctx, resultPath); err != nil {
			logger.Fatalf("Error while handling results: %s", err)
		}
	}

	return err
}
```

注入逻辑 (替换StepContainer)：

```go
// orderContainers returns the specified steps, modified so that they are
// executed in order by overriding the entrypoint binary.
//
// Containers must have Command specified; if the user didn't specify a
// command, we must have fetched the image's ENTRYPOINT before calling this
// method, using entrypoint_lookup.go.
// Additionally, Step timeouts are added as entrypoint flag.
func orderContainers(commonExtraEntrypointArgs []string, steps []corev1.Container, taskSpec *v1.TaskSpec, breakpointConfig *v1.TaskRunDebug, waitForReadyAnnotation bool) ([]corev1.Container, error) {
	if len(steps) == 0 {
		return nil, errors.New("No steps specified")
	}

	for i, s := range steps {
		var argsForEntrypoint = []string{}
		idx := strconv.Itoa(i)
		if i == 0 {
			if waitForReadyAnnotation {
				argsForEntrypoint = append(argsForEntrypoint,
					// First step waits for the Downward volume file.
					"-wait_file", filepath.Join(downwardMountPoint, downwardMountReadyFile),
					"-wait_file_content", // Wait for file contents, not just an empty file.
				)
			}
		} else { // Not the first step - wait for previous
			argsForEntrypoint = append(argsForEntrypoint, "-wait_file", filepath.Join(RunDir, strconv.Itoa(i-1), "out"))
		}
		argsForEntrypoint = append(argsForEntrypoint,
			// Start next step.
			"-post_file", filepath.Join(RunDir, idx, "out"),
			"-termination_path", terminationPath,
			"-step_metadata_dir", filepath.Join(RunDir, idx, "status"),
		)
		argsForEntrypoint = append(argsForEntrypoint, commonExtraEntrypointArgs...)
		if taskSpec != nil {
			if taskSpec.Steps != nil && len(taskSpec.Steps) >= i+1 {
				if taskSpec.Steps[i].OnError != "" {
					if taskSpec.Steps[i].OnError != v1.Continue && taskSpec.Steps[i].OnError != v1.StopAndFail {
						return nil, fmt.Errorf("task step onError must be either \"%s\" or \"%s\" but it is set to an invalid value \"%s\"",
							v1.Continue, v1.StopAndFail, taskSpec.Steps[i].OnError)
					}
					argsForEntrypoint = append(argsForEntrypoint, "-on_error", string(taskSpec.Steps[i].OnError))
				}
				if taskSpec.Steps[i].Timeout != nil {
					argsForEntrypoint = append(argsForEntrypoint, "-timeout", taskSpec.Steps[i].Timeout.Duration.String())
				}
				if taskSpec.Steps[i].StdoutConfig != nil {
					argsForEntrypoint = append(argsForEntrypoint, "-stdout_path", taskSpec.Steps[i].StdoutConfig.Path)
				}
				if taskSpec.Steps[i].StderrConfig != nil {
					argsForEntrypoint = append(argsForEntrypoint, "-stderr_path", taskSpec.Steps[i].StderrConfig.Path)
				}
			}
			argsForEntrypoint = append(argsForEntrypoint, resultArgument(steps, taskSpec.Results)...)
		}

		if breakpointConfig != nil && len(breakpointConfig.Breakpoint) > 0 {
			breakpoints := breakpointConfig.Breakpoint
			for _, b := range breakpoints {
				// TODO(TEP #0042): Add other breakpoints
				if b == breakpointOnFailure {
					argsForEntrypoint = append(argsForEntrypoint, "-breakpoint_on_failure")
				}
			}
		}

		cmd, args := s.Command, s.Args
		if len(cmd) > 0 {
			argsForEntrypoint = append(argsForEntrypoint, "-entrypoint", cmd[0])
		}
		if len(cmd) > 1 {
			args = append(cmd[1:], args...)
		}
		argsForEntrypoint = append(argsForEntrypoint, "--")
		argsForEntrypoint = append(argsForEntrypoint, args...)

		steps[i].Command = []string{entrypointBinary}
		steps[i].Args = argsForEntrypoint
		steps[i].TerminationMessagePath = terminationPath
	}
	if waitForReadyAnnotation {
		// Mount the Downward volume into the first step container.
		steps[0].VolumeMounts = append(steps[0].VolumeMounts, downwardMount)
	}

	return steps, nil
}
```