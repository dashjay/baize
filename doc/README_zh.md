# BAIZE
<p align="center">
    <a href="https://github.com/dashjay/baize" target="_blank">
        <img src="/baize.jpg" width="400">
    </a>
</p>

> 白泽，中国古代神话中的瑞兽。能言语，通万物之情，知鬼神之事，“王者有德”才出现，能辟除人间一切邪气

> 为什么使用这个名字？
> 因为它的发音特别像 Google 开源的 [bazel](https://bazel.build/)  然后这个项目时和bazel相关的

## 关于

这个项目是 [bazel remote execution](https://github.com/bazelbuild/remote-apis) 的最小实现，可以用来学习了解 bazel 的远程执行时如何工作的。我也期望这个项目能够逐步完善，并且可以在生产环境中被使用

## 开发

修改 `cmd` 和 `pkg` 中的代码, 运行如下脚本

```bash
./deploy/docker-compose/up.sh
```