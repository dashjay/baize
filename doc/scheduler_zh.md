## Scheduler Grpc Interface Design


#### HeartBeat 心跳检测

每个 executor 周期性向 scheduler 上报当前 executor 的情况，包含：
- cpu 数量
- memory 数量
- ……

#### GetJob 获取一个要执行的任

根据调度器标记任务执行器的名字，返回它应该执行的任务

#### FinishJob 完成该任务

执行器请求调度器，已经上传完毕执行结果

#### FailJob 失败该任务

执行器在一定时间内，没有完成该任务的执行，被判定为超时

#### ScheduleJob 新增任务（用户正在等待）

用户发来的任务，会由调度器加入到调度列表当中
