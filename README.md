

垂直 Log

1. 可以单独设置某次请求的日志等级
   1. context 中设置 level，log 中优先判断这个 level
   2. context 中 New Logger
   3. context 中传递 traceId，log 判断traceId，查找等级
   4. 包装 context，各方法传递包装的 context
2. 