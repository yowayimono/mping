***一个简单的ping实现，仿造win下面的ping***

***支持简单的命令行参数，通过go的flag包实现,可以通过`mping -help`查看支持的参数和描述***

***利用ICMP协议***

![img.png](img.png)

- 暂时可能有些问题，win和linux上句柄`syscall.Handle`不同系统上不兼容，导致。把syscall.Handle去掉就行
